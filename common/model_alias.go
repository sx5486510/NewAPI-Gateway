package common

import (
	"regexp"
	"sort"
	"strings"
)

var (
	trailingCompactDatePattern = regexp.MustCompile(`(?i)[-_](20\d{6}|\d{8})$`)
	trailingISODatePattern     = regexp.MustCompile(`(?i)-20\d{2}-\d{2}-\d{2}$`)
	versionSplitPattern        = regexp.MustCompile(`[-_.]+`)
	leadingLabelPattern        = regexp.MustCompile(`^(?:\[[^\]]+\]|【[^】]+】|\([^)]+\)|（[^）]+）|\{[^}]+\}|<[^>]+>)\s*`)
)

// NormalizeModelName canonicalizes vendor/date/latest variations for model routing.
func NormalizeModelName(modelName string) string {
	name := strings.TrimSpace(strings.ToLower(modelName))
	if name == "" {
		return ""
	}
	name = strings.TrimPrefix(name, "bigmodel/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 && idx < len(name)-1 {
		name = name[idx+1:]
	}
	for i := 0; i < 6; i++ {
		next := leadingLabelPattern.ReplaceAllString(name, "")
		if next == name {
			break
		}
		name = strings.TrimSpace(next)
		if name == "" {
			break
		}
	}
	if idx := strings.Index(name, ":"); idx >= 0 {
		name = name[:idx]
	}
	name = trailingCompactDatePattern.ReplaceAllString(name, "")
	name = trailingISODatePattern.ReplaceAllString(name, "")
	name = strings.TrimSuffix(name, "-latest")
	return strings.TrimSpace(name)
}

func ToVersionAgnosticKey(modelName string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(strings.ToLower(modelName)), ".", "-")
	if normalized == "" {
		return ""
	}
	tokens := versionSplitPattern.Split(normalized, -1)
	filtered := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		filtered = append(filtered, token)
	}
	if len(filtered) == 0 {
		return ""
	}
	sort.Strings(filtered)
	return strings.Join(filtered, "-")
}
