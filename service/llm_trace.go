package service

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

const maxTraceStreamCaptureBytes = 10 * 1024 * 1024

type llmTraceInput struct {
	AggToken         *model.AggregatedToken
	Provider         *model.Provider
	Token            *model.ProviderToken
	Context          *gin.Context
	RequestId        string
	ModelName        string
	Method           string
	Path             string
	StatusCode       int
	RequestedStream  bool
	ResponseIsStream bool
	RequestBody      []byte
	ResponseBody     []byte
	ErrorMessage     string

	// Resolved from Context before the trace is handed to a goroutine.
	ClientIp  string
	UserAgent string
}

type traceStreamCapture struct {
	enabled bool
	builder strings.Builder
	limit   int
}

func newTraceStreamCapture() *traceStreamCapture {
	return &traceStreamCapture{
		enabled: common.LLMTraceEnabled,
		limit:   maxTraceStreamCaptureBytes,
	}
}

func (c *traceStreamCapture) appendLine(line string) {
	if c == nil || !c.enabled {
		return
	}
	remaining := c.limit - c.builder.Len()
	if remaining <= 0 {
		return
	}
	text := line + "\n"
	if len(text) > remaining {
		text = text[:remaining]
	}
	c.builder.WriteString(text)
}

// appendRaw appends text as-is (no implicit trailing newline), for callers
// that already include their own line terminators (e.g. converted SSE
// events, which may span multiple lines per logical chunk).
func (c *traceStreamCapture) appendRaw(text string) {
	if c == nil || !c.enabled || text == "" {
		return
	}
	remaining := c.limit - c.builder.Len()
	if remaining <= 0 {
		return
	}
	if len(text) > remaining {
		text = text[:remaining]
	}
	c.builder.WriteString(text)
}

func (c *traceStreamCapture) String() string {
	if c == nil || !c.enabled {
		return ""
	}
	return c.builder.String()
}

// pendingLLMTraces tracks in-flight async traces so callers (shutdown, tests)
// can drain them instead of racing the goroutine.
var pendingLLMTraces sync.WaitGroup

// WaitForPendingLLMTraces blocks until every async trace has been written.
func WaitForPendingLLMTraces() {
	pendingLLMTraces.Wait()
}

// captureLLMTraceAsync runs the trace off the request goroutine.
//
// The security audit plus the insert cost hundreds of milliseconds on a Claude
// Code sized payload, and net/http only finalizes the response once the handler
// returns — so doing this inline stalls the client after its last token.
// Anything derived from the gin context is resolved here, before the goroutine
// starts: once the handler returns gin may recycle the context and its request.
func captureLLMTraceAsync(input llmTraceInput) {
	if !common.LLMTraceEnabled {
		return
	}
	if input.AggToken == nil || input.Provider == nil || input.Token == nil || input.Context == nil {
		return
	}
	input.ClientIp = input.Context.ClientIP()
	input.UserAgent = strings.TrimSpace(input.Context.GetHeader("User-Agent"))
	input.Context = nil
	pendingLLMTraces.Add(1)
	go func() {
		defer pendingLLMTraces.Done()
		writeLLMTrace(input)
	}()
}

func captureLLMTrace(input llmTraceInput) {
	if !common.LLMTraceEnabled {
		return
	}
	if input.AggToken == nil || input.Provider == nil || input.Token == nil || input.Context == nil {
		return
	}
	input.ClientIp = input.Context.ClientIP()
	input.UserAgent = strings.TrimSpace(input.Context.GetHeader("User-Agent"))
	writeLLMTrace(input)
}

func writeLLMTrace(input llmTraceInput) {
	// Perform security audit
	auditResult := AuditLLMContent(string(input.RequestBody), string(input.ResponseBody))
	riskTagsJSON := "[]"
	if len(auditResult.RiskTags) > 0 {
		if jsonBytes, err := json.Marshal(auditResult.RiskTags); err == nil {
			riskTagsJSON = string(jsonBytes)
		}
	}

	trace := &model.LLMTrace{
		RequestId:         input.RequestId,
		UserId:            input.AggToken.UserId,
		AggregatedTokenId: input.AggToken.Id,
		ProviderId:        input.Provider.Id,
		ProviderName:      input.Provider.Name,
		ProviderTokenId:   input.Token.Id,
		TokenGroupName:    strings.TrimSpace(input.Token.GroupName),
		ModelName:         input.ModelName,
		Method:            input.Method,
		Path:              input.Path,
		StatusCode:        input.StatusCode,
		RequestedStream:   input.RequestedStream,
		ResponseIsStream:  input.ResponseIsStream,
		RequestBody:       string(input.RequestBody),
		ResponseBody:      string(input.ResponseBody),
		ErrorMessage:      input.ErrorMessage,
		ClientIp:          input.ClientIp,
		UserAgent:         input.UserAgent,
		RiskLevel:         auditResult.RiskLevel,
		RiskTags:          riskTagsJSON,
		AutoReviewed:      true,
	}
	if err := trace.Insert(); err != nil {
		common.SysLog(fmt.Sprintf("failed to insert llm trace: %v", err))
	}
}
