package service

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var proxyHTTPClient = &http.Client{
	Timeout: 5 * time.Minute,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	},
}

// ProxyToUpstream forwards the client request to the selected upstream provider
func ProxyToUpstream(c *gin.Context, token *model.ProviderToken, provider *model.Provider) {
	startTime := time.Now()
	requestId := uuid.New().String()[:8]

	// Get user info from context
	aggToken := c.MustGet("agg_token").(*model.AggregatedToken)

	// 1. Read original request body
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "failed to read request body",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// 2. Construct upstream URL
	upstreamURL := strings.TrimRight(provider.BaseURL, "/") + c.Request.URL.Path
	if c.Request.URL.RawQuery != "" {
		upstreamURL += "?" + c.Request.URL.RawQuery
	}

	// 3. Create upstream request
	req, err := http.NewRequest(c.Request.Method, upstreamURL, bytes.NewReader(bodyBytes))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "failed to create upstream request",
				"type":    "server_error",
			},
		})
		return
	}

	// 4. Carefully set headers — transparency is KEY
	// Only forward safe headers, remove all proxy-revealing headers
	safeHeaders := []string{
		"Content-Type", "Accept", "Accept-Encoding", "Accept-Language", "User-Agent", "anthropic-beta",
	}
	for _, h := range safeHeaders {
		if v := c.GetHeader(h); v != "" {
			req.Header.Set(h, v)
		}
	}

	// 5. Set authentication — replace ag-token with upstream sk-token
	req.Header.Set("Authorization", "Bearer "+token.SkKey)
	logProxyAuthDebug(c, req, requestId, provider, token)

	// 6. Anthropic compatibility
	if isAnthropicPath(c.Request.URL.Path) {
		req.Header.Set("x-api-key", token.SkKey)
		if v := c.GetHeader("anthropic-version"); v != "" {
			req.Header.Set("anthropic-version", v)
		}
	}

	// 7. Gemini compatibility
	if isGeminiPath(c.Request.URL.Path) {
		req.Header.Set("x-goog-api-key", token.SkKey)
	}

	// 8. REMOVE all proxy-revealing headers
	req.Header.Del("X-Forwarded-For")
	req.Header.Del("X-Forwarded-Host")
	req.Header.Del("X-Forwarded-Proto")
	req.Header.Del("X-Real-IP")
	req.Header.Del("Via")
	req.Header.Del("Forwarded")

	// 9. Send request
	resp, err := proxyHTTPClient.Do(req)
	if err != nil {
		errorMsg := buildErrorMessage(err.Error(), c, bodyBytes)
		logProxyErrorTrace(c, requestId, provider, token, errorMsg)
		logUsage(aggToken, provider, token, c, requestId, "", 0, 0, startTime, 0, errorMsg)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": "upstream request failed: " + err.Error(),
				"type":    "server_error",
			},
		})
		return
	}
	defer resp.Body.Close()

	// 10. Detect if streaming
	contentType := resp.Header.Get("Content-Type")
	isStream := strings.Contains(contentType, "text/event-stream")

	// 11. Copy response headers
	for key, values := range resp.Header {
		lowerKey := strings.ToLower(key)
		// Skip hop-by-hop headers
		if lowerKey == "transfer-encoding" || lowerKey == "connection" {
			continue
		}
		for _, v := range values {
			c.Writer.Header().Add(key, v)
		}
	}

	if isStream {
		// Stream SSE response
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Status(resp.StatusCode)
		flusher, ok := c.Writer.(http.Flusher)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var streamErrLines []string
		errorStatus := resp.StatusCode >= 400
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintf(c.Writer, "%s\n", line)
			if errorStatus && len(streamErrLines) < 5 {
				streamErrLines = append(streamErrLines, line)
			}
			if ok {
				flusher.Flush()
			}
		}
		errorMsg := ""
		if errorStatus {
			parts := []string{fmt.Sprintf("upstream status %d", resp.StatusCode)}
			if len(streamErrLines) > 0 {
				parts = append(parts, strings.Join(streamErrLines, "\n"))
			}
			errorMsg = strings.Join(parts, ": ")
		}
		if scanErr := scanner.Err(); scanErr != nil {
			if errorMsg != "" {
				errorMsg += "; scanner error: " + scanErr.Error()
			} else {
				errorMsg = "stream scanner error: " + scanErr.Error()
			}
		}
		if errorMsg != "" {
			errorMsg = buildErrorMessage(errorMsg, c, bodyBytes)
			logProxyErrorTrace(c, requestId, provider, token, errorMsg)
		}
		// Log usage (for stream, we can't easily count tokens from SSE, set 0)
		elapsed := time.Since(startTime).Milliseconds()
		logUsage(aggToken, provider, token, c, requestId, "", 0, 0, startTime, int(elapsed), errorMsg)
	} else {
		// Non-streaming response
		c.Status(resp.StatusCode)
		respBody, _ := io.ReadAll(resp.Body)
		c.Writer.Write(respBody)

		elapsed := time.Since(startTime).Milliseconds()
		errorMsg := ""
		if resp.StatusCode >= 400 {
			errorMsg = buildErrorMessage(string(respBody), c, bodyBytes)
			logProxyErrorTrace(c, requestId, provider, token, errorMsg)
		}
		logUsage(aggToken, provider, token, c, requestId, "", 0, 0, startTime, int(elapsed), errorMsg)
	}
}

func logUsage(aggToken *model.AggregatedToken, provider *model.Provider, token *model.ProviderToken,
	c *gin.Context, requestId string, modelName string,
	promptTokens int, completionTokens int, startTime time.Time, responseTimeMs int, errorMsg string) {

	status := 1
	if errorMsg != "" {
		status = 0
	}

	// Try to extract model from request path or body
	if modelName == "" {
		modelName = c.GetString("request_model")
	}

	log := &model.UsageLog{
		UserId:            aggToken.UserId,
		AggregatedTokenId: aggToken.Id,
		ProviderId:        provider.Id,
		ProviderName:      provider.Name,
		ProviderTokenId:   token.Id,
		ModelName:         modelName,
		PromptTokens:      promptTokens,
		CompletionTokens:  completionTokens,
		ResponseTimeMs:    responseTimeMs,
		Status:            status,
		ErrorMessage:      errorMsg,
		ClientIp:          c.ClientIP(),
		RequestId:         requestId,
	}
	go func() {
		if err := log.Insert(); err != nil {
			common.SysLog(fmt.Sprintf("failed to insert usage log: %v", err))
		}
	}()
}

func isAnthropicPath(path string) bool {
	return strings.Contains(path, "/v1/messages")
}

func isGeminiPath(path string) bool {
	return strings.Contains(path, "/v1beta/")
}

func logProxyAuthDebug(c *gin.Context, req *http.Request, requestId string, provider *model.Provider, token *model.ProviderToken) {
	// Enable only when explicitly requested to avoid noisy/sensitive logs.
	if strings.ToLower(os.Getenv("DEBUG_PROXY_AUTH")) != "1" {
		return
	}

	incomingAuth := c.GetHeader("Authorization")
	if incomingAuth == "" {
		incomingAuth = c.GetHeader("x-api-key")
	}
	if incomingAuth == "" {
		incomingAuth = c.GetHeader("x-goog-api-key")
	}
	if incomingAuth == "" {
		incomingAuth = c.Query("key")
	}

	outgoingAuth := req.Header.Get("Authorization")
	common.SysLog(fmt.Sprintf(
		"[proxy-auth] request_id=%s provider=%s provider_id=%d provider_token_id=%d incoming=%s outgoing=%s",
		requestId,
		provider.Name,
		provider.Id,
		token.Id,
		tokenSummary(incomingAuth),
		tokenSummary(outgoingAuth),
	))
}

func tokenSummary(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "(empty)"
	}

	value := strings.TrimSpace(raw)
	prefix := ""
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		prefix = "Bearer "
		value = strings.TrimSpace(value[7:])
	}

	if value == "" {
		return prefix + "(empty)"
	}

	masked := value
	if len(value) > 8 {
		masked = value[:4] + "..." + value[len(value)-4:]
	}
	hash := sha256.Sum256([]byte(value))
	fp := hex.EncodeToString(hash[:6])
	return prefix + masked + "(sha256:" + fp + ")"
}

func buildErrorMessage(base string, c *gin.Context, bodyBytes []byte) string {
	msg := strings.TrimSpace(base)
	requestBodyLog := requestBodyForErrorLog(c, bodyBytes)
	if requestBodyLog != "" {
		if msg == "" {
			msg = "request body: " + requestBodyLog
		} else {
			msg += "\nrequest body: " + requestBodyLog
		}
	}
	const maxErrorMessageLen = 20000
	if len(msg) > maxErrorMessageLen {
		msg = msg[:maxErrorMessageLen] + "...(truncated)"
	}
	return msg
}

func requestBodyForErrorLog(c *gin.Context, bodyBytes []byte) string {
	if len(bodyBytes) == 0 {
		return "(empty)"
	}
	contentType := strings.ToLower(strings.TrimSpace(c.GetHeader("Content-Type")))
	if strings.Contains(contentType, "application/json") {
		return strings.TrimSpace(string(bodyBytes))
	}
	return fmt.Sprintf("(non-json omitted) content_type=%s body_size=%d", contentType, len(bodyBytes))
}

func logProxyErrorTrace(c *gin.Context, requestId string, provider *model.Provider, token *model.ProviderToken, errorMsg string) {
	compactError := strings.ReplaceAll(strings.ReplaceAll(errorMsg, "\n", " "), "\r", " ")
	if len(compactError) > 1200 {
		compactError = compactError[:1200] + "...(truncated)"
	}
	common.SysError(fmt.Sprintf(
		"[proxy-error] request_id=%s method=%s path=%s provider=%s provider_id=%d provider_token_id=%d model=%s client_ip=%s detail=%s",
		requestId,
		c.Request.Method,
		c.Request.URL.Path,
		provider.Name,
		provider.Id,
		token.Id,
		c.GetString("request_model"),
		c.ClientIP(),
		compactError,
	))
}
