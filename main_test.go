package main

import (
	"os"
	"strings"
	"testing"
)

func TestMainInitializesCPARuntimeWithEnvironmentDefault(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(source)
	if strings.Contains(text, `cpa.NewRuntime("cpa", coordinator)`) {
		t.Fatal("main hardcodes cpa runtime dir instead of allowing CPA_RUNTIME_DIR")
	}
	if !strings.Contains(text, `cpa.NewRuntime("", coordinator)`) {
		t.Fatal("main should pass empty runtime dir so CPA_RUNTIME_DIR or default cpa is used")
	}
}
