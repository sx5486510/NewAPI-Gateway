package cpa

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	cpaconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func TestRuntimeInvariantsForceGatewayOwnedFields(t *testing.T) {
	invariants, err := NewRuntimeInvariants(bytes.NewReader(bytes.Repeat([]byte{0x2a}, 64)))
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("host: 0.0.0.0\nport: 29004\nauth-dir: auth\napi-keys: [key]\nremote-management:\n  allow-remote: true\n  secret-key: user-selected-management-key\n  disable-control-panel: false\n  disable-auto-update-panel: false\n")

	normalized, cfg, err := invariants.ApplyYAML(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != loopbackHost || cfg.RemoteManagement.AllowRemote || !cfg.RemoteManagement.DisableControlPanel || !cfg.RemoteManagement.DisableAutoUpdatePanel {
		t.Fatalf("runtime invariants not applied: %+v", cfg)
	}
	if strings.Contains(string(normalized), "user-selected-management-key") {
		t.Fatalf("user management key survived normalization:\n%s", normalized)
	}
}

func TestStartEmbeddedAcceptsOnlyRuntimeManagementPassword(t *testing.T) {
	configPath, port := writeManagedCPAConfig(t)
	result, err := StartEmbedded(configPath, "runtime-secret")
	if err != nil {
		t.Fatalf("StartEmbedded: %v", err)
	}
	t.Cleanup(func() { stopEmbedResult(t, result) })
	waitForHealth(t, result.BaseURL)

	assertManagementStatus(t, result.BaseURL, "runtime-secret", http.StatusOK)
	assertManagementStatus(t, result.BaseURL, "gateway-managed", http.StatusUnauthorized)
	if _, exposed := reflect.TypeOf(*result).FieldByName("ManagementPassword"); exposed {
		t.Fatal("EmbedResult exposes the runtime management password")
	}
	if result.BaseURL != fmt.Sprintf("http://127.0.0.1:%d", port) {
		t.Fatalf("BaseURL = %q", result.BaseURL)
	}
}

func TestStartEmbeddedRejectsMissingManagementSentinel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := fmt.Sprintf("host: 127.0.0.1\nport: %d\nauth-dir: %q\napi-keys: [key]\nremote-management:\n  secret-key: ''\n", freePort(t), filepath.Join(dir, "auth"))
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := StartEmbedded(path, "runtime-secret"); err == nil || !strings.Contains(err.Error(), "sentinel") {
		t.Fatalf("expected missing sentinel error, got %v", err)
	}
}

func TestStartEmbeddedRejectsEmptyRuntimeManagementPassword(t *testing.T) {
	configPath, _ := writeManagedCPAConfig(t)
	if _, err := StartEmbedded(configPath, "  "); err == nil || !strings.Contains(err.Error(), "management password") {
		t.Fatalf("expected empty management password error, got %v", err)
	}
}

func TestStartEmbeddedRejectsInvalidManagementSentinel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := fmt.Sprintf("host: 127.0.0.1\nport: %d\nauth-dir: %q\napi-keys: [key]\nremote-management:\n  secret-key: not-bcrypt\n", freePort(t), filepath.Join(dir, "auth"))
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := StartEmbedded(path, "runtime-secret"); err == nil || !strings.Contains(err.Error(), "sentinel") {
		t.Fatalf("expected invalid sentinel error, got %v", err)
	}
}

func TestCPALocalPasswordStillRequiresConfiguredSentinel(t *testing.T) {
	dir := t.TempDir()
	port := freePort(t)
	path := filepath.Join(dir, "config.yaml")
	raw := fmt.Sprintf("host: 127.0.0.1\nport: %d\nauth-dir: %q\napi-keys: [key]\nremote-management:\n  secret-key: ''\n", port, filepath.Join(dir, "auth"))
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := cpaconfig.ParseConfigBytes([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	service, err := cliproxy.NewBuilder().
		WithConfig(cfg).
		WithConfigPath(path).
		WithLocalManagementPassword("runtime-secret").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = service.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(35 * time.Second):
			t.Error("CPA did not stop")
		}
	})
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHealth(t, baseURL)

	request, err := http.NewRequest(http.MethodGet, baseURL+"/v0/management/config", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer runtime-secret")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusForbidden || !strings.Contains(string(body), "management key not set") {
		t.Fatalf("response = %d %s", response.StatusCode, body)
	}
}

func writeManagedCPAConfig(t *testing.T) (string, int) {
	t.Helper()
	dir := t.TempDir()
	port := freePort(t)
	raw := fmt.Sprintf("host: 0.0.0.0\nport: %d\nauth-dir: %q\napi-keys: [embed-key]\nremote-management:\n  allow-remote: true\n  secret-key: ignored\n", port, filepath.Join(dir, "auth"))
	invariants, err := NewRuntimeInvariants(bytes.NewReader(bytes.Repeat([]byte{0x33}, 64)))
	if err != nil {
		t.Fatal(err)
	}
	normalized, _, err := invariants.ApplyYAML([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, normalized, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, port
}

func assertManagementStatus(t *testing.T, baseURL, password string, want int) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, baseURL+"/v0/management/config", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+password)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("management status = %d, want %d: %s", response.StatusCode, want, body)
	}
}

func stopEmbedResult(t *testing.T, result *EmbedResult) {
	t.Helper()
	result.Cancel()
	select {
	case <-result.Done:
	case <-time.After(35 * time.Second):
		t.Error("CPA did not stop")
	}
}

func waitForHealth(t *testing.T, baseURL string) {
	t.Helper()
	if _, ok := pollGet(baseURL+"/healthz", 15*time.Second); !ok {
		t.Fatalf("CPA health endpoint %s not reachable", baseURL)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func pollGet(url string, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			return string(body), true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", false
}
