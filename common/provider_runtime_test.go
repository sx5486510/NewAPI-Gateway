package common

import (
	"testing"
)

func TestProviderRuntimeAvailabilityDefaultsToAvailable(t *testing.T) {
	ResetProviderRuntimeAvailabilityForTest()
	if !IsProviderRuntimeAvailable(41) {
		t.Fatal("unregistered provider must remain available")
	}
	SetProviderRuntimeAvailable(41, false)
	if IsProviderRuntimeAvailable(41) {
		t.Fatal("provider should be unavailable")
	}
	ClearProviderRuntimeAvailability(41)
	if !IsProviderRuntimeAvailable(41) {
		t.Fatal("cleared provider should use default")
	}
}
