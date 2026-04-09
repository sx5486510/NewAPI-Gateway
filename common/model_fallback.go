package common

import (
	"encoding/json"
	"strings"
)

type ModelFallbackConfig struct {
	Enabled bool
	MaxHops int
}

const (
	modelFallbackEnabledOptionKey      = "ModelFallbackEnabled"
	modelFallbackMaxHopsOptionKey      = "ModelFallbackMaxHops"
	modelFallbackDefaultChainOptionKey = "ModelFallbackDefaultChain"
	modelFallbackChainPrefix           = "ModelFallbackChain."
)

var defaultModelFallbackChain = []string{"gpt-5.2", "gml-5"}

func LoadModelFallbackConfig() ModelFallbackConfig {
	defaultCfg := ModelFallbackConfig{
		Enabled: true,
		MaxHops: 1,
	}

	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()
	if OptionMap == nil {
		return defaultCfg
	}

	out := defaultCfg
	out.Enabled = parseOptionBool(OptionMap[modelFallbackEnabledOptionKey], defaultCfg.Enabled)
	out.MaxHops = parseOptionIntInRange(OptionMap[modelFallbackMaxHopsOptionKey], defaultCfg.MaxHops, 0, 10)
	return out
}

// GetModelFallbackChain returns the fallback chain for requestedModel.
// It prefers per-model chain (ModelFallbackChain.<normalizedModel>) and falls back to default chain.
// Default chain is only applied to GPT/GML-style models unless explicitly configured per-model.
func GetModelFallbackChain(requestedModel string) ([]string, int, bool) {
	cfg := LoadModelFallbackConfig()
	if !cfg.Enabled {
		return nil, cfg.MaxHops, false
	}
	maxHops := cfg.MaxHops
	if maxHops <= 0 {
		maxHops = 1
	}

	normalized := NormalizeModelName(requestedModel)
	if normalized == "" {
		return nil, maxHops, false
	}

	OptionMapRWMutex.RLock()
	perModelRaw := ""
	defaultRaw := ""
	if OptionMap != nil {
		perModelRaw = OptionMap[modelFallbackChainPrefix+normalized]
		defaultRaw = OptionMap[modelFallbackDefaultChainOptionKey]
	}
	OptionMapRWMutex.RUnlock()

	chain := parseModelFallbackChain(perModelRaw, requestedModel)
	if len(chain) == 0 && isDefaultFallbackFamily(normalized) {
		chain = parseModelFallbackChain(defaultRaw, requestedModel)
		if len(chain) == 0 {
			chain = append([]string(nil), defaultModelFallbackChain...)
		}
	}

	if len(chain) == 0 {
		return nil, maxHops, false
	}

	if len(chain) > maxHops {
		chain = chain[:maxHops]
	}
	return chain, maxHops, true
}

func isDefaultFallbackFamily(normalizedModel string) bool {
	return strings.HasPrefix(normalizedModel, "gpt-") || strings.HasPrefix(normalizedModel, "gml-")
}

func parseModelFallbackChain(raw string, requestedModel string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}

	requestedLower := strings.ToLower(strings.TrimSpace(requestedModel))
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		itemLower := strings.ToLower(item)
		if itemLower == requestedLower {
			continue
		}
		if seen[itemLower] {
			continue
		}
		seen[itemLower] = true
		out = append(out, item)
	}
	return out
}

