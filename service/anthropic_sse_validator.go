package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// AnthropicSSEValidator checks content block lifecycle without changing the
// bytes sent to the client. It is intentionally diagnostic-only.
type AnthropicSSEValidator struct {
	pendingEvent string
	openBlocks   map[int]bool
}

func NewAnthropicSSEValidator() *AnthropicSSEValidator {
	return &AnthropicSSEValidator{openBlocks: make(map[int]bool)}
}

func (v *AnthropicSSEValidator) ConsumeBytes(raw []byte) error {
	for _, line := range strings.Split(string(raw), "\n") {
		if err := v.ConsumeLine(line); err != nil {
			return err
		}
	}
	return nil
}

func (v *AnthropicSSEValidator) ConsumeLine(line string) error {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "event:") {
		v.pendingEvent = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		return nil
	}
	if !strings.HasPrefix(trimmed, "data:") {
		return nil
	}
	data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if data == "" || data == "[DONE]" {
		return nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil
	}
	event := v.pendingEvent
	v.pendingEvent = ""
	if event == "" {
		event = getStringValue(payload["type"])
	}
	switch event {
	case "content_block_start":
		index, ok := sseBlockIndex(payload)
		if !ok {
			return fmt.Errorf("invalid SSE response: content_block_start is missing a numeric index")
		}
		if v.openBlocks[index] {
			return fmt.Errorf("invalid SSE response: content_block_start reopens block index %d", index)
		}
		v.openBlocks[index] = true
	case "content_block_delta":
		index, ok := sseBlockIndex(payload)
		if !ok {
			return fmt.Errorf("invalid SSE response: content_block_delta is missing a numeric index")
		}
		if !v.openBlocks[index] {
			return fmt.Errorf("invalid SSE response: content_block_delta references unopened block index %d", index)
		}
	case "content_block_stop":
		index, ok := sseBlockIndex(payload)
		if !ok {
			return fmt.Errorf("invalid SSE response: content_block_stop is missing a numeric index")
		}
		if !v.openBlocks[index] {
			return fmt.Errorf("invalid SSE response: content_block_stop references unopened block index %d", index)
		}
		delete(v.openBlocks, index)
	}
	return nil
}

func sseBlockIndex(payload map[string]interface{}) (int, bool) {
	raw, ok := payload["index"]
	if !ok {
		return 0, false
	}
	switch value := raw.(type) {
	case float64:
		index := int(value)
		return index, value == float64(index) && index >= 0
	case int:
		return value, value >= 0
	case string:
		index, err := strconv.Atoi(strings.TrimSpace(value))
		return index, err == nil && index >= 0
	default:
		return 0, false
	}
}
