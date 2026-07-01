package model

import (
	"NewAPI-Gateway/common"
	"testing"
)

func TestLLMTraceOptionUpdatesCommonFlag(t *testing.T) {
	common.LLMTraceEnabled = false
	common.OptionMap = map[string]string{}

	updateOptionMap("LLMTraceEnabled", "true")
	if !common.LLMTraceEnabled {
		t.Fatalf("expected LLMTraceEnabled true")
	}

	updateOptionMap("LLMTraceEnabled", "false")
	if common.LLMTraceEnabled {
		t.Fatalf("expected LLMTraceEnabled false")
	}
}
