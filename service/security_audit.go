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

// AuditLLMContent performs security audit on request/response content
func AuditLLMContent(requestBody, responseBody string) SecurityAuditResult {
	result := SecurityAuditResult{
		RiskLevel: "safe",
		RiskTags:  []string{},
	}

	// Parse request and response as JSON to extract text content
	requestText := extractTextFromJSON(requestBody)
	responseText := extractTextFromJSON(responseBody)

	// Run all detection rules
	checkPromptInjection(&result, requestText, responseText)
	checkDangerousActions(&result, requestText, responseText)
	checkSensitiveData(&result, requestText, responseText)
	checkAbnormalToolCalls(&result, requestText, responseText)

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

// checkPromptInjection detects prompt injection and jailbreak attempts
func checkPromptInjection(result *SecurityAuditResult, request, response string) {
	combined := strings.ToLower(request + "\n" + response)

	// Jailbreak keywords
	jailbreakPatterns := []string{
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
	}

	for _, pattern := range jailbreakPatterns {
		if strings.Contains(combined, strings.ToLower(pattern)) {
			result.RiskTags = append(result.RiskTags, "prompt_injection")
			updateRiskLevel(result, "high")
			return
		}
	}

	// Instruction override patterns
	overrideRegex := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(override|bypass|disable|turn off)\s+(safety|filter|restriction|guideline)`),
		regexp.MustCompile(`(?i)(你现在|now you are|you're now)\s+(不受|unrestricted|without)`),
	}

	for _, re := range overrideRegex {
		if re.MatchString(combined) {
			result.RiskTags = append(result.RiskTags, "instruction_override")
			updateRiskLevel(result, "high")
			return
		}
	}
}

// checkDangerousActions detects dangerous operations in tool calls or responses
func checkDangerousActions(result *SecurityAuditResult, request, response string) {
	combined := strings.ToLower(request + "\n" + response)

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

// checkSensitiveData detects sensitive information leakage
func checkSensitiveData(result *SecurityAuditResult, request, response string) {
	combined := request + "\n" + response

	// API Keys and tokens (common patterns)
	apiKeyPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(api[_-]?key|apikey|access[_-]?key)\s*[:=]\s*['"]?([a-zA-Z0-9_\-]{20,})`),
		regexp.MustCompile(`(?i)(secret|password|passwd|pwd)\s*[:=]\s*['"]?([^\s'"]{8,})`),
		regexp.MustCompile(`\bsk-[a-zA-Z0-9\-_]{20,}`),                // OpenAI API key pattern
		regexp.MustCompile(`\bghp_[a-zA-Z0-9]{36,}`),                   // GitHub personal access token
		regexp.MustCompile(`\bglpat-[a-zA-Z0-9_\-]{20,}`),              // GitLab token
		regexp.MustCompile(`\bxox[baprs]-[a-zA-Z0-9\-]{10,}`),          // Slack token
		regexp.MustCompile(`\bAIza[a-zA-Z0-9_\-]{35}`),                 // Google API key
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}`),                       // AWS access key
		regexp.MustCompile(`\bya29\.[a-zA-Z0-9_\-]{100,}`),             // Google OAuth token
		regexp.MustCompile(`\beyJ[a-zA-Z0-9_\-]*\.eyJ[a-zA-Z0-9_\-]*`), // JWT token
	}

	for _, re := range apiKeyPatterns {
		if re.MatchString(combined) {
			result.RiskTags = append(result.RiskTags, "api_key_leak")
			updateRiskLevel(result, "critical")
			break
		}
	}

	// Credit card numbers (basic Luhn check pattern)
	ccPattern := regexp.MustCompile(`\b(?:\d{4}[\s\-]?){3}\d{4}\b`)
	if ccPattern.MatchString(combined) {
		result.RiskTags = append(result.RiskTags, "credit_card")
		updateRiskLevel(result, "critical")
	}

	// Email addresses (potential PII)
	emailPattern := regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	emails := emailPattern.FindAllString(combined, -1)
	if len(emails) > 5 { // Multiple emails might indicate data dump
		result.RiskTags = append(result.RiskTags, "multiple_emails")
		updateRiskLevel(result, "medium")
	}

	// Phone numbers - improved detection to avoid false positives
	phones := detectPhoneNumbers(combined)
	if len(phones) > 5 {
		result.RiskTags = append(result.RiskTags, "multiple_phones")
		updateRiskLevel(result, "medium")
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

	// Chinese ID card (simplified check)
	idCardPattern := regexp.MustCompile(`\b[1-9]\d{5}(18|19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`)
	if idCardPattern.MatchString(combined) {
		result.RiskTags = append(result.RiskTags, "id_card_number")
		updateRiskLevel(result, "high")
	}
}

// checkAbnormalToolCalls detects suspicious tool usage patterns
func checkAbnormalToolCalls(result *SecurityAuditResult, request, response string) {
	combined := strings.ToLower(request + "\n" + response)

	// Look for tool_calls or function_call in JSON
	var requestData map[string]interface{}
	var responseData map[string]interface{}

	json.Unmarshal([]byte(request), &requestData)
	json.Unmarshal([]byte(response), &responseData)

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

// detectPhoneNumbers detects real phone numbers while avoiding false positives
func detectPhoneNumbers(text string) []string {
	var phones []string

	// Pre-filter: remove markdown code blocks and inline code
	text = removeCodeBlocks(text)

	// Exclude patterns that look like technical notation
	excludePatterns := []*regexp.Regexp{
		regexp.MustCompile(`\(.*?:.*?\)`),           // (index:id), (key:value)
		regexp.MustCompile(`\[.*?\]\(.*?\)`),        // [text](link) markdown links
		regexp.MustCompile(`\{.*?:.*?\}`),           // {key:value} JSON
		regexp.MustCompile(`<.*?:.*?>`),             // <tag:attr> XML/HTML
		regexp.MustCompile(`\w+:\w+`),               // protocol:value, format:spec
		regexp.MustCompile(`\(\d+[,\.]\d+\)`),       // (1.5), (10,20) coordinates/tuples
	}

	// Apply exclusions
	for _, re := range excludePatterns {
		text = re.ReplaceAllString(text, "")
	}

	// Real phone number patterns
	phonePatterns := []*regexp.Regexp{
		// Chinese mobile: 1[3-9]xxxxxxxxx
		regexp.MustCompile(`\b1[3-9]\d{9}\b`),
		// Chinese with country code: +86 1xxxxxxxxxx or 0086 1xxxxxxxxxx
		regexp.MustCompile(`\+86\s?1[3-9]\d{9}\b`),
		regexp.MustCompile(`0086\s?1[3-9]\d{9}\b`),
		// Formatted: 138-xxxx-xxxx or 138 xxxx xxxx
		regexp.MustCompile(`\b1[3-9]\d[\s\-]\d{4}[\s\-]\d{4}\b`),
		// International format with parentheses: +1 (xxx) xxx-xxxx
		regexp.MustCompile(`\+\d{1,3}\s?\(\d{3}\)\s?\d{3}[\s\-]?\d{4}\b`),
	}

	seen := make(map[string]bool)
	for _, re := range phonePatterns {
		matches := re.FindAllString(text, -1)
		for _, match := range matches {
			// Additional validation: must be mostly digits
			digitCount := 0
			for _, ch := range match {
				if ch >= '0' && ch <= '9' {
					digitCount++
				}
			}
			// At least 10 digits for a valid phone number
			if digitCount >= 10 && !seen[match] {
				phones = append(phones, match)
				seen[match] = true
			}
		}
	}

	return phones
}

// removeCodeBlocks removes markdown code blocks and inline code
func removeCodeBlocks(text string) string {
	// Remove code blocks: ```...``` (must be done first, before inline)
	re2 := regexp.MustCompile("(?s)```[^`]*```")
	text = re2.ReplaceAllString(text, "")

	// Remove inline code: `...`
	re1 := regexp.MustCompile("`[^`]+`")
	text = re1.ReplaceAllString(text, "")

	return text
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
