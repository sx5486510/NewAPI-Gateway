package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func extractUpstreamErrorInfo(body []byte) struct {
	Type    string
	Code    string
	Message string
} {
	info := struct {
		Type    string
		Code    string
		Message string
	}{}
	if len(body) == 0 {
		return info
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return info
	}

	if errField, ok := payload["error"].(map[string]interface{}); ok {
		if typ, ok := errField["type"].(string); ok {
			info.Type = strings.TrimSpace(typ)
		}
		if code, ok := errField["code"].(string); ok {
			info.Code = strings.TrimSpace(code)
		}
		if msg, ok := errField["message"].(string); ok {
			info.Message = strings.TrimSpace(msg)
		}
	}

	if info.Type == "" {
		if typ, ok := payload["type"].(string); ok {
			info.Type = strings.TrimSpace(typ)
		}
	}
	if info.Code == "" {
		if code, ok := payload["code"].(string); ok {
			info.Code = strings.TrimSpace(code)
		}
	}
	if info.Message == "" {
		if msg, ok := payload["message"].(string); ok {
			info.Message = strings.TrimSpace(msg)
		}
	}
	return info
}

func extractUpstreamErrorInfoFromPayload(payload map[string]interface{}) struct {
	Type    string
	Code    string
	Message string
} {
	info := struct {
		Type    string
		Code    string
		Message string
	}{}

	if errField, ok := payload["error"].(map[string]interface{}); ok {
		if typ, ok := errField["type"].(string); ok {
			info.Type = strings.TrimSpace(typ)
		}
		if code, ok := errField["code"].(string); ok {
			info.Code = strings.TrimSpace(code)
		}
		if msg, ok := errField["message"].(string); ok {
			info.Message = strings.TrimSpace(msg)
		}
	}

	if info.Type == "" {
		if typ, ok := payload["type"].(string); ok {
			info.Type = strings.TrimSpace(typ)
		}
	}
	if info.Code == "" {
		if code, ok := payload["code"].(string); ok {
			info.Code = strings.TrimSpace(code)
		}
	}
	if info.Message == "" {
		if msg, ok := payload["message"].(string); ok {
			info.Message = strings.TrimSpace(msg)
		}
	}
	return info
}

func getStringValue(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func getBoolValue(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	}
	return false, false
}

func upstreamErrorText(info struct {
	Type    string
	Code    string
	Message string
}) string {
	if info.Message != "" {
		return info.Message
	}
	if info.Code != "" {
		return info.Code
	}
	if info.Type != "" {
		return info.Type
	}
	return ""
}

func extractLLMResponseErrorMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}

	info := extractUpstreamErrorInfo(body)
	errorText := upstreamErrorText(info)
	typeValue := strings.ToLower(strings.TrimSpace(getStringValue(payload["type"])))
	if _, ok := payload["error"]; ok {
		return errorText
	}

	if response, ok := payload["response"].(map[string]interface{}); ok {
		responseInfo := extractUpstreamErrorInfoFromPayload(response)
		responseErrorText := upstreamErrorText(responseInfo)
		responseStatus := strings.ToLower(strings.TrimSpace(getStringValue(response["status"])))
		if responseStatus == "failed" || responseStatus == "error" || typeValue == "response.failed" {
			if responseErrorText != "" {
				return responseErrorText
			}
			if responseStatus == "failed" || typeValue == "response.failed" {
				return "response status failed"
			}
			return "response status error"
		}
		if _, ok := response["error"]; ok {
			return responseErrorText
		}
	}

	status := strings.ToLower(strings.TrimSpace(getStringValue(payload["status"])))
	if status == "failed" || status == "error" {
		if errorText != "" {
			return errorText
		}
		if status == "failed" {
			return "response status failed"
		}
		return "response status error"
	}

	if typeValue == "error" {
		if errorText != "" {
			return errorText
		}
		return "response type error"
	}

	if success, ok := getBoolValue(payload["success"]); ok && !success {
		if errorText != "" {
			return errorText
		}
		return "response success false"
	}

	return ""
}

func extractSSELineErrorMessage(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.Contains(trimmed, "event: codex.rate_limits") {
		return "upstream rate limit event detected"
	}
	if !strings.HasPrefix(trimmed, "data:") {
		return ""
	}

	dataContent := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if dataContent == "" || dataContent == "[DONE]" {
		return ""
	}
	if bodyError := extractLLMResponseErrorMessage([]byte(dataContent)); bodyError != "" {
		return fmt.Sprintf("upstream SSE response error: %s", bodyError)
	}
	return ""
}

func main() {
	testLines := []string{
		`data:`,
		`{`,
		`  "type": "ping"`,
		`}`,
		`data: {"type": "ping"}`,
		``,
		`event: response.failed`,
		`data: {"type":"response.failed","response":{"id":"resp_d1d9d56f37db4b3ca2b239ab1b885835","object":"response","model":"claude-opus-4-8","status":"failed","output":[],"error":{"code":"rate_limit_exceeded","message":"Concurrency limit exceeded for account, please retry later"}}}`,
	}

	fmt.Println("Testing SSE Line Error Detection:")
	fmt.Println("=" + strings.Repeat("=", 80))
	for i, line := range testLines {
		result := extractSSELineErrorMessage(line)
		if result != "" {
			fmt.Printf("Line %d: %s\n", i+1, line)
			fmt.Printf("  ✅ Detected: %s\n\n", result)
		} else {
			fmt.Printf("Line %d: %s\n", i+1, line)
			fmt.Printf("  ⚪ No error (normal)\n\n")
		}
	}
}
