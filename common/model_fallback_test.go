package common

import (
	"reflect"
	"testing"
)

func TestGetModelFallbackChain_DefaultFamily(t *testing.T) {
	OptionMapRWMutex.Lock()
	previous := OptionMap
	OptionMap = map[string]string{}
	OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		OptionMapRWMutex.Lock()
		OptionMap = previous
		OptionMapRWMutex.Unlock()
	})

	chain, maxHops, ok := GetModelFallbackChain("gpt-5")
	if !ok {
		t.Fatalf("expected fallback enabled")
	}
	if maxHops != 1 {
		t.Fatalf("expected maxHops=1, got %d", maxHops)
	}
	want := []string{"gpt-5.2"}
	if !reflect.DeepEqual(chain, want) {
		t.Fatalf("GetModelFallbackChain() = %v, want %v", chain, want)
	}
}

func TestGetModelFallbackChain_NoDefaultForOtherFamilies(t *testing.T) {
	OptionMapRWMutex.Lock()
	previous := OptionMap
	OptionMap = map[string]string{}
	OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		OptionMapRWMutex.Lock()
		OptionMap = previous
		OptionMapRWMutex.Unlock()
	})

	chain, _, ok := GetModelFallbackChain("claude-3.5-sonnet")
	if ok || len(chain) != 0 {
		t.Fatalf("expected no default fallback for non-gpt model, got ok=%v chain=%v", ok, chain)
	}
}

func TestGetModelFallbackChain_PerModelOverridesDefault(t *testing.T) {
	OptionMapRWMutex.Lock()
	previous := OptionMap
	OptionMap = map[string]string{
		modelFallbackChainPrefix + "gpt-5": `["gpt-5.2","gml-5"]`,
	}
	OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		OptionMapRWMutex.Lock()
		OptionMap = previous
		OptionMapRWMutex.Unlock()
	})

	chain, maxHops, ok := GetModelFallbackChain("gpt-5")
	if !ok {
		t.Fatalf("expected fallback enabled")
	}
	if maxHops != 1 {
		t.Fatalf("expected maxHops=1, got %d", maxHops)
	}
	want := []string{"gpt-5.2"}
	if !reflect.DeepEqual(chain, want) {
		t.Fatalf("GetModelFallbackChain() = %v, want %v", chain, want)
	}
}

