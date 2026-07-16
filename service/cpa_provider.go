package service

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// EmbeddedCPAProviderName is the fixed, well-known name of the auto-registered
// provider that fronts the embedded CPA instance. Registration is idempotent on
// this name: an existing provider is updated in place, preserving any manual
// tuning (priority/weight/status) an operator applied.
const EmbeddedCPAProviderName = "__embedded_cpa__"

// CPAProviderRegistrationCallback returns an onReady callback (baseURL, apiKey)
// suitable for cpa.StartFromDB / cpa.Reload. It runs RegisterEmbeddedCPAProvider
// and logs any error. This adapter lets the cpa package trigger provider
// registration without importing the service package (avoiding an import cycle).
func CPAProviderRegistrationCallback() func(baseURL, apiKey string) {
	return func(baseURL, apiKey string) {
		if err := RegisterEmbeddedCPAProvider(baseURL, apiKey); err != nil {
			common.SysLog("embedded CPA provider registration failed: " + err.Error())
		}
	}
}

// RegisterEmbeddedCPAProvider ensures a key_only provider pointing at the
// embedded CPA loopback endpoint exists, then waits for CPA to become ready and
// syncs its model list so routes are built.
//
// It is safe to call on every startup. baseURL is e.g. "http://127.0.0.1:18317"
// and apiKey is the CPA client key used to authenticate to that instance.
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

	// CPA starts asynchronously; wait until its HTTP API answers before syncing,
	// otherwise fetching /v1/models fails.
	if !waitForCPAReady(baseURL, 30*time.Second) {
		return fmt.Errorf("cpa: embedded instance at %s not ready within timeout", baseURL)
	}

	if err := SyncProvider(provider); err != nil {
		return fmt.Errorf("cpa: sync embedded provider failed: %w", err)
	}
	common.SysLog(fmt.Sprintf("embedded CPA provider %q registered and synced (id=%d)", EmbeddedCPAProviderName, provider.Id))
	return nil
}

// upsertEmbeddedCPAProvider creates or updates the well-known embedded CPA
// provider and returns the persisted record.
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
// elapses.
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
