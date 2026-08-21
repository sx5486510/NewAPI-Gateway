package service

import (
	"encoding/json"
	"regexp"
	"strings"
)

// SecurityAuditResult represents the result of security audit
type SecurityAuditResult struct {
	RiskLevel string   `json:"risk_level"` // safe, low, medium, high, critical
	RiskTags  []string `json:"risk_tags"`
}

// multipleMatchThreshold is the number of email/phone hits above which the
// content is flagged as a possible data dump.
const multipleMatchThreshold = 5

// gatedRegexp pairs a pattern with the literals any match of it must contain.
//
// Scanning a megabyte of text with one regexp costs roughly 40ms, while
// strings.Contains over the same text costs well under a millisecond. Most rules
// here can only match adjacent to a fixed literal, so checking for that literal
// first turns the common "no hit anywhere" case from a scan into a memchr.
//
// gates is an AND of ORs: every inner slice is one required alternation and is
// satisfied by any of its literals. A missing gate is *proof* the pattern cannot
// match, so skipping the scan is exact rather than a heuristic that might drop a
// real finding.
type gatedRegexp struct {
	re    *regexp.Regexp
	gates [][]string
}

// gatesPass reports whether haystack clears every gate.
//
// Gate literals are written lowercase, so haystack must be the lowercased text
// whenever the gates contain letters: that lets one gate serve both the (?i)
// patterns and the case-sensitive ones (an "AKIA" in the original still shows up
// as "akia" once lowered, so the condition stays necessary). Gates made only of
// digits and punctuation are unaffected by case and may be checked against the
// raw text.
func (g gatedRegexp) gatesPass(haystack string) bool {
	for _, alternation := range g.gates {
		if !containsAny(haystack, alternation) {
			return false
		}
	}
	return true
}

// matchString reports whether the pattern matches haystack, consulting the gates
// against gateHaystack first. Callers whose haystack is already lowercased pass
// it as both arguments.
func (g gatedRegexp) matchString(haystack, gateHaystack string) bool {
	return g.gatesPass(gateHaystack) && g.re.MatchString(haystack)
}

func containsAny(haystack string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

// unmarshalJSONObject decodes s into a JSON object, returning nil when s is not
// one. The cheap prefix check keeps a multi-megabyte non-JSON haystack from
// being copied into a []byte just for Unmarshal to reject it.
func unmarshalJSONObject(s string) map[string]interface{} {
	if !strings.HasPrefix(strings.TrimLeft(s, " \t\r\n"), "{") {
		return nil
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return nil
	}
	return data
}

// AuditLLMContent performs security audit on request/response content
func AuditLLMContent(requestBody, responseBody string) SecurityAuditResult {
	result := SecurityAuditResult{
		RiskLevel: "safe",
		RiskTags:  []string{},
	}

	// Parse request and response as JSON to extract text content
	requestText := extractTextFromJSON(requestBody)
	responseText := extractTextFromJSON(responseBody)

	// Every rule below scans the same haystack, so build the raw and lowercased
	// forms once instead of re-concatenating (and re-lowering) per rule. On a
	// Claude Code sized payload that alone removes several megabytes of copies.
	combined := requestText + "\n" + responseText
	combinedLower := strings.ToLower(combined)

	// Run all detection rules
	checkPromptInjection(&result, combinedLower)
	checkDangerousActions(&result, combinedLower)
	checkSensitiveData(&result, combined, combinedLower)
	checkAbnormalToolCalls(&result, responseText, combinedLower)

	return result
}

// extractTextFromJSON extracts all text content from JSON for analysis
func extractTextFromJSON(jsonStr string) string {
	if jsonStr == "" {
		return ""
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		// If not valid JSON, return as-is
		return jsonStr
	}

	var texts []string
	extractTextRecursive(data, &texts)
	return strings.Join(texts, "\n")
}

func extractTextRecursive(data interface{}, texts *[]string) {
	switch v := data.(type) {
	case string:
		*texts = append(*texts, v)
	case map[string]interface{}:
		for _, val := range v {
			extractTextRecursive(val, texts)
		}
	case []interface{}:
		for _, val := range v {
			extractTextRecursive(val, texts)
		}
	}
}

// Jailbreak keywords, pre-lowered once so the hot path only does Contains.
var jailbreakPatterns = lowerAll([]string{
	"ignore previous instructions",
	"ignore all previous",
	"disregard previous",
	"forget previous",
	"new instructions",
	"system prompt",
	"you are now",
	"act as if",
	"pretend to be",
	"roleplay as",
	"do anything now",
	"DAN mode",
	"developer mode",
	"god mode",
	"unrestricted mode",
	"jailbreak",
	"越狱",
	"忽略之前",
	"忽略上面",
	"忽略以上",
	"无视规则",
	"现在你是",
	"扮演",
})

// Instruction override patterns. Both alternations are required, so gating on
// each one independently is far more selective than gating on either alone.
var overrideRegexps = []gatedRegexp{
	{
		regexp.MustCompile(`(?i)(override|bypass|disable|turn off)\s+(safety|filter|restriction|guideline)`),
		[][]string{
			{"override", "bypass", "disable", "turn off"},
			{"safety", "filter", "restriction", "guideline"},
		},
	},
	{
		regexp.MustCompile(`(?i)(你现在|now you are|you're now)\s+(不受|unrestricted|without)`),
		[][]string{
			{"你现在", "now you are", "you're now"},
			{"不受", "unrestricted", "without"},
		},
	},
}

func lowerAll(patterns []string) []string {
	lowered := make([]string, len(patterns))
	for i, pattern := range patterns {
		lowered[i] = strings.ToLower(pattern)
	}
	return lowered
}

// checkPromptInjection detects prompt injection and jailbreak attempts.
// combined must already be lowercased.
func checkPromptInjection(result *SecurityAuditResult, combined string) {
	for _, pattern := range jailbreakPatterns {
		if strings.Contains(combined, pattern) {
			result.RiskTags = append(result.RiskTags, "prompt_injection")
			updateRiskLevel(result, "high")
			return
		}
	}

	for _, re := range overrideRegexps {
		if re.matchString(combined, combined) {
			result.RiskTags = append(result.RiskTags, "instruction_override")
			updateRiskLevel(result, "high")
			return
		}
	}
}

// checkDangerousActions detects dangerous operations in tool calls or responses.
// combined must already be lowercased.
func checkDangerousActions(result *SecurityAuditResult, combined string) {
	// File system operations
	dangerousFileOps := []string{
		"rm -rf",
		"rmdir /s",
		"del /f",
		"format c:",
		"dd if=",
		"mkfs.",
		"shred",
		"unlink",
	}

	for _, op := range dangerousFileOps {
		if strings.Contains(combined, op) {
			result.RiskTags = append(result.RiskTags, "dangerous_file_operation")
			updateRiskLevel(result, "critical")
			break
		}
	}

	// Command execution
	dangerousCmds := []string{
		"eval(",
		"exec(",
		"system(",
		"shell_exec",
		"passthru",
		"proc_open",
		"popen",
		"curl -o",
		"wget -o",
		"invoke-expression",
		"iex ",
		"powershell -encodedcommand",
	}

	for _, cmd := range dangerousCmds {
		if strings.Contains(combined, cmd) {
			result.RiskTags = append(result.RiskTags, "command_execution")
			updateRiskLevel(result, "high")
			break
		}
	}

	// Network attacks
	networkPatterns := []string{
		"nmap",
		"nikto",
		"sqlmap",
		"metasploit",
		"burp suite",
		"hydra",
		"port scan",
		"dos attack",
		"ddos",
	}

	for _, pattern := range networkPatterns {
		if strings.Contains(combined, pattern) {
			result.RiskTags = append(result.RiskTags, "network_attack")
			updateRiskLevel(result, "high")
			break
		}
	}

	// Database operations
	sqlPatterns := []string{
		"drop database",
		"drop table",
		"truncate table",
		"delete from",
		"update.*set.*=",
		"; --",
		"union select",
		"' or '1'='1",
	}

	for _, pattern := range sqlPatterns {
		if strings.Contains(combined, pattern) {
			result.RiskTags = append(result.RiskTags, "sql_operation")
			updateRiskLevel(result, "medium")
			break
		}
	}
}

// API Keys and tokens (common patterns)
var apiKeyRegexps = []gatedRegexp{
	// Every alternation of the name group ends in "key".
	{regexp.MustCompile(`(?i)(api[_-]?key|apikey|access[_-]?key)\s*[:=]\s*['"]?([a-zA-Z0-9_\-]{20,})`), [][]string{{"key"}}},
	// "password" and "passwd" share the "pass" prefix.
	{regexp.MustCompile(`(?i)(secret|password|passwd|pwd)\s*[:=]\s*['"]?([^\s'"]{8,})`), [][]string{{"secret", "pass", "pwd"}}},
	{regexp.MustCompile(`\bsk-[a-zA-Z0-9\-_]{20,}`), [][]string{{"sk-"}}},                 // OpenAI API key pattern
	{regexp.MustCompile(`\bghp_[a-zA-Z0-9]{36,}`), [][]string{{"ghp_"}}},                  // GitHub personal access token
	{regexp.MustCompile(`\bglpat-[a-zA-Z0-9_\-]{20,}`), [][]string{{"glpat-"}}},           // GitLab token
	{regexp.MustCompile(`\bxox[baprs]-[a-zA-Z0-9\-]{10,}`), [][]string{{"xox"}}},          // Slack token
	{regexp.MustCompile(`\bAIza[a-zA-Z0-9_\-]{35}`), [][]string{{"aiza"}}},                // Google API key
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}`), [][]string{{"akia"}}},                      // AWS access key
	{regexp.MustCompile(`\bya29\.[a-zA-Z0-9_\-]{100,}`), [][]string{{"ya29."}}},           // Google OAuth token
	{regexp.MustCompile(`\beyJ[a-zA-Z0-9_\-]*\.eyJ[a-zA-Z0-9_\-]*`), [][]string{{"eyj"}}}, // JWT token
}

// Email addresses (potential PII)
var emailRegexp = gatedRegexp{regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`), [][]string{{"@"}}}

// checkSensitiveData detects sensitive information leakage.
// combined must be the raw (case-preserving) text: several rules below are
// case-sensitive. combinedLower is the same text lowercased, used for gating.
func checkSensitiveData(result *SecurityAuditResult, combined, combinedLower string) {
	for _, re := range apiKeyRegexps {
		if re.matchString(combined, combinedLower) {
			result.RiskTags = append(result.RiskTags, "api_key_leak")
			updateRiskLevel(result, "critical")
			break
		}
	}

	// Stop at 6 matches: the only question is whether more than 5 exist, so there
	// is no reason to materialize every address in a large payload.
	if emailRegexp.gatesPass(combined) {
		emails := emailRegexp.re.FindAllString(combined, multipleMatchThreshold+1)
		if len(emails) > multipleMatchThreshold { // Multiple emails might indicate data dump
			result.RiskTags = append(result.RiskTags, "multiple_emails")
			updateRiskLevel(result, "medium")
		}
	}

	// SSH private keys
	if strings.Contains(combined, "BEGIN RSA PRIVATE KEY") ||
		strings.Contains(combined, "BEGIN OPENSSH PRIVATE KEY") ||
		strings.Contains(combined, "BEGIN DSA PRIVATE KEY") ||
		strings.Contains(combined, "BEGIN EC PRIVATE KEY") {
		result.RiskTags = append(result.RiskTags, "private_key_leak")
		updateRiskLevel(result, "critical")
	}

	// Database connection strings
	dbConnPatterns := []string{
		"mysql://",
		"postgresql://",
		"mongodb://",
		"redis://",
		"Server=",
		"Data Source=",
		"Initial Catalog=",
		"User ID=",
	}

	for _, pattern := range dbConnPatterns {
		if strings.Contains(combined, pattern) {
			result.RiskTags = append(result.RiskTags, "db_connection_string")
			updateRiskLevel(result, "high")
			break
		}
	}
}

// checkAbnormalToolCalls detects suspicious tool usage patterns.
// combined must already be lowercased.
func checkAbnormalToolCalls(result *SecurityAuditResult, response, combined string) {
	// Look for tool_calls or function_call in JSON
	responseData := unmarshalJSONObject(response)

	// Check for excessive tool calls
	toolCallCount := 0

	// Check top-level tool_calls (some APIs)
	if tools, ok := responseData["tool_calls"].([]interface{}); ok {
		toolCallCount += len(tools)
	}

	// Check choices[].message.tool_calls (OpenAI format)
	if choices, ok := responseData["choices"].([]interface{}); ok {
		for _, choice := range choices {
			if choiceMap, ok := choice.(map[string]interface{}); ok {
				if message, ok := choiceMap["message"].(map[string]interface{}); ok {
					if tools, ok := message["tool_calls"].([]interface{}); ok {
						toolCallCount += len(tools)
					}
				}
			}
		}
	}

	if toolCallCount > 10 {
		result.RiskTags = append(result.RiskTags, "excessive_tool_calls")
		updateRiskLevel(result, "medium")
	}

	// Check for suspicious tool names
	suspiciousTools := []string{
		"execute_code",
		"run_command",
		"eval_code",
		"system_call",
		"file_delete",
		"file_write",
		"network_request",
		"database_query",
	}

	for _, tool := range suspiciousTools {
		if strings.Contains(combined, tool) {
			result.RiskTags = append(result.RiskTags, "suspicious_tool_call")
			updateRiskLevel(result, "high")
			break
		}
	}

	// Check for repeated failed tool calls
	if strings.Count(combined, "\"error\"") > 5 {
		result.RiskTags = append(result.RiskTags, "repeated_tool_errors")
		updateRiskLevel(result, "low")
	}
}

// updateRiskLevel updates risk level to higher severity if needed
func updateRiskLevel(result *SecurityAuditResult, newLevel string) {
	levels := map[string]int{
		"safe":     0,
		"unknown":  0,
		"low":      1,
		"medium":   2,
		"high":     3,
		"critical": 4,
	}

	currentLevel := levels[result.RiskLevel]
	proposedLevel := levels[newLevel]

	if proposedLevel > currentLevel {
		result.RiskLevel = newLevel
	}
}
