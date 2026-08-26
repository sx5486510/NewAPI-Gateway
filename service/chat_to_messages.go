package service

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicMessageResponse struct {
	Id           string                   `json:"id"`
	Type         string                   `json:"type"`
	Role         string                   `json:"role"`
	Model        string                   `json:"model"`
	Content      []map[string]interface{} `json:"content"`
	StopReason   string                   `json:"stop_reason"`
	StopSequence interface{}              `json:"stop_sequence"`
	Usage        anthropicUsage           `json:"usage"`
}

// ConvertChatResponseToMessages converts a non-streaming OpenAI Chat
// Completions response body into an Anthropic Messages response body. It
// never fabricates a thinking block when the upstream did not provide
// reasoning_content, and returns an error for malformed/error-shaped bodies
// so the caller can retry instead of forwarding a broken conversion.
func ConvertChatResponseToMessages(body []byte) ([]byte, error) {
	var resp struct {
		Id      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content          interface{} `json:"content"`
				ReasoningContent string      `json:"reasoning_content"`
				ToolCalls        []struct {
					Id       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, newMessagesConversionError("invalid JSON response body: %v", err)
	}
	if len(resp.Choices) == 0 {
		return nil, newMessagesConversionError("response has no choices")
	}
	choice := resp.Choices[0]

	out := anthropicMessageResponse{
		Id:    resp.Id,
		Type:  "message",
		Role:  "assistant",
		Model: resp.Model,
		Usage: anthropicUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}
	if out.Id == "" {
		out.Id = "msg_" + uuid.New().String()
	}

	content := make([]map[string]interface{}, 0)
	if choice.Message.ReasoningContent != "" {
		content = append(content, map[string]interface{}{
			"type":     "thinking",
			"thinking": choice.Message.ReasoningContent,
		})
	}
	if text := chatMessageContentToText(choice.Message.Content); text != "" {
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": text,
		})
	}
	for _, tc := range choice.Message.ToolCalls {
		var input interface{} = map[string]interface{}{}
		if strings.TrimSpace(tc.Function.Arguments) != "" {
			var parsed interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &parsed); err == nil {
				input = parsed
			} else {
				input = map[string]interface{}{"_raw": tc.Function.Arguments}
			}
		}
		content = append(content, map[string]interface{}{
			"type":  "tool_use",
			"id":    tc.Id,
			"name":  tc.Function.Name,
			"input": input,
		})
	}
	if len(content) == 0 {
		content = append(content, map[string]interface{}{"type": "text", "text": ""})
	}
	out.Content = content
	out.StopReason = mapFinishReasonToStopReason(choice.FinishReason)

	return json.Marshal(out)
}

func chatMessageContentToText(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var sb strings.Builder
		for _, item := range v {
			block, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if getStringValue(block["type"]) == "text" {
				sb.WriteString(getStringValue(block["text"]))
			}
		}
		return sb.String()
	default:
		return ""
	}
}

func mapFinishReasonToStopReason(finishReason string) string {
	switch finishReason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	case "content_filter":
		return "refusal"
	case "":
		return "end_turn"
	default:
		return "end_turn"
	}
}

// ConvertChatErrorToMessagesError converts an upstream OpenAI-style error
// response into an Anthropic-compatible error response body, so a
// non-retryable upstream failure never leaks the OpenAI error shape to an
// Anthropic client.
func ConvertChatErrorToMessagesError(statusCode int, upstreamErr upstreamErrorInfo) []byte {
	errorType := mapStatusToAnthropicErrorType(statusCode, upstreamErr)
	message := upstreamErrorText(upstreamErr)
	if message == "" {
		message = "upstream request failed"
	}
	body, err := json.Marshal(map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    errorType,
			"message": message,
		},
	})
	if err != nil {
		return []byte(`{"type":"error","error":{"type":"api_error","message":"upstream request failed"}}`)
	}
	return body
}

func mapStatusToAnthropicErrorType(statusCode int, upstreamErr upstreamErrorInfo) string {
	switch statusCode {
	case 400:
		return "invalid_request_error"
	case 401:
		return "authentication_error"
	case 403:
		return "permission_error"
	case 404:
		return "not_found_error"
	case 408:
		return "timeout_error"
	case 413:
		return "invalid_request_error"
	case 422:
		return "invalid_request_error"
	case 429:
		return "rate_limit_error"
	case 529:
		return "overloaded_error"
	default:
		if statusCode >= 500 {
			return "api_error"
		}
		return "invalid_request_error"
	}
}
