package cpa

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestManagerLifecycle exercises the full DB-driven lifecycle against a real
// embedded CPA instance: StartFromDB (enabled) → running + onReady fired →
// Reload → still running → Stop. Uses a temp working dir so the materialized
// .smoke/cpa-config.yaml doesn't touch the repo.
func TestManagerLifecycle(t *testing.T) {
	origWd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		Stop()
		_ = os.Chdir(origWd)
	})

	authDir := filepath.Join(tmp, "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth: %v", err)
	}

	port := freePort(t)
	seedOptionMap(true, `["mgr-key"]`, authDir, fmt.Sprintf("%d", port))

	var readyCount int32
	var gotBaseURL, gotAPIKey atomicString
	onReady := func(baseURL, apiKey string) {
		atomic.AddInt32(&readyCount, 1)
		gotBaseURL.Store(baseURL)
		gotAPIKey.Store(apiKey)
	}

	if err := StartFromDB(onReady); err != nil {
		t.Fatalf("StartFromDB: %v", err)
	}
	if !IsRunning() {
		t.Fatalf("expected IsRunning=true after StartFromDB")
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d/", port)
	if _, ok := pollGet(baseURL, 15*time.Second); !ok {
		t.Fatalf("CPA not reachable on %s after StartFromDB", baseURL)
	}
	waitFor(t, func() bool { return atomic.LoadInt32(&readyCount) >= 1 }, 5*time.Second, "onReady not fired")
	if gotAPIKey.Load() != "mgr-key" {
		t.Fatalf("onReady apiKey = %q, want mgr-key", gotAPIKey.Load())
	}

	// Reload must keep the service reachable (stop old, start new).
	if err := Reload(onReady); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if !IsRunning() {
		t.Fatalf("expected IsRunning=true after Reload")
	}
	if _, ok := pollGet(baseURL, 15*time.Second); !ok {
		t.Fatalf("CPA not reachable on %s after Reload", baseURL)
	}
	waitFor(t, func() bool { return atomic.LoadInt32(&readyCount) >= 2 }, 5*time.Second, "onReady not fired after reload")

	// Stop must tear it down.
	Stop()
	if IsRunning() {
		t.Fatalf("expected IsRunning=false after Stop")
	}
}

// TestStartFromDBDisabled verifies a disabled config is a no-op (no instance).
func TestStartFromDBDisabled(t *testing.T) {
	origWd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	seedOptionMap(false, `["x"]`, filepath.Join(tmp, "auth"), "29010")

	if err := StartFromDB(nil); err != nil {
		t.Fatalf("StartFromDB (disabled): %v", err)
	}
	if IsRunning() {
		t.Fatalf("expected IsRunning=false when CPAEnabled=false")
	}
}

// --- helpers ---

type atomicString struct{ v atomic.Value }

func (a *atomicString) Store(s string) { a.v.Store(s) }
func (a *atomicString) Load() string {
	if s, ok := a.v.Load().(string); ok {
		return s
	}
	return ""
}

func waitFor(t *testing.T, cond func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timeout waiting: %s", msg)
}

// pollGetBody is a small helper used by pollGet already defined in the PoC test;
// re-declared here defensively is avoided — pollGet lives in embed_poc_test.go.
var _ = io.Discard
var _ = http.MethodGet
