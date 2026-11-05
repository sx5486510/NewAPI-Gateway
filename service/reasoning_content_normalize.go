package service

import (
	"bytes"
	"encoding/json"
	"strings"
)

// normalizeReasoningContent rewrites assistant messages that carry a
// client-specific "reasoning" field into the "reasoning_content" field required
// by DeepSeek thinking-mode requests that include tools.
//
// It is deliberately narrow and opt-in:
//   - it only runs for routes with NormalizeReasoningContent enabled (see proxy.go);
//   - it only acts when the request carries a non-empty tools array;
//   - it only touches assistant messages with a non-empty string reasoning
//     value and no existing reasoning_content key.
//
// The original reasoning field and every other key (client metadata, content,
// tool_calls, ...) are preserved verbatim. When nothing needs to change the
// original body bytes are returned unchanged, so the default relay path stays
// byte-for-byte transparent.
func normalizeReasoningContent(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	var request map[string]json.RawMessage
	if err := json.Unmarshal(body, &request); err != nil {
		return body
	}
	if !hasNonEmptyTools(request["tools"]) {
		return body
	}
	encodedMessages, ok := request["messages"]
	if !ok || !bytes.HasPrefix(bytes.TrimSpace(encodedMessages), []byte("[")) {
		return body
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(encodedMessages, &messages); err != nil {
		return body
	}

	changed := false
	for i, message := range messages {
		rewritten, ok := appendReasoningContent(message)
		if !ok {
			continue
		}
		messages[i] = rewritten
		changed = true
	}
	if !changed {
		return body
	}

	encoded, err := json.Marshal(messages)
	if err != nil {
		return body
	}
	request["messages"] = encoded
	out, err := json.Marshal(request)
	if err != nil {
		return body
	}
	return out
}

// hasNonEmptyTools reports whether the raw value is a JSON array with at least
// one element. Requests without tools are ignored by the upstream thinking-mode
// validation, so they must not be rewritten.
func hasNonEmptyTools(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) <= 2 || trimmed[0] != '[' {
		return false
	}
	var tools []json.RawMessage
	return json.Unmarshal(trimmed, &tools) == nil && len(tools) > 0
}

// appendReasoningContent adds reasoning_content to a single assistant message
// when its reasoning value is a non-empty string. The original raw value is
// reused so escaping and content stay identical.
func appendReasoningContent(message json.RawMessage) (json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(message, &fields); err != nil {
		return nil, false
	}
	var role string
	if err := json.Unmarshal(fields["role"], &role); err != nil || !strings.EqualFold(role, "assistant") {
		return nil, false
	}
	if _, exists := fields["reasoning_content"]; exists {
		return nil, false
	}
	reasoning, ok := fields["reasoning"]
	if !ok {
		return nil, false
	}
	var value string
	if err := json.Unmarshal(reasoning, &value); err != nil || strings.TrimSpace(value) == "" {
		return nil, false
	}
	fields["reasoning_content"] = reasoning
	out, err := json.Marshal(fields)
	if err != nil {
		return nil, false
	}
	return out, true
}
