package service

import (
	"encoding/json"
	"testing"
)

func decodeNormalizedMessages(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var request map[string]interface{}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatalf("invalid JSON out: %v\n%s", err, body)
	}
	return request
}

func decodeNormalizedMessageList(t *testing.T, body []byte) []map[string]interface{} {
	t.Helper()
	request := decodeNormalizedMessages(t, body)
	items, ok := request["messages"].([]interface{})
	if !ok {
		t.Fatalf("messages is not an array: %T", request["messages"])
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("message is not an object: %T", item)
		}
		out = append(out, m)
	}
	return out
}

func TestNormalizeReasoningContentNoOpCases(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty body", ``},
		{"invalid json", `{"messages":`},
		{"no tools", `{"messages":[{"role":"assistant","reasoning":"think"}]}`},
		{"empty tools", `{"tools":[],"messages":[{"role":"assistant","reasoning":"think"}]}`},
		{"tools null", `{"tools":null,"messages":[{"role":"assistant","reasoning":"think"}]}`},
		{"tools object", `{"tools":{"type":"function"},"messages":[{"role":"assistant","reasoning":"think"}]}`},
		{"messages missing", `{"tools":[{"type":"function"}]}`},
		{"messages not array", `{"tools":[{"type":"function"}],"messages":"oops"}`},
		{"no assistant", `{"tools":[{"type":"function"}],"messages":[{"role":"user","reasoning":"think"}]}`},
		{"already has field", `{"tools":[{"type":"function"}],"messages":[{"role":"assistant","reasoning":"new","reasoning_content":"old"}]}`},
		{"all assistants covered", `{"tools":[{"type":"function"}],"messages":[{"role":"assistant","reasoning":"x","reasoning_content":"x"},{"role":"assistant","reasoning_content":""}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			got := normalizeReasoningContent(body)
			if string(got) != string(body) {
				t.Fatalf("body changed\n got: %s\nwant: %s", got, body)
			}
		})
	}
}

func TestNormalizeReasoningContentRewritesAssistantOnly(t *testing.T) {
	body := []byte(`{
		"enable_thinking":true,
		"model":"deepseek-v4.1-flash",
		"messages":[
			{"role":"system","content":"preset","reasoning":"system-thought"},
			{"role":"user","content":"question","reasoning":"user-thought"},
			{"role":"assistant","content":null,"reasoning":"first thought",
			 "messageId":"m1","traceId":"t1","tool_calls":[{"id":"c1","type":"function","function":{"name":"Bash","arguments":"{}"}}]},
			{"role":"tool","content":"result","tool_call_id":"c1","reasoning":"tool-thought"},
			{"role":"assistant","content":"final answer","reasoning":""}
		],
		"reasoning_effort":"high",
		"stream":true,
		"tools":[{"type":"function","function":{"name":"Bash"}}]
	}`)

	got := normalizeReasoningContent(body)
	request := decodeNormalizedMessages(t, got)
	for _, key := range []string{"enable_thinking", "model", "reasoning_effort", "stream", "tools"} {
		if _, ok := request[key]; !ok {
			t.Fatalf("top-level field %q was dropped", key)
		}
	}
	msgs := decodeNormalizedMessageList(t, got)
	if len(msgs) != 5 {
		t.Fatalf("message count = %d, want 5", len(msgs))
	}
	for _, i := range []int{0, 1, 3} {
		if _, ok := msgs[i]["reasoning_content"]; ok {
			t.Fatalf("message[%d] (role=%v) should not be rewritten", i, msgs[i]["role"])
		}
	}
	if got, ok := msgs[4]["reasoning_content"]; !ok || got != "" {
		t.Fatalf("final assistant with empty reasoning should get an empty string, got %v (present=%v)", got, ok)
	}
	assistant := msgs[2]
	if assistant["reasoning_content"] != "first thought" {
		t.Fatalf("reasoning_content = %v", assistant["reasoning_content"])
	}
	if assistant["reasoning"] != "first thought" {
		t.Fatalf("original reasoning field was changed: %v", assistant["reasoning"])
	}
	if assistant["content"] != nil {
		t.Fatalf("content should stay null, got %v", assistant["content"])
	}
	for _, key := range []string{"messageId", "traceId", "tool_calls"} {
		if _, ok := assistant[key]; !ok {
			t.Fatalf("client metadata %q was dropped", key)
		}
	}
}

func TestNormalizeReasoningContentKeepsExistingFieldValue(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function"}],"messages":[{"role":"assistant","reasoning":"new","reasoning_content":"original"}]}`)
	got := normalizeReasoningContent(body)
	msgs := decodeNormalizedMessageList(t, got)
	if msgs[0]["reasoning_content"] != "original" {
		t.Fatalf("existing value overwritten: %v", msgs[0]["reasoning_content"])
	}
}

func TestNormalizeReasoningContentMultipleAssistantMessages(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function"}],"messages":[
		{"role":"assistant","reasoning":"a"},
		{"role":"assistant","reasoning_content":"kept","reasoning":"b"},
		{"role":"assistant","reasoning":"c"},
		{"role":"assistant","content":"plain"}
	]}`)
	got := normalizeReasoningContent(body)
	msgs := decodeNormalizedMessageList(t, got)
	if msgs[0]["reasoning_content"] != "a" || msgs[2]["reasoning_content"] != "c" {
		t.Fatalf("expected a and c to be added, got %v / %v", msgs[0]["reasoning_content"], msgs[2]["reasoning_content"])
	}
	if msgs[1]["reasoning_content"] != "kept" {
		t.Fatalf("existing value overwritten: %v", msgs[1]["reasoning_content"])
	}
	if got, ok := msgs[3]["reasoning_content"]; !ok || got != "" {
		t.Fatalf("assistant without reasoning should get an empty string, got %v (present=%v)", got, ok)
	}
}

func TestNormalizeReasoningContentPreservesUnicodeAndEscapes(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function"}],"messages":[{"role":"assistant","reasoning":"line1\n中文 <tag> \"quoted\""}]}`)
	got := normalizeReasoningContent(body)
	msgs := decodeNormalizedMessageList(t, got)
	want := "line1\n中文 <tag> \"quoted\""
	if msgs[0]["reasoning_content"] != want || msgs[0]["reasoning"] != want {
		t.Fatalf("value changed: %v / %v", msgs[0]["reasoning_content"], msgs[0]["reasoning"])
	}
}

// Regression for the second production 400: the last assistant of the tool
// loop had no "reasoning" field at all (trace 2331, message [49]), so copying
// only non-empty reasoning values still left it without reasoning_content.
func TestNormalizeReasoningContentBackfillsToolLoopAssistant(t *testing.T) {
	body := []byte(`{
		"enable_thinking":true,
		"model":"deepseek-v4.1-flash",
		"stream":true,
		"tools":[{"type":"function","function":{"name":"Bash"}}],
		"messages":[
			{"role":"system","content":"preset"},
			{"role":"assistant","content":null,"reasoning":"first thought",
			 "tool_calls":[{"id":"c1","type":"function","function":{"name":"Bash","arguments":"{}"}}]},
			{"role":"tool","content":"first result","tool_call_id":"c1"},
			{"role":"assistant","content":"\n\n","messageId":"m49","stepSeq":49,"traceId":"t49",
			 "tool_calls":[{"id":"c2","type":"function","function":{"name":"Bash","arguments":"{}"}}]},
			{"role":"tool","content":"last result","tool_call_id":"c2"}
		]
	}`)
	got := normalizeReasoningContent(body)
	request := decodeNormalizedMessages(t, got)
	if request["stream"] != true || request["enable_thinking"] != true {
		t.Fatal("top-level flags were changed")
	}
	msgs := decodeNormalizedMessageList(t, got)
	if msgs[1]["reasoning_content"] != "first thought" {
		t.Fatalf("reasoning value was not reused: %v", msgs[1]["reasoning_content"])
	}
	last := msgs[3]
	if value, ok := last["reasoning_content"]; !ok || value != "" {
		t.Fatalf("tool-loop assistant must get an empty reasoning_content, got %v (present=%v)", value, ok)
	}
	if _, ok := last["reasoning"]; ok {
		t.Fatal("should not invent a reasoning field")
	}
	for _, key := range []string{"content", "messageId", "stepSeq", "traceId", "tool_calls"} {
		if _, ok := last[key]; !ok {
			t.Fatalf("client metadata %q was dropped", key)
		}
	}
	if last["content"] != "\n\n" {
		t.Fatalf("content changed: %v", last["content"])
	}
	for _, i := range []int{0, 2, 4} {
		if _, ok := msgs[i]["reasoning_content"]; ok {
			t.Fatalf("message[%d] (role=%v) should not be rewritten", i, msgs[i]["role"])
		}
	}
}

func TestNormalizeReasoningContentBackfillsUnusableReasoning(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function"}],"messages":[
		{"role":"assistant","reasoning":"   "},
		{"role":"assistant","reasoning":null},
		{"role":"assistant","reasoning":{"text":"x"}},
		{"role":"assistant"}
	]}`)
	got := normalizeReasoningContent(body)
	msgs := decodeNormalizedMessageList(t, got)
	for i, m := range msgs {
		if value, ok := m["reasoning_content"]; !ok || value != "" {
			t.Fatalf("message[%d] should get an empty reasoning_content, got %v (present=%v)", i, value, ok)
		}
	}
}
