package common

import "sync"

// providerRuntimeAvailability tracks the runtime availability of providers.
// A missing entry defaults to available. An explicit false entry marks the
// provider as temporarily unavailable (e.g., embedded CPA is stopped).
var providerRuntimeAvailability sync.Map

// SetProviderRuntimeAvailable marks a provider as available or unavailable
// at runtime. This does not affect the provider's persisted status in the
// database.
func SetProviderRuntimeAvailable(providerID int, available bool) {
	providerRuntimeAvailability.Store(providerID, available)
}

// ClearProviderRuntimeAvailability removes the runtime availability entry
// for a provider, causing it to default back to available.
func ClearProviderRuntimeAvailability(providerID int) {
	providerRuntimeAvailability.Delete(providerID)
}

// IsProviderRuntimeAvailable returns whether a provider is available at
// runtime. The default for unregistered providers is true.
func IsProviderRuntimeAvailable(providerID int) bool {
	val, ok := providerRuntimeAvailability.Load(providerID)
	if !ok {
		return true // default to available
	}
	available, _ := val.(bool)
	return available
}

// ResetProviderRuntimeAvailabilityForTest clears all runtime availability
// entries. Only for use in tests.
func ResetProviderRuntimeAvailabilityForTest() {
	providerRuntimeAvailability.Range(func(key, value interface{}) bool {
		providerRuntimeAvailability.Delete(key)
		return true
	})
}
