package service

import (
	"NewAPI-Gateway/common"
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

// systemReminderTagPattern matches Claude Code's <system-reminder>...</system-reminder>
// blocks. (?s) makes '.' match newlines so multi-line reminder content is captured.
var systemReminderTagPattern = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)

// stripSystemReminders removes <system-reminder>...</system-reminder> blocks from
// every string value in the JSON request body before it is forwarded upstream.
// Some providers treat this tag as evidence of prompt injection and refuse to
// serve the request, so the gateway strips it regardless of which protocol
// (OpenAI chat completions/responses, Anthropic messages, etc.) the body uses.
//
// json.Unmarshal decodes both the literal "<system-reminder>" form and the
// HTML-escaped "\u003csystem-reminder\u003e" form (produced by some JSON
// encoders) into the same Go string, so a single pattern covers both.
func stripSystemReminders(body []byte) []byte {
	if !common.StripSystemReminderEnabled {
		return body
	}
	if len(body) == 0 || !bytes.Contains(body, []byte("system-reminder")) {
		return body
	}

	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}

	cleaned, changed := stripSystemReminderValue(payload)
	if !changed {
		return body
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(cleaned); err != nil {
		return body
	}
	// json.Encoder.Encode appends a trailing newline that the original
	// single-line body does not have.
	return bytes.TrimRight(buf.Bytes(), "\n")
}

func stripSystemReminderValue(value interface{}) (interface{}, bool) {
	switch v := value.(type) {
	case string:
		if !systemReminderTagPattern.MatchString(v) {
			return v, false
		}
		cleaned := systemReminderTagPattern.ReplaceAllString(v, "")
		if strings.TrimSpace(cleaned) == "" {
			// Avoid leaving a fully empty string behind (some providers
			// reject empty text content blocks).
			cleaned = " "
		}
		return cleaned, true
	case map[string]interface{}:
		changedAny := false
		for k, item := range v {
			newItem, changed := stripSystemReminderValue(item)
			if changed {
				v[k] = newItem
				changedAny = true
			}
		}
		return v, changedAny
	case []interface{}:
		changedAny := false
		for i, item := range v {
			newItem, changed := stripSystemReminderValue(item)
			if changed {
				v[i] = newItem
				changedAny = true
			}
		}
		return v, changedAny
	default:
		return v, false
	}
}