package service

import (
	"regexp"
	"strings"
	"testing"
)

var sseEventNamePattern = regexp.MustCompile(`event: (\S+)`)

func extractEventNames(raw string) []string {
	matches := sseEventNamePattern.FindAllStringSubmatch(raw, -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	return names
}

func TestMessagesSSEConverterFullTextLifecycle(t *testing.T) {
	conv := NewMessagesSSEConverter("gpt-4")
	var out strings.Builder

	lines := []string{
		`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"delta":{"role":"assistant"}}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"delta":{"content":"Hello"}}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"delta":{"content":" world"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`,
		`data: [DONE]`,
	}
	for _, line := range lines {
		chunk, err := conv.ConvertLine(line)
		if err != nil {
			t.Fatalf("ConvertLine(%q): %v", line, err)
		}
		out.Write(chunk)
	}

	got := extractEventNames(out.String())
	want := []string{"message_start", "content_block_start", "content_block_delta", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}
	if len(got) != len(want) {
		t.Fatalf("event sequence = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event[%d] = %s, want %s (full: %v)", i, got[i], want[i], got)
		}
	}
	if !strings.Contains(out.String(), `"text":"Hello"`) && !strings.Contains(out.String(), `"text_delta"`) {
		t.Fatalf("expected text delta events in output: %s", out.String())
	}
	if !strings.Contains(out.String(), `"stop_reason":"end_turn"`) {
		t.Fatalf("expected end_turn stop reason: %s", out.String())
	}
}

func TestMessagesSSEConverterToolCallLifecycle(t *testing.T) {
	conv := NewMessagesSSEConverter("gpt-4")
	var out strings.Builder

	lines := []string{
		`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"NYC\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}
	for _, line := range lines {
		chunk, err := conv.ConvertLine(line)
		if err != nil {
			t.Fatalf("ConvertLine(%q): %v", line, err)
		}
		out.Write(chunk)
	}

	full := out.String()
	got := extractEventNames(full)
	want := []string{"message_start", "content_block_start", "content_block_delta", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}
	if len(got) != len(want) {
		t.Fatalf("event sequence = %v, want %v (full: %s)", got, want, full)
	}
	if !strings.Contains(full, `"id":"call_1"`) {
		t.Fatalf("tool call id not preserved: %s", full)
	}
	if !strings.Contains(full, `"stop_reason":"tool_use"`) {
		t.Fatalf("expected tool_use stop reason: %s", full)
	}
}

func TestMessagesSSEConverterReasoningContentThinkingBlock(t *testing.T) {
	conv := NewMessagesSSEConverter("gpt-4")
	var out strings.Builder

	lines := []string{
		`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"delta":{"reasoning_content":"pondering"}}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"delta":{"content":"answer"},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}
	for _, line := range lines {
		chunk, err := conv.ConvertLine(line)
		if err != nil {
			t.Fatalf("ConvertLine(%q): %v", line, err)
		}
		out.Write(chunk)
	}

	full := out.String()
	got := extractEventNames(full)
	want := []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}
	if len(got) != len(want) {
		t.Fatalf("event sequence = %v, want %v (full: %s)", got, want, full)
	}
	if !strings.Contains(full, `"thinking_delta"`) {
		t.Fatalf("expected thinking_delta event: %s", full)
	}
}

func TestMessagesSSEConverterErrorBeforeFirstEventAllowsRetry(t *testing.T) {
	conv := NewMessagesSSEConverter("gpt-4")
	_, err := conv.ConvertLine(`data: {"error":{"message":"boom","type":"server_error"}}`)
	if err == nil {
		t.Fatal("expected error from inline error chunk")
	}
	if conv.HasEmittedFirstEvent() {
		t.Fatal("expected no events emitted before failure")
	}
}

func TestMessagesSSEConverterErrorAfterFirstEventMustNotRetry(t *testing.T) {
	conv := NewMessagesSSEConverter("gpt-4")
	if _, err := conv.ConvertLine(`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"delta":{"content":"hi"}}]}`); err != nil {
		t.Fatalf("first chunk: %v", err)
	}
	if !conv.HasEmittedFirstEvent() {
		t.Fatal("expected first event to be recorded as emitted")
	}
	if _, err := conv.ConvertLine(`data: {"error":{"message":"boom"}}`); err == nil {
		t.Fatal("expected error from inline error chunk")
	}
	if !conv.HasEmittedFirstEvent() {
		t.Fatal("first-event flag must remain true after mid-stream failure")
	}
}

func TestMessagesSSEConverterFinalizeIsIdempotent(t *testing.T) {
	conv := NewMessagesSSEConverter("gpt-4")
	if _, err := conv.ConvertLine(`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"delta":{"content":"hi"},"finish_reason":"stop"}]}`); err != nil {
		t.Fatalf("chunk: %v", err)
	}
	first, err := conv.Finalize()
	if err != nil {
		t.Fatalf("first finalize: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("expected finalize to emit message_delta/message_stop")
	}
	second, err := conv.Finalize()
	if err != nil {
		t.Fatalf("second finalize: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("expected idempotent finalize to emit nothing, got: %s", second)
	}
}
