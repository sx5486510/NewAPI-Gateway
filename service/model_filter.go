package service

import (
	"strings"
)

// ParseModelBlocklist parses the raw RouteModelBlocklist option value into
// normalized patterns. Patterns are separated by newlines or commas,
// trimmed, lowercased, and de-duplicated. Empty entries are dropped.
// An empty input yields an empty slice, which means "no filtering".
func ParseModelBlocklist(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})

	patterns := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		pattern := strings.ToLower(strings.TrimSpace(field))
		if pattern == "" || seen[pattern] {
			continue
		}
		seen[pattern] = true
		patterns = append(patterns, pattern)
	}
	return patterns
}

// IsModelBlocked reports whether modelName matches any blocklist pattern.
// A pattern matches if it equals the (lowercased) model name exactly, or if
// it contains '*' wildcards and the name matches the resulting glob, where
// each '*' matches any (possibly empty) sequence of characters.
// Patterns are lowercased defensively, so un-normalized input works too.
func IsModelBlocked(patterns []string, modelName string) bool {
	if len(patterns) == 0 {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(modelName))
	if name == "" {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if matchBlocklistPattern(pattern, name) {
			return true
		}
	}
	return false
}

// BuildModelAliasSources builds a lookup from alias target (the original
// model name) to the alias sources that resolve to it. Target names are
// lowercased for case-insensitive lookup against route model names.
func BuildModelAliasSources(mapping map[string]string) map[string][]string {
	sources := make(map[string][]string)
	for source, target := range mapping {
		source = strings.ToLower(strings.TrimSpace(source))
		target = strings.ToLower(strings.TrimSpace(target))
		if source == "" || target == "" {
			continue
		}
		sources[target] = append(sources[target], source)
	}
	return sources
}

// IsModelBlockedWithAliases reports whether modelName matches the blocklist
// either directly or through an alias: when an alias source that resolves to
// modelName is blocked, the underlying route is blocked as well. aliasSources
// comes from BuildModelAliasSources and may be nil.
func IsModelBlockedWithAliases(patterns []string, modelName string, aliasSources map[string][]string) bool {
	if IsModelBlocked(patterns, modelName) {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(modelName))
	if name == "" {
		return false
	}
	for _, alias := range aliasSources[name] {
		if IsModelBlocked(patterns, alias) {
			return true
		}
	}
	return false
}

// matchBlocklistPattern matches name against pattern containing '*' wildcards.
func matchBlocklistPattern(pattern string, name string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == name
	}
	return globMatch(pattern, name)
}

// globMatch implements a simple '*' wildcard matcher.
// It supports '*' (any sequence of characters) but not '?' or character
// classes, which are treated as literal characters.
func globMatch(pattern string, name string) bool {
	parts := strings.Split(pattern, "*")
	// The name must start with the first literal part...
	if !strings.HasPrefix(name, parts[0]) {
		return false
	}
	// ...and end with the last literal part (when it is not consumed by the
	// prefix above in the single-part edge case).
	if len(parts) > 1 && !strings.HasSuffix(name, parts[len(parts)-1]) {
		return false
	}
	// Middle parts must appear in order, without overlapping each other.
	pos := len(parts[0])
	for _, part := range parts[1 : len(parts)-1] {
		if part == "" {
			continue
		}
		idx := strings.Index(name[pos:], part)
		if idx < 0 {
			return false
		}
		pos += idx + len(part)
	}
	// Ensure the suffix does not overlap the already consumed prefix.
	if len(parts) > 1 {
		suffixLen := len(parts[len(parts)-1])
		if len(name)-suffixLen < pos {
			return false
		}
	}
	return true
}
