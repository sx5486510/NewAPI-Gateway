package service

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

type sseToolCallDelta struct {
	Index    int    `json:"index"`
	Id       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type sseChoiceDelta struct {
	Role             string             `json:"role"`
	Content          string             `json:"content"`
	ReasoningContent string             `json:"reasoning_content"`
	ToolCalls        []sseToolCallDelta `json:"tool_calls"`
}

type sseChunk struct {
	Id      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta        sseChoiceDelta `json:"delta"`
		FinishReason string         `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// MessagesSSEConverter converts an OpenAI Chat Completions streaming
// response (one line at a time) into an equivalent Anthropic Messages SSE
// event sequence. It is stateful: content block boundaries are inferred from
// consecutive delta shapes so a single converter instance must be used for
// the whole lifetime of one upstream stream.
type MessagesSSEConverter struct {
	requestedModel      string
	messageId           string
	started             bool
	finished            bool
	nextIndex           int
	openIndex           int
	openKind            string
	toolBlockByOAIIndex map[int]int
	usage               anthropicUsage
	stopReason          string
}

// NewMessagesSSEConverter creates a converter that falls back to
// requestedModel in the message_start event if the upstream chunk omits the
// model field.
func NewMessagesSSEConverter(requestedModel string) *MessagesSSEConverter {
	return &MessagesSSEConverter{
		requestedModel:      requestedModel,
		openIndex:           -1,
		toolBlockByOAIIndex: make(map[int]int),
	}
}

// HasEmittedFirstEvent reports whether the converter has already produced at
// least one Anthropic SSE event (i.e. message_start). Callers use this to
// decide whether a mid-stream failure may still fall back to the next route.
func (conv *MessagesSSEConverter) HasEmittedFirstEvent() bool {
	return conv.started
}

// ConvertLine consumes a single raw line read from the upstream OpenAI SSE
// body and returns the equivalent Anthropic SSE bytes to write to the
// client, if any. A non-nil error means the upstream chunk was malformed or
// carried an inline error object; the caller is responsible for deciding
// whether to retry (before the first event) or terminate the stream (after).
func (conv *MessagesSSEConverter) ConvertLine(line string) ([]byte, error) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "data:") {
		return nil, nil
	}
	dataContent := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if dataContent == "" {
		return nil, nil
	}
	if dataContent == "[DONE]" {
		return conv.Finalize()
	}

	var chunk sseChunk
	if err := json.Unmarshal([]byte(dataContent), &chunk); err != nil {
		return nil, newMessagesConversionError("invalid SSE JSON chunk: %v", err)
	}
	if chunk.Error != nil {
		msg := chunk.Error.Message
		if msg == "" {
			msg = chunk.Error.Type
		}
		if msg == "" {
			msg = chunk.Error.Code
		}
		if msg == "" {
			msg = "upstream stream error"
		}
		return nil, newMessagesConversionError("%s", msg)
	}

	var buf bytes.Buffer
	conv.ensureStarted(&buf, chunk.Id, chunk.Model)

	if chunk.Usage != nil {
		conv.usage.InputTokens = chunk.Usage.PromptTokens
		conv.usage.OutputTokens = chunk.Usage.CompletionTokens
	}

	if len(chunk.Choices) > 0 {
		choice := chunk.Choices[0]
		conv.applyDelta(&buf, choice.Delta)
		if choice.FinishReason != "" {
			conv.stopReason = mapFinishReasonToStopReason(choice.FinishReason)
		}
	}
	return buf.Bytes(), nil
}

// Finalize closes any open content block and emits message_delta +
// message_stop. It is safe to call more than once; subsequent calls are
// no-ops. Callers must invoke this on [DONE] and also on unexpected stream
// EOF so the client always receives a well-formed Anthropic event sequence.
func (conv *MessagesSSEConverter) Finalize() ([]byte, error) {
	if conv.finished {
		return nil, nil
	}
	var buf bytes.Buffer
	conv.ensureStarted(&buf, "", conv.requestedModel)
	conv.closeOpenBlock(&buf)

	stopReason := conv.stopReason
	if stopReason == "" {
		stopReason = "end_turn"
	}
	writeAnthropicSSEEvent(&buf, "message_delta", map[string]interface{}{
		"type":  "message_delta",
		"delta": map[string]interface{}{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": map[string]interface{}{"output_tokens": conv.usage.OutputTokens},
	})
	writeAnthropicSSEEvent(&buf, "message_stop", map[string]interface{}{"type": "message_stop"})
	conv.finished = true
	return buf.Bytes(), nil
}

func (conv *MessagesSSEConverter) ensureStarted(buf *bytes.Buffer, id string, model string) {
	if conv.started {
		return
	}
	conv.started = true
	conv.messageId = id
	if conv.messageId == "" {
		conv.messageId = "msg_" + uuid.New().String()
	}
	modelName := model
	if modelName == "" {
		modelName = conv.requestedModel
	}
	writeAnthropicSSEEvent(buf, "message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            conv.messageId,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         modelName,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]interface{}{"input_tokens": 0, "output_tokens": 0},
		},
	})
}

func (conv *MessagesSSEConverter) applyDelta(buf *bytes.Buffer, delta sseChoiceDelta) {
	if delta.ReasoningContent != "" {
		conv.ensureTextLikeBlock(buf, "thinking")
		writeAnthropicSSEEvent(buf, "content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": conv.openIndex,
			"delta": map[string]interface{}{"type": "thinking_delta", "thinking": delta.ReasoningContent},
		})
	}
	if delta.Content != "" {
		conv.ensureTextLikeBlock(buf, "text")
		writeAnthropicSSEEvent(buf, "content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": conv.openIndex,
			"delta": map[string]interface{}{"type": "text_delta", "text": delta.Content},
		})
	}
	for _, tc := range delta.ToolCalls {
		conv.ensureToolUseBlock(buf, tc)
		if tc.Function.Arguments != "" {
			writeAnthropicSSEEvent(buf, "content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": conv.openIndex,
				"delta": map[string]interface{}{"type": "input_json_delta", "partial_json": tc.Function.Arguments},
			})
		}
	}
}

func (conv *MessagesSSEConverter) ensureTextLikeBlock(buf *bytes.Buffer, kind string) {
	if conv.openKind == kind && conv.openIndex != -1 {
		return
	}
	conv.closeOpenBlock(buf)
	newIndex := conv.nextIndex
	conv.nextIndex++
	conv.openIndex = newIndex
	conv.openKind = kind
	blockPayload := map[string]interface{}{"type": kind}
	if kind == "text" {
		blockPayload["text"] = ""
	} else {
		blockPayload["thinking"] = ""
	}
	writeAnthropicSSEEvent(buf, "content_block_start", map[string]interface{}{
		"type":          "content_block_start",
		"index":         newIndex,
		"content_block": blockPayload,
	})
}

func (conv *MessagesSSEConverter) ensureToolUseBlock(buf *bytes.Buffer, tc sseToolCallDelta) {
	if idx, ok := conv.toolBlockByOAIIndex[tc.Index]; ok {
		if conv.openIndex == idx && conv.openKind == "tool_use" {
			return
		}
		conv.closeOpenBlock(buf)
		conv.openIndex = idx
		conv.openKind = "tool_use"
		return
	}

	conv.closeOpenBlock(buf)
	newIndex := conv.nextIndex
	conv.nextIndex++
	conv.toolBlockByOAIIndex[tc.Index] = newIndex
	conv.openIndex = newIndex
	conv.openKind = "tool_use"
	writeAnthropicSSEEvent(buf, "content_block_start", map[string]interface{}{
		"type":  "content_block_start",
		"index": newIndex,
		"content_block": map[string]interface{}{
			"type":  "tool_use",
			"id":    tc.Id,
			"name":  tc.Function.Name,
			"input": map[string]interface{}{},
		},
	})
}

func (conv *MessagesSSEConverter) closeOpenBlock(buf *bytes.Buffer) {
	if conv.openIndex == -1 {
		return
	}
	writeAnthropicSSEEvent(buf, "content_block_stop", map[string]interface{}{
		"type":  "content_block_stop",
		"index": conv.openIndex,
	})
	conv.openIndex = -1
	conv.openKind = ""
}

func writeAnthropicSSEEvent(buf *bytes.Buffer, event string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	buf.WriteString("event: ")
	buf.WriteString(event)
	buf.WriteString("\n")
	buf.WriteString("data: ")
	buf.Write(data)
	buf.WriteString("\n\n")
}

// buildAnthropicStreamErrorEvent renders a terminal Anthropic "error" SSE
// event for a mid-stream upstream failure. It is used only after at least
// one real Anthropic event has already reached the client, so the stream
// cannot be retried on a different route; the client instead sees an
// explicit error event instead of a truncated/garbled stream.
func buildAnthropicStreamErrorEvent(err error) []byte {
	var buf bytes.Buffer
	writeAnthropicSSEEvent(&buf, "error", map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "api_error",
			"message": err.Error(),
		},
	})
	return buf.Bytes()
}
