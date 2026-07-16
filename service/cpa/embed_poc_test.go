package cpa

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestStartEmbeddedPoC proves the embedded CPA service boots inside this
// process, binds to loopback on the requested port, and serves its HTTP API.
// It is a proof-of-concept smoke test, not a long-term assertion of behavior.
func TestStartEmbeddedPoC(t *testing.T) {
	dir := t.TempDir()
	authDir := filepath.Join(dir, "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth: %v", err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := fmt.Sprintf("host: \"\"\nport: 8317\ntls:\n  enable: false\nremote-management:\n  allow-remote: false\nauth-dir: %q\napi-keys:\n  - \"poc-test-key\"\ndebug: false\npprof:\n  enable: false\nplugins:\n  enabled: false\nlogging-to-file: false\nusage-statistics-enabled: false\nrequest-retry: 1\n", authDir)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	port := freePort(t)
	res, err := StartEmbedded(cfgPath, port)
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

	if res.APIKey != "poc-test-key" {
		t.Fatalf("expected APIKey %q, got %q", "poc-test-key", res.APIKey)
	}
	if res.BaseURL != fmt.Sprintf("http://127.0.0.1:%d", port) {
		t.Fatalf("unexpected BaseURL: %s", res.BaseURL)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	body, ok := pollGet(url, 15*time.Second)
	if !ok {
		t.Fatalf("CPA loopback endpoint %s not reachable within timeout", url)
	}
	if !strings.Contains(body, "CLI Proxy API Server") {
		t.Fatalf("unexpected root response: %s", body)
	}
	t.Logf("embedded CPA reachable on 127.0.0.1:%d, root body: %s", port, body)

	// Confirm loopback-only: the service must not answer on a non-loopback host.
	// (We only assert the root endpoint served the expected CPA banner above.)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func pollGet(url string, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return string(b), true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return "", false
}
