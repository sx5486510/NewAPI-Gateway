package service

import (
	"NewAPI-Gateway/common"
	"encoding/json"
	"testing"
)

func TestStripSystemRemindersLeavesUnrelatedBodyUnchanged(t *testing.T) {
	old := common.StripSystemReminderEnabled
	common.StripSystemReminderEnabled = true
	defer func() { common.StripSystemReminderEnabled = old }()
	body := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`)
	got := stripSystemReminders(body)
	if string(got) != string(body) {
		t.Fatalf("body changed: got %s want %s", got, body)
	}
}

func TestStripSystemRemindersRemovesWholeBlockContent(t *testing.T) {
	old := common.StripSystemReminderEnabled
	common.StripSystemReminderEnabled = true
	defer func() { common.StripSystemReminderEnabled = old }()
	body := []byte(`{"model":"claude-3","messages":[{"role":"user","content":[{"type":"text","text":"<system-reminder>\nsome injected context\n</system-reminder>\n\n"}]}]}`)
	got := stripSystemReminders(body)

	var payload map[string]interface{}
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v, body=%s", err, got)
	}

	messages := payload["messages"].([]interface{})
	msg := messages[0].(map[string]interface{})
	content := msg["content"].([]interface{})
	block := content[0].(map[string]interface{})
	text := block["text"].(string)

	if containsSystemReminder(text) {
		t.Fatalf("system-reminder tag still present: %q", text)
	}
}

func TestStripSystemRemindersRemovesEmbeddedBlockKeepingSurroundingText(t *testing.T) {
	old := common.StripSystemReminderEnabled
	common.StripSystemReminderEnabled = true
	defer func() { common.StripSystemReminderEnabled = old }()
	body := []byte(`{"model":"claude-3","messages":[{"role":"user","content":"before <system-reminder>hidden instructions</system-reminder> after"}]}`)
	got := stripSystemReminders(body)

	var payload map[string]interface{}
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v, body=%s", err, got)
	}

	messages := payload["messages"].([]interface{})
	msg := messages[0].(map[string]interface{})
	text := msg["content"].(string)

	if containsSystemReminder(text) {
		t.Fatalf("system-reminder tag still present: %q", text)
	}
	if !contains(text, "before") || !contains(text, "after") {
		t.Fatalf("surrounding text was unexpectedly removed: %q", text)
	}
}

func TestStripSystemRemindersHandlesHTMLEscapedForm(t *testing.T) {
	old := common.StripSystemReminderEnabled
	common.StripSystemReminderEnabled = true
	defer func() { common.StripSystemReminderEnabled = old }()
	// Simulates a body produced by an encoder with HTML escaping enabled,
	// where json.Unmarshal decodes \u003c/\u003e back into < and >.
	body := []byte(`{"messages":[{"role":"user","content":"tag: \u003csystem-reminder\u003ehidden\u003c/system-reminder\u003e done"}]}`)
	got := stripSystemReminders(body)

	var payload map[string]interface{}
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v, body=%s", err, got)
	}
	messages := payload["messages"].([]interface{})
	msg := messages[0].(map[string]interface{})
	text := msg["content"].(string)
	if containsSystemReminder(text) {
		t.Fatalf("system-reminder tag still present: %q", text)
	}
}

func TestStripSystemRemindersHandlesMultipleBlocks(t *testing.T) {
	old := common.StripSystemReminderEnabled
	common.StripSystemReminderEnabled = true
	defer func() { common.StripSystemReminderEnabled = old }()
	body := []byte(`{"messages":[{"role":"user","content":"a <system-reminder>one</system-reminder> b <system-reminder>two</system-reminder> c"}]}`)
	got := stripSystemReminders(body)

	var payload map[string]interface{}
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v, body=%s", err, got)
	}
	messages := payload["messages"].([]interface{})
	msg := messages[0].(map[string]interface{})
	text := msg["content"].(string)
	if containsSystemReminder(text) {
		t.Fatalf("system-reminder tag still present: %q", text)
	}
}

func TestStripSystemRemindersInvalidJSONReturnsOriginal(t *testing.T) {
	body := []byte(`not json but mentions system-reminder`)
	got := stripSystemReminders(body)
	if string(got) != string(body) {
		t.Fatalf("expected unchanged body for invalid JSON, got %s", got)
	}
}

func TestStripSystemRemindersDisabledReturnsOriginal(t *testing.T) {
	old := common.StripSystemReminderEnabled
	common.StripSystemReminderEnabled = false
	defer func() { common.StripSystemReminderEnabled = old }()

	body := []byte(`{"messages":[{"role":"user","content":"<system-reminder>hidden</system-reminder>"}]}`)
	got := stripSystemReminders(body)
	if string(got) != string(body) {
		t.Fatalf("expected body unchanged when disabled: got %s want %s", got, body)
	}
}

func containsSystemReminder(s string) bool {
	return contains(s, "system-reminder")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}