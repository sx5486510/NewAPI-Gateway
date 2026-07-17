package service

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// EmbeddedCPAProviderName is the fixed, well-known name of the auto-registered
// provider that fronts the embedded CPA instance. Registration is idempotent on
// this name: an existing provider is updated in place, preserving any manual
// tuning (priority/weight/status) an operator applied.
const EmbeddedCPAProviderName = "__embedded_cpa__"

// CPAProviderRegistrationCallback returns an onReady callback (baseURL, apiKey)
// suitable for cpa.StartFromDB / cpa.Reload. This is a legacy compatibility
// adapter for the old synchronous registration path. New code should use
// CPAProviderCoordinator directly.
func CPAProviderRegistrationCallback() func(baseURL, apiKey string) {
	return func(baseURL, apiKey string) {
		if err := RegisterEmbeddedCPAProvider(baseURL, apiKey); err != nil {
			common.SysLog("embedded CPA provider registration failed: " + err.Error())
		}
	}
}

// RegisterEmbeddedCPAProvider is a legacy synchronous registration function
// that upserts the provider, waits for CPA readiness, and syncs. New code
// should use CPAProviderCoordinator for lifecycle-aware, debounced sync.
func RegisterEmbeddedCPAProvider(baseURL string, apiKey string) error {
	baseURL = strings.TrimSpace(baseURL)
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" {
		return fmt.Errorf("cpa: base url is empty")
	}
	if apiKey == "" {
		return fmt.Errorf("cpa: api key is empty (set at least one api-keys entry in the CPA config)")
	}

	provider, err := upsertEmbeddedCPAProvider(baseURL, apiKey)
	if err != nil {
		return err
	}

	// Mark runtime available for legacy path
	common.SetProviderRuntimeAvailable(provider.Id, true)

	// Legacy: wait for CPA ready before sync
	if !waitForCPAReady(baseURL, 30*time.Second) {
		return fmt.Errorf("cpa: embedded instance at %s not ready within timeout", baseURL)
	}

	if err := SyncProvider(provider); err != nil {
		return fmt.Errorf("cpa: sync embedded provider failed: %w", err)
	}
	common.SysLog(fmt.Sprintf("embedded CPA provider %q registered and synced (id=%d)", EmbeddedCPAProviderName, provider.Id))
	return nil
}

// CPAProviderCoordinator manages the lifecycle-aware synchronization of the
// embedded CPA provider. It debounces sync requests within 750ms and ensures
// the provider's runtime availability matches CPA's actual running state.
type CPAProviderCoordinator struct {
	mu           sync.Mutex
	providerID   int
	timer        *time.Timer
	debounce     time.Duration
	syncProvider func(*model.Provider) error
	closed       bool
}

// NewCPAProviderCoordinator creates a coordinator with the given sync function.
// The sync function should call SyncProvider or equivalent.
func NewCPAProviderCoordinator(syncFn func(*model.Provider) error) *CPAProviderCoordinator {
	return &CPAProviderCoordinator{
		debounce:     750 * time.Millisecond,
		syncProvider: syncFn,
	}
}

// OnCPAReady is called when CPA becomes running and ready. It upserts the
// provider connection details, marks it runtime-available, and performs one
// immediate sync.
func (c *CPAProviderCoordinator) OnCPAReady(baseURL, apiKey string) {
	provider, err := upsertEmbeddedCPAProvider(baseURL, apiKey)
	if err != nil {
		common.SysLog(fmt.Sprintf("embedded CPA provider upsert failed: %v", err))
		return
	}

	c.mu.Lock()
	c.providerID = provider.Id
	c.mu.Unlock()

	// Mark runtime available before sync
	common.SetProviderRuntimeAvailable(provider.Id, true)

	// Immediate sync on ready
	if err := c.syncProvider(provider); err != nil {
		common.SysLog(fmt.Sprintf("embedded CPA provider sync failed: %v", err))
	} else {
		common.SysLog(fmt.Sprintf("embedded CPA provider %q ready and synced (id=%d)", EmbeddedCPAProviderName, provider.Id))
	}
}

// OnCPAUnavailable is called when CPA stops. It marks the provider runtime-
// unavailable and cancels any pending sync timer.
func (c *CPAProviderCoordinator) OnCPAUnavailable() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}

	// Mark runtime unavailable
	if c.providerID != 0 {
		common.SetProviderRuntimeAvailable(c.providerID, false)
	} else {
		// Look up by name if we don't have the ID
		if provider, err := model.GetProviderByName(EmbeddedCPAProviderName); err == nil && provider != nil {
			common.SetProviderRuntimeAvailable(provider.Id, false)
		}
	}
}

// ScheduleCPASync requests a provider sync. Multiple calls within 750ms are
// collapsed into one sync operation.
func (c *CPAProviderCoordinator) ScheduleCPASync() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	if c.timer != nil {
		c.timer.Stop()
	}

	c.timer = time.AfterFunc(c.debounce, func() {
		provider, err := model.GetProviderByName(EmbeddedCPAProviderName)
		if err != nil || provider == nil {
			return
		}
		if err := c.syncProvider(provider); err != nil {
			common.SysLog(fmt.Sprintf("debounced CPA sync failed: %v", err))
		}
	})
}

// Close stops the coordinator and cancels any pending timer.
func (c *CPAProviderCoordinator) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

// upsertEmbeddedCPAProvider creates or updates the well-known embedded CPA
// provider and returns the persisted record. It preserves operator-tuned
// fields like Status, Priority, and Weight.
func upsertEmbeddedCPAProvider(baseURL string, apiKey string) (*model.Provider, error) {
	existing, err := model.GetProviderByName(EmbeddedCPAProviderName)
	if err != nil {
		return nil, fmt.Errorf("cpa: lookup existing provider failed: %w", err)
	}

	if existing != nil {
		// Update only the connection details; keep operator-tuned fields intact.
		existing.BaseURL = baseURL
		existing.ApiKey = apiKey
		existing.ProviderType = model.ProviderTypeKeyOnly
		existing.CheckinEnabled = false
		if err := existing.Update(); err != nil {
			return nil, fmt.Errorf("cpa: update embedded provider failed: %w", err)
		}
		return model.GetProviderById(existing.Id)
	}

	provider := &model.Provider{
		Name:           EmbeddedCPAProviderName,
		BaseURL:        baseURL,
		ApiKey:         apiKey,
		ProviderType:   model.ProviderTypeKeyOnly,
		Status:         common.UserStatusEnabled,
		Priority:       0,
		Weight:         10,
		CheckinEnabled: false,
		Remark:         "Auto-registered embedded CLIProxyAPI (CPA) instance on loopback.",
	}
	if err := provider.Insert(); err != nil {
		return nil, fmt.Errorf("cpa: insert embedded provider failed: %w", err)
	}
	return model.GetProviderByName(EmbeddedCPAProviderName)
}

// waitForCPAReady polls the CPA root endpoint until it responds or the timeout
// elapses. Only used by legacy integration paths.
func waitForCPAReady(baseURL string, timeout time.Duration) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	url := strings.TrimRight(baseURL, "/") + "/"
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}
