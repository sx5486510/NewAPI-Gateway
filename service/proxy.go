package service

import (
	"bufio"
	"bytes"
	"fmt"
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"io"
	"net/http"
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
		"Content-Type", "Accept", "Accept-Encoding", "Accept-Language", "User-Agent",
	}
	for _, h := range safeHeaders {
		if v := c.GetHeader(h); v != "" {
			req.Header.Set(h, v)
		}
	}

	// 5. Set authentication — replace ag-token with upstream sk-token
	req.Header.Set("Authorization", "Bearer "+token.SkKey)

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
		logUsage(aggToken, provider, token, c, requestId, "", 0, 0, startTime, 0, err.Error())
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
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintf(c.Writer, "%s\n", line)
			if ok {
				flusher.Flush()
			}
		}
		// Log usage (for stream, we can't easily count tokens from SSE, set 0)
		elapsed := time.Since(startTime).Milliseconds()
		logUsage(aggToken, provider, token, c, requestId, "", 0, 0, startTime, int(elapsed), "")
	} else {
		// Non-streaming response
		c.Status(resp.StatusCode)
		respBody, _ := io.ReadAll(resp.Body)
		c.Writer.Write(respBody)

		elapsed := time.Since(startTime).Milliseconds()
		errorMsg := ""
		if resp.StatusCode >= 400 {
			errorMsg = string(respBody)
			if len(errorMsg) > 500 {
				errorMsg = errorMsg[:500]
			}
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
