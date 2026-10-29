package service

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// pendingProgressInterval is the minimum gap between two progressive updates
// of an in-flight request record.
const pendingProgressInterval = 5 * time.Second

// pendingBodyCaptureLimit caps how much of the request body is stored on the
// initial (in-flight) record. Bigger payloads are only stored on finalize, so
// accepting a request stays cheap.
const pendingBodyCaptureLimit = 32 * 1024

// requestRecorder owns the lifecycle of the usage log / LLM trace pair for a
// single upstream attempt.
//
// Instead of writing the record only once the response is finished, a pending
// row is created the moment the request is accepted, and every milestone
// (first token, periodic token counts, final result) patches that same row.
type requestRecorder struct {
	mu       sync.Mutex
	seq      int64
	applied  int64
	finished bool

	// ready is closed once the in-flight rows exist (or the insert gave up).
	// Every writer waits on it inside its own goroutine, so the request path
	// never blocks on the pending insert.
	ready chan struct{}

	usageLogID int64
	traceID    int64

	aggToken        *model.AggregatedToken
	provider        *model.Provider
	token           *model.ProviderToken
	requestID       string
	modelName       string
	method          string
	path            string
	requestedStream bool
	clientIP        string
	userAgent       string
	requestBody     string

	startTime        time.Time
	lastProgressAt   time.Time
	progressInterval time.Duration
}

type requestRecorderInput struct {
	AggToken        *model.AggregatedToken
	Provider        *model.Provider
	Token           *model.ProviderToken
	Context         *gin.Context
	RequestId       string
	ModelName       string
	Method          string
	Path            string
	RequestedStream bool
	RequestBody     []byte
}

// newRequestRecorder creates the in-flight rows for a request.
//
// The insert runs off the request goroutine: it used to be synchronous and sat
// in front of the upstream call, so every request paid a full database commit
// (tens of milliseconds with the rollback journal) before its first byte went
// out. Ordering is preserved by r.ready — the final update and every progress
// patch wait for the insert to finish before touching the row.
//
// It never returns nil: if the insert fails the recorder degrades to
// "write once at the end", which is the previous behaviour.
func newRequestRecorder(input requestRecorderInput) *requestRecorder {
	now := time.Now()
	recorder := &requestRecorder{
		ready:            make(chan struct{}),
		aggToken:         input.AggToken,
		provider:         input.Provider,
		token:            input.Token,
		requestID:        input.RequestId,
		modelName:        input.ModelName,
		method:           input.Method,
		path:             input.Path,
		requestedStream:  input.RequestedStream,
		startTime:        now,
		lastProgressAt:   now,
		progressInterval: pendingProgressInterval,
	}
	if len(input.RequestBody) > 0 {
		body := string(input.RequestBody)
		if len(body) > pendingBodyCaptureLimit {
			body = ""
		}
		recorder.requestBody = body
	}
	if input.Context != nil {
		recorder.clientIP = input.Context.ClientIP()
		recorder.userAgent = strings.TrimSpace(input.Context.GetHeader("User-Agent"))
	}

	recorder.insertPendingAsync()
	return recorder
}

// insertPendingAsync writes the pending usage log / trace in the background and
// closes r.ready when it is done. It is tracked by pendingLLMTraces so shutdown
// and tests can drain it instead of racing the goroutine.
func (r *requestRecorder) insertPendingAsync() {
	pendingLLMTraces.Add(1)
	go func() {
		defer pendingLLMTraces.Done()
		defer close(r.ready)
		r.insertPending()
	}()
}

// awaitPendingInsert blocks until the pending rows exist or the insert failed.
// Production code never calls it on the request goroutine.
func (r *requestRecorder) awaitPendingInsert() {
	if r == nil || r.ready == nil {
		return
	}
	<-r.ready
}

func (r *requestRecorder) insertPending() {
	if r == nil || r.aggToken == nil || r.provider == nil || r.token == nil {
		return
	}

	usageLog := &model.UsageLog{
		UserId:            r.aggToken.UserId,
		AggregatedTokenId: r.aggToken.Id,
		ProviderId:        r.provider.Id,
		ProviderName:      r.provider.Name,
		ProviderTokenId:   r.token.Id,
		TokenGroupName:    strings.TrimSpace(r.token.GroupName),
		ModelName:         r.modelName,
		RequestedStream:   r.requestedStream,
		Status:            model.UsageLogStatusPending,
		ClientIp:          r.clientIP,
		UserAgent:         r.userAgent,
		RequestId:         r.requestId(),
	}
	if err := usageLog.Insert(); err != nil {
		common.SysLog(fmt.Sprintf("failed to insert pending usage log: %v", err))
		return
	}
	r.usageLogID = usageLog.Id

	if !common.LLMTraceEnabled {
		return
	}
	trace := &model.LLMTrace{
		RequestId:         r.requestId(),
		UserId:            r.aggToken.UserId,
		AggregatedTokenId: r.aggToken.Id,
		ProviderId:        r.provider.Id,
		ProviderName:      r.provider.Name,
		ProviderTokenId:   r.token.Id,
		TokenGroupName:    strings.TrimSpace(r.token.GroupName),
		ModelName:         r.modelName,
		Method:            r.method,
		Path:              r.path,
		RequestedStream:   r.requestedStream,
		RequestBody:       r.requestBody,
		ClientIp:          r.clientIP,
		UserAgent:         r.userAgent,
		InProgress:        true,
		RiskLevel:         "unknown",
		RiskTags:          "[]",
	}
	if err := trace.Insert(); err != nil {
		common.SysLog(fmt.Sprintf("failed to insert pending llm trace: %v", err))
		return
	}
	r.traceID = trace.Id
}

func (r *requestRecorder) requestId() string {
	return r.requestID
}

// MarkFirstToken records the moment the upstream produced its first token.
func (r *requestRecorder) MarkFirstToken(firstTokenMs int) {
	if r == nil || firstTokenMs <= 0 {
		return
	}
	r.patch(map[string]interface{}{
		"first_token_ms": firstTokenMs,
	}, nil)
}

// ShouldUpdateProgress reports whether a progressive refresh is due.
func (r *requestRecorder) ShouldUpdateProgress() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return false
	}
	return time.Since(r.lastProgressAt) >= r.progressInterval
}

// UpdateProgress refreshes the partial token counts / elapsed time of an
// in-flight request so a long stream stays visible and up to date.
func (r *requestRecorder) UpdateProgress(usage usageMetrics, responseIsStream bool, firstTokenMs int, responseTimeMs int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.lastProgressAt = time.Now()
	r.mu.Unlock()

	values := map[string]interface{}{
		"prompt_tokens":      usage.PromptTokens,
		"completion_tokens":  usage.CompletionTokens,
		"cache_tokens":       usage.CacheTokens,
		"response_time_ms":   responseTimeMs,
		"response_is_stream": responseIsStream,
		"is_stream":          responseIsStream,
	}
	if usage.ModelName != "" {
		values["model_name"] = usage.ModelName
	}
	if firstTokenMs > 0 {
		values["first_token_ms"] = firstTokenMs
	}
	if usage.CostUSD > 0 {
		values["cost_usd"] = usage.CostUSD
	}
	r.patch(values, nil)
}

type recordFinishInput struct {
	Usage            usageMetrics
	StatusCode       int
	ResponseIsStream bool
	FirstTokenMs     int
	ResponseTimeMs   int
	ErrorMessage     string
	RequestBody      []byte
	ResponseBody     []byte
}

// Finish completes the record: it is the only place that decides success vs
// failure, and the only place that stores response bodies / audit results.
func (r *requestRecorder) Finish(input recordFinishInput) {
	if r == nil {
		return
	}

	r.mu.Lock()
	if r.finished {
		r.mu.Unlock()
		return
	}
	r.finished = true
	r.mu.Unlock()

	// The security audit plus the body write is expensive (hundreds of ms on
	// a Claude Code sized payload), and net/http only finalizes the response
	// once the handler returns — keep it off the request goroutine.
	pendingLLMTraces.Add(1)
	go func() {
		defer pendingLLMTraces.Done()
		r.writeFinal(input)
	}()
}

func (r *requestRecorder) writeFinal(input recordFinishInput) {
	// The pending insert may still be in flight; wait for it so the final
	// values patch that row instead of racing it.
	r.awaitPendingInsert()

	// Serialize against any in-flight progress write so the final values are
	// always the last ones applied.
	r.mu.Lock()
	defer r.mu.Unlock()

	modelName := strings.TrimSpace(input.Usage.ModelName)
	if modelName == "" {
		modelName = r.modelName
	}
	status := model.UsageLogStatusSuccess
	if input.ErrorMessage != "" {
		status = model.UsageLogStatusFailed
	}
	httpStatus, errorType, upstreamHost := extractErrorKeyInfo(input.ErrorMessage)
	costUSD := input.Usage.CostUSD
	if costUSD <= 0 {
		costUSD = estimateUsageCostUSD(r.providerId(), modelName, input.Usage.PromptTokens, input.Usage.CompletionTokens)
	}

	usageValues := map[string]interface{}{
		"model_name":              modelName,
		"prompt_tokens":           input.Usage.PromptTokens,
		"completion_tokens":       input.Usage.CompletionTokens,
		"cache_tokens":            input.Usage.CacheTokens,
		"cache_creation_tokens":   input.Usage.CacheCreationTokens,
		"cache_creation5m_tokens": input.Usage.CacheCreation5mTokens,
		"cache_creation1h_tokens": input.Usage.CacheCreation1hTokens,
		"response_time_ms":        input.ResponseTimeMs,
		"first_token_ms":          input.FirstTokenMs,
		"is_stream":               input.ResponseIsStream,
		"response_is_stream":      input.ResponseIsStream,
		"cost_usd":                costUSD,
		"status":                  status,
		"error_message":           input.ErrorMessage,
		"error_http_status":       httpStatus,
		"error_type":              errorType,
		"upstream_host":           upstreamHost,
	}

	if r.usageLogID <= 0 {
		// The pending insert failed earlier — fall back to a single insert.
		r.insertFallbackUsageLog(modelName, input, status, costUSD, httpStatus, errorType, upstreamHost)
	} else {
		if err := model.UpdateUsageLog(r.usageLogID, usageValues); err != nil {
			common.SysLog(fmt.Sprintf("failed to update usage log: %v", err))
		}
	}

	if common.LLMTraceEnabled {
		r.writeFinalTrace(input, modelName)
	}
}

func (r *requestRecorder) writeFinalTrace(input recordFinishInput, modelName string) {
	requestBody := string(input.RequestBody)
	if requestBody == "" {
		requestBody = r.requestBody
	}
	responseBody := string(input.ResponseBody)

	auditResult := AuditLLMContent(requestBody, responseBody)
	riskTagsJSON := "[]"
	if len(auditResult.RiskTags) > 0 {
		if jsonBytes, err := json.Marshal(auditResult.RiskTags); err == nil {
			riskTagsJSON = string(jsonBytes)
		}
	}

	traceValues := map[string]interface{}{
		"model_name":         modelName,
		"status_code":        input.StatusCode,
		"response_is_stream": input.ResponseIsStream,
		"request_body":       requestBody,
		"response_body":      responseBody,
		"error_message":      input.ErrorMessage,
		"risk_level":         auditResult.RiskLevel,
		"risk_tags":          riskTagsJSON,
		"auto_reviewed":      true,
		"in_progress":        false,
	}

	if r.traceID <= 0 {
		if r.aggToken == nil || r.provider == nil || r.token == nil {
			return
		}
		trace := &model.LLMTrace{
			RequestId:         r.requestId(),
			UserId:            r.aggToken.UserId,
			AggregatedTokenId: r.aggToken.Id,
			ProviderId:        r.provider.Id,
			ProviderName:      r.provider.Name,
			ProviderTokenId:   r.token.Id,
			TokenGroupName:    strings.TrimSpace(r.token.GroupName),
			ModelName:         modelName,
			Method:            r.method,
			Path:              r.path,
			StatusCode:        input.StatusCode,
			RequestedStream:   r.requestedStream,
			ResponseIsStream:  input.ResponseIsStream,
			RequestBody:       requestBody,
			ResponseBody:      responseBody,
			ErrorMessage:      input.ErrorMessage,
			ClientIp:          r.clientIP,
			UserAgent:         r.userAgent,
			RiskLevel:         auditResult.RiskLevel,
			RiskTags:          riskTagsJSON,
			AutoReviewed:      true,
			InProgress:        false,
		}
		if err := trace.Insert(); err != nil {
			common.SysLog(fmt.Sprintf("failed to insert llm trace: %v", err))
		}
		return
	}

	if err := model.UpdateLLMTrace(r.traceID, traceValues); err != nil {
		common.SysLog(fmt.Sprintf("failed to update llm trace: %v", err))
	}
}

func (r *requestRecorder) insertFallbackUsageLog(modelName string, input recordFinishInput,
	status int, costUSD float64, httpStatus int, errorType string, upstreamHost string) {
	if r.aggToken == nil || r.provider == nil || r.token == nil {
		return
	}
	log := &model.UsageLog{
		UserId:                r.aggToken.UserId,
		AggregatedTokenId:     r.aggToken.Id,
		ProviderId:            r.provider.Id,
		ProviderName:          r.provider.Name,
		ProviderTokenId:       r.token.Id,
		TokenGroupName:        strings.TrimSpace(r.token.GroupName),
		ModelName:             modelName,
		PromptTokens:          input.Usage.PromptTokens,
		CompletionTokens:      input.Usage.CompletionTokens,
		CacheTokens:           input.Usage.CacheTokens,
		CacheCreationTokens:   input.Usage.CacheCreationTokens,
		CacheCreation5mTokens: input.Usage.CacheCreation5mTokens,
		CacheCreation1hTokens: input.Usage.CacheCreation1hTokens,
		ResponseTimeMs:        input.ResponseTimeMs,
		FirstTokenMs:          input.FirstTokenMs,
		IsStream:              input.ResponseIsStream,
		RequestedStream:       r.requestedStream,
		ResponseIsStream:      input.ResponseIsStream,
		CostUSD:               costUSD,
		Status:                status,
		ErrorMessage:          input.ErrorMessage,
		ErrorHttpStatus:       httpStatus,
		ErrorType:             errorType,
		UpstreamHost:          upstreamHost,
		ClientIp:              r.clientIP,
		UserAgent:             r.userAgent,
		RequestId:             r.requestId(),
	}
	if err := log.Insert(); err != nil {
		common.SysLog(fmt.Sprintf("failed to insert usage log: %v", err))
	}
}

// patch schedules an incremental write. Writes are serialized through a mutex
// and tagged with a sequence number, so a slow progress update can never
// overwrite the final result.
func (r *requestRecorder) patch(usageValues map[string]interface{}, traceValues map[string]interface{}) {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.seq++
	seq := r.seq
	r.mu.Unlock()

	pendingLLMTraces.Add(1)
	go func() {
		defer pendingLLMTraces.Done()

		r.awaitPendingInsert()

		r.mu.Lock()
		defer r.mu.Unlock()
		if r.finished || seq <= r.applied {
			return
		}
		r.applied = seq

		if r.usageLogID > 0 && len(usageValues) > 0 {
			if err := model.UpdateUsageLog(r.usageLogID, usageValues); err != nil {
				common.SysLog(fmt.Sprintf("failed to update usage log progress: %v", err))
			}
		}
		if r.traceID > 0 && len(traceValues) > 0 {
			if err := model.UpdateLLMTrace(r.traceID, traceValues); err != nil {
				common.SysLog(fmt.Sprintf("failed to update llm trace progress: %v", err))
			}
		}
	}()
}

// EnsureFinished closes the record if no branch did it explicitly, so an
// unexpected return path can never leave a row pending forever.
func (r *requestRecorder) EnsureFinished() {
	if r == nil {
		return
	}
	r.mu.Lock()
	done := r.finished
	r.mu.Unlock()
	if done {
		return
	}
	r.Finish(recordFinishInput{
		StatusCode:   http.StatusBadGateway,
		ErrorMessage: "request ended without a recorded result",
		RequestBody:  []byte(r.requestBody),
	})
}

func (r *requestRecorder) providerId() int {
	if r.provider == nil {
		return 0
	}
	return r.provider.Id
}

// reapStalePendingRecords finalizes in-flight rows that never completed.
//
// cutoff is a unix timestamp: rows still pending and not touched since then
// belong to requests that are gone (process restart, hard disconnect).
func reapStalePendingRecords(cutoff int64) {
	message := "request did not complete (connection lost or service restarted)"
	if n, err := model.MarkStaleUsageLogsFailed(cutoff, message); err != nil {
		common.SysLog(fmt.Sprintf("failed to reap pending usage logs: %v", err))
	} else if n > 0 {
		common.SysLog(fmt.Sprintf("reaped %d pending usage logs", n))
	}
	if n, err := model.MarkStaleLLMTracesFailed(cutoff, message); err != nil {
		common.SysLog(fmt.Sprintf("failed to reap pending llm traces: %v", err))
	} else if n > 0 {
		common.SysLog(fmt.Sprintf("reaped %d pending llm traces", n))
	}
}

// ReapAllPendingRecords marks every in-flight row as failed. It runs on
// startup: after a restart no request can still be in flight.
func ReapAllPendingRecords() {
	reapStalePendingRecords(time.Now().Unix() + 1)
}
