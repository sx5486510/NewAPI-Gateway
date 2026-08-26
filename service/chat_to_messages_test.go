package service

import (
	"encoding/json"
	"testing"
)

func TestConvertChatResponseTextOnly(t *testing.T) {
	out, err := ConvertChatResponseToMessages([]byte(`{
		"id":"chatcmpl-1",
		"model":"gpt-4",
		"choices":[{"finish_reason":"stop","message":{"content":"hello there"}}],
		"usage":{"prompt_tokens":5,"completion_tokens":3}
	}`))
	if err != nil {
		t.Fatalf("ConvertChatResponseToMessages: %v", err)
	}
	var resp anthropicMessageResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Type != "message" || resp.Role != "assistant" || resp.Model != "gpt-4" {
		t.Fatalf("unexpected envelope: %#v", resp)
	}
	if resp.StopReason != "end_turn" {
		t.Fatalf("unexpected stop_reason: %s", resp.StopReason)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("unexpected usage: %#v", resp.Usage)
	}
	if len(resp.Content) != 1 || resp.Content[0]["type"] != "text" || resp.Content[0]["text"] != "hello there" {
		t.Fatalf("unexpected content: %#v", resp.Content)
	}
}

func TestConvertChatResponseToolCallsPreserveId(t *testing.T) {
	out, err := ConvertChatResponseToMessages([]byte(`{
		"id":"chatcmpl-2",
		"model":"gpt-4",
		"choices":[{"finish_reason":"tool_calls","message":{"content":null,"tool_calls":[
			{"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"NYC\"}"}}
		]}}]
	}`))
	if err != nil {
		t.Fatalf("ConvertChatResponseToMessages: %v", err)
	}
	var resp anthropicMessageResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.StopReason != "tool_use" {
		t.Fatalf("unexpected stop_reason: %s", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %#v", resp.Content)
	}
	block := resp.Content[0]
	if block["type"] != "tool_use" || block["id"] != "call_abc" || block["name"] != "get_weather" {
		t.Fatalf("unexpected tool_use block: %#v", block)
	}
	input, ok := block["input"].(map[string]interface{})
	if !ok || input["city"] != "NYC" {
		t.Fatalf("unexpected tool_use input: %#v", block["input"])
	}
}

func TestConvertChatResponseReasoningContentBecomesThinking(t *testing.T) {
	out, err := ConvertChatResponseToMessages([]byte(`{
		"id":"chatcmpl-3",
		"model":"gpt-4",
		"choices":[{"finish_reason":"stop","message":{"content":"answer","reasoning_content":"deep thought"}}]
	}`))
	if err != nil {
		t.Fatalf("ConvertChatResponseToMessages: %v", err)
	}
	var resp anthropicMessageResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("expected thinking + text blocks, got %#v", resp.Content)
	}
	if resp.Content[0]["type"] != "thinking" || resp.Content[0]["thinking"] != "deep thought" {
		t.Fatalf("unexpected thinking block: %#v", resp.Content[0])
	}
	if resp.Content[1]["type"] != "text" || resp.Content[1]["text"] != "answer" {
		t.Fatalf("unexpected text block: %#v", resp.Content[1])
	}
}

func TestConvertChatResponseNoReasoningContentNoFakeThinking(t *testing.T) {
	out, err := ConvertChatResponseToMessages([]byte(`{
		"id":"chatcmpl-4",
		"model":"gpt-4",
		"choices":[{"finish_reason":"stop","message":{"content":"answer"}}]
	}`))
	if err != nil {
		t.Fatalf("ConvertChatResponseToMessages: %v", err)
	}
	var resp anthropicMessageResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	for _, block := range resp.Content {
		if block["type"] == "thinking" {
			t.Fatalf("did not expect fabricated thinking block: %#v", resp.Content)
		}
	}
}

func TestConvertChatResponseMalformedReturnsError(t *testing.T) {
	if _, err := ConvertChatResponseToMessages([]byte(`{"choices":[]}`)); err == nil {
		t.Fatal("expected error for response with no choices")
	}
	if _, err := ConvertChatResponseToMessages([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestConvertChatErrorToMessagesErrorMapsStatusCodes(t *testing.T) {
	cases := []struct {
		status   int
		wantType string
	}{
		{400, "invalid_request_error"},
		{401, "authentication_error"},
		{403, "permission_error"},
		{404, "not_found_error"},
		{408, "timeout_error"},
		{429, "rate_limit_error"},
		{500, "api_error"},
		{529, "overloaded_error"},
	}
	for _, tc := range cases {
		body := ConvertChatErrorToMessagesError(tc.status, upstreamErrorInfo{Message: "boom"})
		var parsed map[string]interface{}
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatalf("status %d: unmarshal: %v", tc.status, err)
		}
		if parsed["type"] != "error" {
			t.Fatalf("status %d: expected type=error, got %#v", tc.status, parsed["type"])
		}
		errObj := parsed["error"].(map[string]interface{})
		if errObj["type"] != tc.wantType {
			t.Fatalf("status %d: expected error type %s, got %#v", tc.status, tc.wantType, errObj["type"])
		}
		if errObj["message"] != "boom" {
			t.Fatalf("status %d: expected message boom, got %#v", tc.status, errObj["message"])
		}
	}
}
