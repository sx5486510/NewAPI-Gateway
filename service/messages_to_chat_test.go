package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustConvertRequest(t *testing.T, body string) map[string]interface{} {
	t.Helper()
	out, err := ConvertAnthropicRequestToChat([]byte(body))
	if err != nil {
		t.Fatalf("ConvertAnthropicRequestToChat: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal converted body: %v", err)
	}
	return parsed
}

func TestConvertRequestSystemStringAndTextBlock(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"system":"be nice",
		"messages":[{"role":"user","content":"hi"}]
	}`)
	messages, ok := got["messages"].([]interface{})
	if !ok || len(messages) != 2 {
		t.Fatalf("expected system + user message, got %#v", got["messages"])
	}
	first := messages[0].(map[string]interface{})
	if first["role"] != "system" || first["content"] != "be nice" {
		t.Fatalf("unexpected system message: %#v", first)
	}
}

func TestConvertRequestSystemTextBlockArray(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"system":[{"type":"text","text":"part1"},{"type":"text","text":"part2"}],
		"messages":[{"role":"user","content":"hi"}]
	}`)
	messages := got["messages"].([]interface{})
	first := messages[0].(map[string]interface{})
	if first["content"] != "part1\n\npart2" {
		t.Fatalf("unexpected joined system text: %#v", first["content"])
	}
}

func TestConvertRequestSystemRoleMessageInArray(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"messages":[
			{"role":"system","content":"be nice"},
			{"role":"user","content":"hi"}
		]
	}`)
	messages, ok := got["messages"].([]interface{})
	if !ok || len(messages) != 2 {
		t.Fatalf("expected system + user message, got %#v", got["messages"])
	}
	first := messages[0].(map[string]interface{})
	if first["role"] != "system" || first["content"] != "be nice" {
		t.Fatalf("unexpected system message: %#v", first)
	}
	second := messages[1].(map[string]interface{})
	if second["role"] != "user" || second["content"] != "hi" {
		t.Fatalf("unexpected user message: %#v", second)
	}
}

func TestConvertRequestSystemRoleMessageContentBlockArray(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"messages":[
			{"role":"system","content":[{"type":"text","text":"part1"},{"type":"text","text":"part2"}]},
			{"role":"user","content":"hi"}
		]
	}`)
	messages := got["messages"].([]interface{})
	first := messages[0].(map[string]interface{})
	if first["content"] != "part1\n\npart2" {
		t.Fatalf("unexpected joined system text: %#v", first["content"])
	}
}

func TestConvertRequestUserTextBlockArray(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]
	}`)
	messages := got["messages"].([]interface{})
	msg := messages[0].(map[string]interface{})
	if msg["content"] != "hello" {
		t.Fatalf("unexpected content: %#v", msg["content"])
	}
}

func TestConvertRequestImageBlock(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"messages":[{"role":"user","content":[
			{"type":"text","text":"look"},
			{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}
		]}]
	}`)
	messages := got["messages"].([]interface{})
	msg := messages[0].(map[string]interface{})
	parts, ok := msg["content"].([]interface{})
	if !ok || len(parts) != 2 {
		t.Fatalf("expected 2 content parts, got %#v", msg["content"])
	}
	imgPart := parts[1].(map[string]interface{})
	if imgPart["type"] != "image_url" {
		t.Fatalf("unexpected image part: %#v", imgPart)
	}
	imageURL := imgPart["image_url"].(map[string]interface{})
	if imageURL["url"] != "data:image/png;base64,AAAA" {
		t.Fatalf("unexpected image url: %#v", imageURL["url"])
	}
}

func TestConvertRequestToolsAndToolChoiceAuto(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"messages":[{"role":"user","content":"hi"}],
		"tools":[{"name":"get_weather","description":"gw","input_schema":{"type":"object"}}],
		"tool_choice":{"type":"auto"}
	}`)
	tools := got["tools"].([]interface{})
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %#v", tools)
	}
	tool := tools[0].(map[string]interface{})
	if tool["type"] != "function" {
		t.Fatalf("unexpected tool type: %#v", tool)
	}
	fn := tool["function"].(map[string]interface{})
	if fn["name"] != "get_weather" {
		t.Fatalf("unexpected function name: %#v", fn)
	}
	if got["tool_choice"] != "auto" {
		t.Fatalf("expected tool_choice auto, got %#v", got["tool_choice"])
	}
}

func TestConvertRequestToolChoiceAnyMapsToRequired(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"messages":[{"role":"user","content":"hi"}],
		"tool_choice":{"type":"any"}
	}`)
	if got["tool_choice"] != "required" {
		t.Fatalf("expected tool_choice required, got %#v", got["tool_choice"])
	}
}

func TestConvertRequestToolChoiceSpecificTool(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"messages":[{"role":"user","content":"hi"}],
		"tool_choice":{"type":"tool","name":"get_weather"}
	}`)
	choice, ok := got["tool_choice"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected object tool_choice, got %#v", got["tool_choice"])
	}
	if choice["type"] != "function" {
		t.Fatalf("unexpected tool_choice type: %#v", choice)
	}
	fn := choice["function"].(map[string]interface{})
	if fn["name"] != "get_weather" {
		t.Fatalf("unexpected tool_choice function: %#v", fn)
	}
}

func TestConvertRequestToolChoiceInvalidReturnsError(t *testing.T) {
	_, err := ConvertAnthropicRequestToChat([]byte(`{
		"model":"claude-3",
		"messages":[{"role":"user","content":"hi"}],
		"tool_choice":{"type":"bogus"}
	}`))
	if err == nil {
		t.Fatal("expected error for unsupported tool_choice type")
	}
}

func TestConvertRequestToolUseAndToolResultPreserveId(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"messages":[
			{"role":"user","content":"what's the weather"},
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"NYC"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"sunny"}]}
		]
	}`)
	messages := got["messages"].([]interface{})
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d: %#v", len(messages), messages)
	}
	assistantMsg := messages[1].(map[string]interface{})
	toolCalls, ok := assistantMsg["tool_calls"].([]interface{})
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %#v", assistantMsg["tool_calls"])
	}
	tc := toolCalls[0].(map[string]interface{})
	if tc["id"] != "toolu_1" {
		t.Fatalf("tool call id not preserved: %#v", tc)
	}
	fn := tc["function"].(map[string]interface{})
	if fn["name"] != "get_weather" {
		t.Fatalf("unexpected function name: %#v", fn)
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(fn["arguments"].(string)), &args); err != nil {
		t.Fatalf("arguments not valid JSON: %v", err)
	}
	if args["city"] != "NYC" {
		t.Fatalf("unexpected arguments: %#v", args)
	}

	toolMsg := messages[2].(map[string]interface{})
	if toolMsg["role"] != "tool" || toolMsg["tool_call_id"] != "toolu_1" || toolMsg["content"] != "sunny" {
		t.Fatalf("unexpected tool result message: %#v", toolMsg)
	}
}

func TestConvertRequestToolResultIsErrorPrefixesMessage(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"messages":[
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"boom","is_error":true}]}
		]
	}`)
	messages := got["messages"].([]interface{})
	toolMsg := messages[0].(map[string]interface{})
	if !strings.HasPrefix(toolMsg["content"].(string), "Error: ") {
		t.Fatalf("expected error-prefixed content, got %#v", toolMsg["content"])
	}
}

func TestConvertRequestThinkingBecomesTaggedText(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"pondering"}]}]
	}`)
	messages := got["messages"].([]interface{})
	msg := messages[0].(map[string]interface{})
	if msg["content"] != "<thinking>pondering</thinking>" {
		t.Fatalf("unexpected thinking text: %#v", msg["content"])
	}
}

func TestConvertRequestRedactedThinkingPreservedRaw(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","data":"opaque-blob"}]}]
	}`)
	messages := got["messages"].([]interface{})
	msg := messages[0].(map[string]interface{})
	if msg["content"] != "<redacted_thinking>opaque-blob</redacted_thinking>" {
		t.Fatalf("unexpected redacted thinking text: %#v", msg["content"])
	}
}

func TestConvertRequestUnknownContentBlockReturnsError(t *testing.T) {
	_, err := ConvertAnthropicRequestToChat([]byte(`{
		"model":"claude-3",
		"messages":[{"role":"user","content":[{"type":"mystery_block"}]}]
	}`))
	if err == nil {
		t.Fatal("expected error for unknown content block")
	}
	var convErr *MessagesConversionError
	if !asMessagesConversionError(err, &convErr) {
		t.Fatalf("expected *MessagesConversionError, got %T: %v", err, err)
	}
}

func asMessagesConversionError(err error, target **MessagesConversionError) bool {
	if convErr, ok := err.(*MessagesConversionError); ok {
		*target = convErr
		return true
	}
	return false
}

func TestConvertRequestPassesThroughSamplingParams(t *testing.T) {
	got := mustConvertRequest(t, `{
		"model":"claude-3",
		"max_tokens":128,
		"temperature":0.5,
		"top_p":0.9,
		"stop_sequences":["END"],
		"stream":true,
		"messages":[{"role":"user","content":"hi"}]
	}`)
	if got["max_tokens"] != float64(128) {
		t.Fatalf("max_tokens not preserved: %#v", got["max_tokens"])
	}
	if got["temperature"] != 0.5 {
		t.Fatalf("temperature not preserved: %#v", got["temperature"])
	}
	if got["top_p"] != 0.9 {
		t.Fatalf("top_p not preserved: %#v", got["top_p"])
	}
	stop, ok := got["stop"].([]interface{})
	if !ok || len(stop) != 1 || stop[0] != "END" {
		t.Fatalf("stop_sequences not mapped to stop: %#v", got["stop"])
	}
	if got["stream"] != true {
		t.Fatalf("stream not preserved: %#v", got["stream"])
	}
}
