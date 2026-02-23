package model

import (
	"NewAPI-Gateway/common"
	"encoding/json"
	"sort"
	"strings"
)

type providerModelAliasLookup struct {
	direct     map[string]string
	normalized map[string]string
}

type providerModelAliasReverseLookup struct {
	directByTarget     map[string]string
	normalizedByTarget map[string]string
}

func NormalizeProviderAliasMapping(mapping map[string]string) map[string]string {
	normalized := make(map[string]string)
	for source, target := range mapping {
		key := strings.TrimSpace(source)
		value := strings.TrimSpace(target)
		if key == "" || value == "" {
			continue
		}
		normalized[key] = value
	}
	return normalized
}

func ParseProviderAliasMapping(raw string) map[string]string {
	payload := make(map[string]string)
	if strings.TrimSpace(raw) == "" {
		return payload
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return payload
	}
	return NormalizeProviderAliasMapping(parsed)
}

func MarshalProviderAliasMapping(mapping map[string]string) (string, error) {
	clean := NormalizeProviderAliasMapping(mapping)
	if len(clean) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(clean)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func buildProviderModelAliasLookup(mapping map[string]string) providerModelAliasLookup {
	lookup := providerModelAliasLookup{
		direct:     make(map[string]string),
		normalized: make(map[string]string),
	}
	sources := make([]string, 0, len(mapping))
	for source := range mapping {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	for _, source := range sources {
		target := mapping[source]
		key := strings.TrimSpace(source)
		value := strings.TrimSpace(target)
		if key == "" || value == "" {
			continue
		}
		lookup.direct[strings.ToLower(key)] = value
		normalizedKey := common.NormalizeModelName(key)
		if normalizedKey != "" {
			lookup.normalized[normalizedKey] = value
		}
	}
	return lookup
}

func buildProviderModelAliasReverseLookup(mapping map[string]string) providerModelAliasReverseLookup {
	reverse := providerModelAliasReverseLookup{
		directByTarget:     make(map[string]string),
		normalizedByTarget: make(map[string]string),
	}

	sources := make([]string, 0, len(mapping))
	for source := range mapping {
		sources = append(sources, source)
	}
	sort.Strings(sources)

	for _, source := range sources {
		target := mapping[source]
		normalizedSource := strings.TrimSpace(source)
		normalizedTarget := strings.TrimSpace(target)
		if normalizedSource == "" || normalizedTarget == "" {
			continue
		}

		directKey := strings.ToLower(normalizedTarget)
		if _, exists := reverse.directByTarget[directKey]; !exists {
			reverse.directByTarget[directKey] = normalizedSource
		}

		normTarget := common.NormalizeModelName(normalizedTarget)
		if normTarget != "" {
			if _, exists := reverse.normalizedByTarget[normTarget]; !exists {
				reverse.normalizedByTarget[normTarget] = normalizedSource
			}
		}
	}
	return reverse
}

func (l providerModelAliasLookup) Resolve(source string) (string, bool) {
	key := strings.TrimSpace(source)
	if key == "" {
		return "", false
	}
	if target, ok := l.direct[strings.ToLower(key)]; ok && strings.TrimSpace(target) != "" {
		return strings.TrimSpace(target), true
	}
	normalizedKey := common.NormalizeModelName(key)
	if normalizedKey == "" {
		return "", false
	}
	target, ok := l.normalized[normalizedKey]
	if !ok || strings.TrimSpace(target) == "" {
		return "", false
	}
	return strings.TrimSpace(target), true
}

func (l providerModelAliasReverseLookup) ResolveByTarget(target string) (string, bool) {
	raw := strings.TrimSpace(target)
	if raw == "" {
		return "", false
	}
	if source, ok := l.directByTarget[strings.ToLower(raw)]; ok && strings.TrimSpace(source) != "" {
		return strings.TrimSpace(source), true
	}
	normTarget := common.NormalizeModelName(raw)
	if normTarget == "" {
		return "", false
	}
	source, ok := l.normalizedByTarget[normTarget]
	if !ok || strings.TrimSpace(source) == "" {
		return "", false
	}
	return strings.TrimSpace(source), true
}
