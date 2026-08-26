package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MessagesConversionError represents a client-facing error produced while
// converting an Anthropic Messages request/response. Callers must translate
// it into an HTTP 400 invalid_request_error without leaking internal detail.
type MessagesConversionError struct {
	Message string
}

func (e *MessagesConversionError) Error() string {
	return e.Message
}

func newMessagesConversionError(format string, args ...interface{}) *MessagesConversionError {
	return &MessagesConversionError{Message: fmt.Sprintf(format, args...)}
}

// ConvertAnthropicRequestToChat converts an Anthropic /v1/messages request
// body into an OpenAI /v1/chat/completions request body. Unknown content
// blocks or unsupported tool_choice shapes return a *MessagesConversionError
// instead of silently dropping data.
func ConvertAnthropicRequestToChat(body []byte) ([]byte, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, newMessagesConversionError("invalid JSON request body: %v", err)
	}

	out := map[string]interface{}{}

	if v, ok := req["model"]; ok {
		out["model"] = v
	}
	if v, ok := req["max_tokens"]; ok {
		out["max_tokens"] = v
	}
	if v, ok := req["temperature"]; ok {
		out["temperature"] = v
	}
	if v, ok := req["top_p"]; ok {
		out["top_p"] = v
	}
	if v, ok := req["stop_sequences"]; ok {
		out["stop"] = v
	}
	if v, ok := req["stream"]; ok {
		out["stream"] = v
	}

	messages := make([]map[string]interface{}, 0)

	if sysRaw, ok := req["system"]; ok {
		systemText, err := convertAnthropicSystemField(sysRaw)
		if err != nil {
			return nil, err
		}
		if systemText != "" {
			messages = append(messages, map[string]interface{}{
				"role":    "system",
				"content": systemText,
			})
		}
	}

	rawMessages, ok := req["messages"].([]interface{})
	if !ok {
		return nil, newMessagesConversionError("messages field must be an array")
	}
	for i, rawMsg := range rawMessages {
		msgMap, ok := rawMsg.(map[string]interface{})
		if !ok {
			return nil, newMessagesConversionError("messages[%d] must be an object", i)
		}
		converted, err := convertAnthropicMessage(msgMap)
		if err != nil {
			return nil, newMessagesConversionError("messages[%d]: %s", i, err.Error())
		}
		messages = append(messages, converted...)
	}
	out["messages"] = messages

	if toolsRaw, ok := req["tools"]; ok {
		tools, err := convertAnthropicTools(toolsRaw)
		if err != nil {
			return nil, err
		}
		out["tools"] = tools
	}

	if toolChoiceRaw, ok := req["tool_choice"]; ok {
		toolChoice, err := convertAnthropicToolChoice(toolChoiceRaw)
		if err != nil {
			return nil, err
		}
		out["tool_choice"] = toolChoice
	}

	return json.Marshal(out)
}

func convertAnthropicSystemField(raw interface{}) (string, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case []interface{}:
		parts := make([]string, 0, len(v))
		for i, item := range v {
			block, ok := item.(map[string]interface{})
			if !ok {
				return "", newMessagesConversionError("system[%d] must be an object", i)
			}
			blockType := getStringValue(block["type"])
			if blockType != "text" {
				return "", newMessagesConversionError("system[%d] has unsupported content block type: %s", i, blockType)
			}
			parts = append(parts, getStringValue(block["text"]))
		}
		return strings.Join(parts, "\n\n"), nil
	case nil:
		return "", nil
	default:
		return "", newMessagesConversionError("system field must be a string or an array of text blocks")
	}
}

func anthropicTextPart(text string) map[string]interface{} {
	return map[string]interface{}{"type": "text", "text": text}
}

func convertAnthropicImageBlock(block map[string]interface{}) (map[string]interface{}, error) {
	source, ok := block["source"].(map[string]interface{})
	if !ok {
		return nil, newMessagesConversionError("image block missing source object")
	}
	sourceType := getStringValue(source["type"])
	if sourceType != "base64" {
		return nil, newMessagesConversionError("unsupported image source type: %s", sourceType)
	}
	mediaType := getStringValue(source["media_type"])
	data := getStringValue(source["data"])
	if mediaType == "" || data == "" {
		return nil, newMessagesConversionError("image source requires media_type and data")
	}
	url := fmt.Sprintf("data:%s;base64,%s", mediaType, data)
	return map[string]interface{}{
		"type":      "image_url",
		"image_url": map[string]interface{}{"url": url},
	}, nil
}

func convertAnthropicToolResultContent(raw interface{}) (interface{}, error) {
	switch v := raw.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case []interface{}:
		textParts := make([]string, 0, len(v))
		parts := make([]map[string]interface{}, 0, len(v))
		hasImage := false
		for i, item := range v {
			block, ok := item.(map[string]interface{})
			if !ok {
				return nil, newMessagesConversionError("tool_result content[%d] must be an object", i)
			}
			blockType := getStringValue(block["type"])
			switch blockType {
			case "text":
				text := getStringValue(block["text"])
				textParts = append(textParts, text)
				parts = append(parts, anthropicTextPart(text))
			case "image":
				hasImage = true
				part, err := convertAnthropicImageBlock(block)
				if err != nil {
					return nil, err
				}
				parts = append(parts, part)
			default:
				return nil, newMessagesConversionError("tool_result content[%d] has unsupported content block type: %s", i, blockType)
			}
		}
		if hasImage {
			return parts, nil
		}
		return strings.Join(textParts, ""), nil
	default:
		return nil, newMessagesConversionError("tool_result content must be a string or an array of content blocks")
	}
}

func prefixAnthropicToolErrorContent(content interface{}) interface{} {
	switch v := content.(type) {
	case string:
		return "Error: " + v
	case []map[string]interface{}:
		return append([]map[string]interface{}{anthropicTextPart("Error: ")}, v...)
	default:
		return content
	}
}

func convertAnthropicMessage(msg map[string]interface{}) ([]map[string]interface{}, error) {
	role := getStringValue(msg["role"])
	if role != "user" && role != "assistant" && role != "system" {
		return nil, newMessagesConversionError("unsupported message role: %s", role)
	}

	content := msg["content"]
	if text, ok := content.(string); ok {
		return []map[string]interface{}{
			{"role": role, "content": text},
		}, nil
	}

	if role == "system" {
		text, err := convertAnthropicSystemField(content)
		if err != nil {
			return nil, err
		}
		return []map[string]interface{}{
			{"role": "system", "content": text},
		}, nil
	}

	blocks, ok := content.([]interface{})
	if !ok {
		return nil, newMessagesConversionError("message content must be a string or an array of content blocks")
	}

	if role == "assistant" {
		return convertAnthropicAssistantMessage(blocks)
	}
	return convertAnthropicUserMessage(blocks)
}

func convertAnthropicAssistantMessage(blocks []interface{}) ([]map[string]interface{}, error) {
	var textParts []string
	var contentParts []map[string]interface{}
	var toolCalls []map[string]interface{}
	hasImage := false

	for i, item := range blocks {
		block, ok := item.(map[string]interface{})
		if !ok {
			return nil, newMessagesConversionError("content[%d] must be an object", i)
		}
		blockType := getStringValue(block["type"])
		switch blockType {
		case "text":
			text := getStringValue(block["text"])
			textParts = append(textParts, text)
			contentParts = append(contentParts, anthropicTextPart(text))
		case "thinking":
			wrapped := "<thinking>" + getStringValue(block["thinking"]) + "</thinking>"
			textParts = append(textParts, wrapped)
			contentParts = append(contentParts, anthropicTextPart(wrapped))
		case "redacted_thinking":
			wrapped := "<redacted_thinking>" + getStringValue(block["data"]) + "</redacted_thinking>"
			textParts = append(textParts, wrapped)
			contentParts = append(contentParts, anthropicTextPart(wrapped))
		case "image":
			hasImage = true
			part, err := convertAnthropicImageBlock(block)
			if err != nil {
				return nil, err
			}
			contentParts = append(contentParts, part)
		case "tool_use":
			id := getStringValue(block["id"])
			name := getStringValue(block["name"])
			if id == "" || name == "" {
				return nil, newMessagesConversionError("tool_use block requires id and name")
			}
			input := block["input"]
			if input == nil {
				input = map[string]interface{}{}
			}
			argsBytes, err := json.Marshal(input)
			if err != nil {
				return nil, newMessagesConversionError("tool_use[%s] input is not serializable: %v", id, err)
			}
			toolCalls = append(toolCalls, map[string]interface{}{
				"id":   id,
				"type": "function",
				"function": map[string]interface{}{
					"name":      name,
					"arguments": string(argsBytes),
				},
			})
		default:
			return nil, newMessagesConversionError("content[%d] has unsupported content block type: %s", i, blockType)
		}
	}

	msgOut := map[string]interface{}{"role": "assistant"}
	switch {
	case hasImage:
		if len(contentParts) > 0 {
			msgOut["content"] = contentParts
		} else {
			msgOut["content"] = nil
		}
	case len(textParts) > 0:
		msgOut["content"] = strings.Join(textParts, "")
	case len(toolCalls) > 0:
		msgOut["content"] = nil
	default:
		msgOut["content"] = ""
	}
	if len(toolCalls) > 0 {
		msgOut["tool_calls"] = toolCalls
	}
	return []map[string]interface{}{msgOut}, nil
}

func convertAnthropicUserMessage(blocks []interface{}) ([]map[string]interface{}, error) {
	var messagesOut []map[string]interface{}
	var pendingParts []map[string]interface{}
	var pendingTextParts []string
	hasImage := false

	flush := func() {
		if len(pendingParts) == 0 {
			return
		}
		var contentVal interface{}
		if hasImage {
			contentVal = pendingParts
		} else {
			contentVal = strings.Join(pendingTextParts, "")
		}
		messagesOut = append(messagesOut, map[string]interface{}{"role": "user", "content": contentVal})
		pendingParts = nil
		pendingTextParts = nil
		hasImage = false
	}

	for i, item := range blocks {
		block, ok := item.(map[string]interface{})
		if !ok {
			return nil, newMessagesConversionError("content[%d] must be an object", i)
		}
		blockType := getStringValue(block["type"])
		switch blockType {
		case "text":
			text := getStringValue(block["text"])
			pendingTextParts = append(pendingTextParts, text)
			pendingParts = append(pendingParts, anthropicTextPart(text))
		case "thinking":
			wrapped := "<thinking>" + getStringValue(block["thinking"]) + "</thinking>"
			pendingTextParts = append(pendingTextParts, wrapped)
			pendingParts = append(pendingParts, anthropicTextPart(wrapped))
		case "redacted_thinking":
			wrapped := "<redacted_thinking>" + getStringValue(block["data"]) + "</redacted_thinking>"
			pendingTextParts = append(pendingTextParts, wrapped)
			pendingParts = append(pendingParts, anthropicTextPart(wrapped))
		case "image":
			hasImage = true
			part, err := convertAnthropicImageBlock(block)
			if err != nil {
				return nil, err
			}
			pendingParts = append(pendingParts, part)
		case "tool_result":
			flush()
			toolUseId := getStringValue(block["tool_use_id"])
			if toolUseId == "" {
				return nil, newMessagesConversionError("tool_result block requires tool_use_id")
			}
			contentVal, err := convertAnthropicToolResultContent(block["content"])
			if err != nil {
				return nil, err
			}
			if isError, ok := getBoolValue(block["is_error"]); ok && isError {
				contentVal = prefixAnthropicToolErrorContent(contentVal)
			}
			messagesOut = append(messagesOut, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": toolUseId,
				"content":      contentVal,
			})
		default:
			return nil, newMessagesConversionError("content[%d] has unsupported content block type: %s", i, blockType)
		}
	}
	flush()

	if len(messagesOut) == 0 {
		messagesOut = append(messagesOut, map[string]interface{}{"role": "user", "content": ""})
	}
	return messagesOut, nil
}

func convertAnthropicTools(raw interface{}) ([]map[string]interface{}, error) {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, newMessagesConversionError("tools field must be an array")
	}
	out := make([]map[string]interface{}, 0, len(arr))
	for i, item := range arr {
		tool, ok := item.(map[string]interface{})
		if !ok {
			return nil, newMessagesConversionError("tools[%d] must be an object", i)
		}
		name := getStringValue(tool["name"])
		if name == "" {
			return nil, newMessagesConversionError("tools[%d].name is required", i)
		}
		schema := tool["input_schema"]
		if schema == nil {
			schema = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
		}
		function := map[string]interface{}{
			"name":       name,
			"parameters": schema,
		}
		if description := getStringValue(tool["description"]); description != "" {
			function["description"] = description
		}
		out = append(out, map[string]interface{}{
			"type":     "function",
			"function": function,
		})
	}
	return out, nil
}

func convertAnthropicToolChoice(raw interface{}) (interface{}, error) {
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return nil, newMessagesConversionError("tool_choice must be an object")
	}
	choiceType := getStringValue(obj["type"])
	switch choiceType {
	case "auto":
		return "auto", nil
	case "any":
		return "required", nil
	case "tool":
		name := getStringValue(obj["name"])
		if name == "" {
			return nil, newMessagesConversionError("tool_choice.name is required when type=tool")
		}
		return map[string]interface{}{
			"type":     "function",
			"function": map[string]interface{}{"name": name},
		}, nil
	default:
		return nil, newMessagesConversionError("unsupported tool_choice.type: %s", choiceType)
	}
}
