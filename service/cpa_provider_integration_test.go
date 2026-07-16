package service

import (
	"NewAPI-Gateway/model"
	"NewAPI-Gateway/service/cpa"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestEmbeddedCPARegistrationIntegration boots a real embedded CPA instance on
// loopback and drives the actual registration path: provider upsert + readiness
// wait against the live instance. It stops short of full model sync, which
// requires real upstream OAuth credentials not available in CI.
func TestEmbeddedCPARegistrationIntegration(t *testing.T) {
	setupCPAProviderTestDB(t)

	dir := t.TempDir()
	authDir := filepath.Join(dir, "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth: %v", err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := fmt.Sprintf("host: \"\"\nport: 8317\ntls:\n  enable: false\nremote-management:\n  allow-remote: false\nauth-dir: %q\napi-keys:\n  - \"integration-key\"\ndebug: false\npprof:\n  enable: false\nplugins:\n  enabled: false\nlogging-to-file: false\nusage-statistics-enabled: false\nrequest-retry: 1\n", authDir)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	port := freeTCPPort(t)
	res, err := cpa.StartEmbedded(cfgPath, port)
	if err != nil {
		t.Fatalf("StartEmbedded: %v", err)
	}
	t.Cleanup(func() {
		res.Cancel()
		select {
		case <-res.Done:
		case <-time.After(35 * time.Second):
			t.Log("warning: CPA did not stop within timeout")
		}
	})

	// The registration upsert must persist a key_only provider.
	provider, err := upsertEmbeddedCPAProvider(res.BaseURL, res.APIKey)
	if err != nil {
		t.Fatalf("upsert embedded provider: %v", err)
	}
	if provider.ProviderType != model.ProviderTypeKeyOnly {
		t.Fatalf("provider type = %q, want key_only", provider.ProviderType)
	}
	if provider.BaseURL != res.BaseURL {
		t.Fatalf("provider base url = %q, want %q", provider.BaseURL, res.BaseURL)
	}

	// The live embedded instance must become reachable on loopback.
	if !waitForCPAReady(res.BaseURL, 20*time.Second) {
		t.Fatalf("embedded CPA at %s never became ready", res.BaseURL)
	}
	t.Logf("integration ok: provider id=%d base=%s ready", provider.Id, res.BaseURL)
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
