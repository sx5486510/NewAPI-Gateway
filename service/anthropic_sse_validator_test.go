package service

import "testing"

func TestAnthropicSSEValidatorFlagsUnknownContentBlockStop(t *testing.T) {
	validator := NewAnthropicSSEValidator()
	if err := validator.ConsumeLine("event: content_block_start"); err != nil {
		t.Fatalf("start event: %v", err)
	}
	if err := validator.ConsumeLine(`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`); err != nil {
		t.Fatalf("start data: %v", err)
	}
	if err := validator.ConsumeLine("event: content_block_stop"); err != nil {
		t.Fatalf("stop event: %v", err)
	}
	err := validator.ConsumeLine(`data: {"type":"content_block_stop","index":1}`)
	if err == nil || err.Error() != "invalid SSE response: content_block_stop references unopened block index 1" {
		t.Fatalf("unexpected validation result: %v", err)
	}
}

func TestAnthropicSSEValidatorAcceptsValidBlockLifecycle(t *testing.T) {
	validator := NewAnthropicSSEValidator()
	lines := []string{
		"event: content_block_start",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
		"event: content_block_stop",
		`data: {"type":"content_block_stop","index":0}`,
		"event: message_stop",
		`data: {"type":"message_stop"}`,
	}
	for _, line := range lines {
		if err := validator.ConsumeLine(line); err != nil {
			t.Fatalf("ConsumeLine(%q): %v", line, err)
		}
	}
}

func TestAnthropicSSEValidatorFlagsDeltaForUnopenedBlock(t *testing.T) {
	validator := NewAnthropicSSEValidator()
	if err := validator.ConsumeLine("event: content_block_delta"); err != nil {
		t.Fatalf("delta event: %v", err)
	}
	err := validator.ConsumeLine(`data: {"type":"content_block_delta","index":3,"delta":{"type":"text_delta","text":"bad"}}`)
	if err == nil || err.Error() != "invalid SSE response: content_block_delta references unopened block index 3" {
		t.Fatalf("unexpected validation result: %v", err)
	}
}

func TestAnthropicSSEValidatorAcceptsConvertedBytes(t *testing.T) {
	validator := NewAnthropicSSEValidator()
	if err := validator.ConsumeBytes([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")); err != nil {
		t.Fatalf("ConsumeBytes: %v", err)
	}
}

func TestInvalidSSEResponseIsClassifiedForUsageLogs(t *testing.T) {
	_, errorType, _ := extractErrorKeyInfo("client canceled; invalid SSE response: content_block_stop references unopened block index 1")
	if errorType != "INVALID_SSE_RESPONSE" {
		t.Fatalf("error type = %q, want INVALID_SSE_RESPONSE", errorType)
	}
}
