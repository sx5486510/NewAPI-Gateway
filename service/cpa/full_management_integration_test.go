//go:build cpa_integration

package cpa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	sdkAuth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cpaconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// TestFullManagementRoundTripAgainstRealCPA starts a real embedded CPA instance
// and exercises the complete management API surface through the Gateway proxy.
// This is an integration test - it verifies that Runtime, Manager, and ManagementProxy
// work together correctly with a real CPA binary.
func TestFullManagementRoundTripAgainstRealCPA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tempDir := t.TempDir()
	runtimeDir := filepath.Join(tempDir, "cpa-runtime")
	authDir := filepath.Join(tempDir, "auth")
	rawConfig := strings.Join([]string{
		"host: 127.0.0.1",
		"port: " + strconv.Itoa(freePort(t)),
		`auth-dir: "` + filepath.ToSlash(authDir) + `"`,
		"api-keys: [integration-key]",
		"debug: false",
		"plugins:",
		"  instances:",
		"    task10-demo:",
		"      custom-field: keep-me",
		"",
	}, "\n")
	options := newMemoryOptions(map[string]string{
		snapshotOptionKey: rawConfig,
		"CPAEnabled":      "true",
	})
	invariants, err := NewRuntimeInvariants(nil)
	if err != nil {
		t.Fatalf("NewRuntimeInvariants: %v", err)
	}
	store := NewSnapshotStore(runtimeDir, invariants)
	store.options = options
	authStore := sdkAuth.NewFileTokenStore()
	authStore.SetBaseDir(authDir)
	coreManager := coreauth.NewManager(authStore, nil, nil)
	manager := NewManager(store, nil)
	manager.startEmbedded = startEmbeddedWithCoreAuth(coreManager)
	proxy := NewManagementProxy(manager, store, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := manager.StartFromDB(ctx); err != nil {
		t.Fatalf("failed to start CPA: %v", err)
	}
	defer manager.Shutdown(context.Background())

	status := waitRealCPARunning(t, manager, 60*time.Second)
	t.Logf("CPA started: state=%s endpoint=%s version=%s", status.State, status.Endpoint, status.Version)

	server := httptest.NewServer(proxy)
	defer server.Close()

	client := server.Client()
	baseURL := server.URL
	managementBaseURL := baseURL + "/v0/management"
	authFileName := "codex-task10@example.com-plus.json"
	authFileBody := []byte(`{"type":"codex","email":"task10@example.com","priority":98}`)
	normalizedAuthFileBody := []byte(`{"type":"codex","email":"task10@example.com","priority":98,"disabled":false}`)

	t.Run("GET config", func(t *testing.T) {
		config := getManagementJSON(t, client, managementBaseURL+"/config", http.StatusOK)
		if config["api_keys"] == nil && config["apiKeys"] == nil && config["api-keys"] == nil {
			t.Fatalf("config missing API key field: %+v", config)
		}
	})

	t.Run("PATCH debug mode", func(t *testing.T) {
		doManagementJSON(t, client, http.MethodPatch, managementBaseURL+"/debug", `{"value":true}`, http.StatusOK)
		if snapshot := options.Get(snapshotOptionKey); !strings.Contains(snapshot, "debug: true") {
			t.Fatalf("debug setting was not persisted to snapshot:\n%s", snapshot)
		}
	})

	t.Run("POST reset quota clears runtime auth state", func(t *testing.T) {
		authID, modelName, authIndex := seedQuotaResetAuth(t, coreManager)
		payload := doManagementJSON(t, client, http.MethodPost, managementBaseURL+"/reset-quota", `{"auth_index":`+quoteJSON(authIndex)+`}`, http.StatusOK)
		if got, _ := payload["auth_index"].(string); got != authIndex {
			t.Fatalf("reset auth_index = %#v, want %q", payload["auth_index"], authIndex)
		}

		updated, ok := coreManager.GetByID(authID)
		if !ok || updated == nil {
			t.Fatalf("auth %q missing after quota reset", authID)
		}
		assertQuotaCleared(t, updated, modelName)
	})

	t.Run("POST auth file upload", func(t *testing.T) {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, err := writer.CreateFormFile("file", authFileName)
		if err != nil {
			t.Fatalf("failed to create form file: %v", err)
		}
		if _, err = part.Write(authFileBody); err != nil {
			t.Fatalf("failed to write multipart content: %v", err)
		}
		if err = writer.Close(); err != nil {
			t.Fatalf("failed to close multipart writer: %v", err)
		}

		req, _ := http.NewRequest(http.MethodPost, managementBaseURL+"/auth-files", &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST /auth-files failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			failHTTP(t, resp, "POST /auth-files")
		}
	})

	t.Run("GET auth list", func(t *testing.T) {
		payload := getManagementJSON(t, client, managementBaseURL+"/auth-files", http.StatusOK)
		file := requireAuthFileEntry(t, payload, authFileName)
		if got := file["name"]; got != authFileName {
			t.Fatalf("listed auth name = %#v, want %q", got, authFileName)
		}
	})

	t.Run("download auth file", func(t *testing.T) {
		resp, err := client.Get(managementBaseURL + "/auth-files/download?name=" + url.QueryEscape(authFileName))
		if err != nil {
			t.Fatalf("GET auth download failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			failHTTP(t, resp, "GET auth download")
		}
		body, _ := io.ReadAll(resp.Body)
		if !jsonBodiesEqual(body, normalizedAuthFileBody) {
			t.Fatalf("downloaded auth body mismatch\n got: %s\nwant: %s", body, authFileBody)
		}
		if disposition := resp.Header.Get("Content-Disposition"); !strings.Contains(disposition, authFileName) {
			t.Fatalf("Content-Disposition = %q, want filename", disposition)
		}
	})

	t.Run("PATCH auth status and fields", func(t *testing.T) {
		doManagementJSON(t, client, http.MethodPatch, managementBaseURL+"/auth-files/status", `{"name":`+quoteJSON(authFileName)+`,"disabled":true}`, http.StatusOK)
		payload := getManagementJSON(t, client, managementBaseURL+"/auth-files", http.StatusOK)
		file := requireAuthFileEntry(t, payload, authFileName)
		if disabled, _ := file["disabled"].(bool); !disabled {
			t.Fatalf("disabled = %#v, want true", file["disabled"])
		}

		doManagementJSON(t, client, http.MethodPatch, managementBaseURL+"/auth-files/fields", `{"name":`+quoteJSON(authFileName)+`,"note":"task10-note"}`, http.StatusOK)
		downloaded := downloadAuthFileJSON(t, client, managementBaseURL, authFileName)
		if note, _ := downloaded["note"].(string); note != "task10-note" {
			t.Fatalf("downloaded note = %#v, want task10-note", downloaded["note"])
		}
	})

	t.Run("Restart maintains target and config", func(t *testing.T) {
		before := manager.Status().Endpoint
		basic, err := store.Basic()
		if err != nil {
			t.Fatalf("load basic config: %v", err)
		}
		basic.Port = freePort(t)
		if err := store.PatchBasic(*basic); err != nil {
			t.Fatalf("patch basic port: %v", err)
		}
		restartCtx, restartCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer restartCancel()

		if err := manager.Restart(restartCtx); err != nil {
			t.Fatalf("restart failed: %v", err)
		}
		after := waitRealCPARunning(t, manager, 45*time.Second)
		if after.Endpoint == before {
			t.Fatalf("restart endpoint did not change: before=%q after=%q", before, after.Endpoint)
		}
		config := getManagementJSON(t, client, managementBaseURL+"/config", http.StatusOK)
		if debug, _ := config["debug"].(bool); !debug {
			t.Fatalf("debug config after restart = %#v, want true", config["debug"])
		}
		if snapshot := options.Get(snapshotOptionKey); !strings.Contains(snapshot, "custom-field: keep-me") {
			t.Fatalf("plugin config was not preserved in snapshot:\n%s", snapshot)
		}
	})

	t.Run("DELETE auth file", func(t *testing.T) {
		resp, err := client.Do(mustRequest(t, http.MethodDelete, managementBaseURL+"/auth-files?name="+url.QueryEscape(authFileName), nil))
		if err != nil {
			t.Fatalf("DELETE auth file failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			failHTTP(t, resp, "DELETE auth file")
		}
		payload := getManagementJSON(t, client, managementBaseURL+"/auth-files", http.StatusOK)
		if findAuthFileEntry(payload, authFileName) != nil {
			t.Fatalf("auth file %q still present after delete: %+v", authFileName, payload)
		}
	})

	t.Run("Management unavailable after stop", func(t *testing.T) {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()

		if err := manager.Stop(stopCtx); err != nil {
			t.Fatalf("stop failed: %v", err)
		}
		resp, err := client.Get(managementBaseURL + "/config")
		if err != nil {
			t.Fatalf("GET /config after stop failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable {
			failHTTP(t, resp, "GET /config after stop")
		}
		var payload struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode 503 payload: %v", err)
		}
		if payload.Code != "cpa_unavailable" {
			t.Fatalf("error code = %q, want cpa_unavailable", payload.Code)
		}
	})
}

// TestManagementLeaseLifecycle verifies that management lease acquisition
// works correctly and provides valid credentials.
func TestManagementLeaseLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tempDir := t.TempDir()
	runtimeDir := filepath.Join(tempDir, "cpa-runtime")
	authDir := filepath.Join(tempDir, "auth")
	rawConfig := strings.Join([]string{
		"host: 127.0.0.1",
		"port: " + strconv.Itoa(freePort(t)),
		`auth-dir: "` + filepath.ToSlash(authDir) + `"`,
		"api-keys: [integration-key]",
		"",
	}, "\n")
	invariants, err := NewRuntimeInvariants(nil)
	if err != nil {
		t.Fatalf("NewRuntimeInvariants: %v", err)
	}
	store := NewSnapshotStore(runtimeDir, invariants)
	store.options = newMemoryOptions(map[string]string{
		snapshotOptionKey: rawConfig,
		"CPAEnabled":      "true",
	})
	manager := NewManager(store, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := manager.StartFromDB(ctx); err != nil {
		t.Fatalf("failed to start CPA: %v", err)
	}
	defer manager.Shutdown(context.Background())

	waitRealCPARunning(t, manager, 60*time.Second)

	lease, err := manager.AcquireManagement()
	if err != nil {
		t.Fatalf("failed to acquire management lease: %v", err)
	}
	defer lease.Release()

	if lease.Target == nil || lease.Target.String() == "" {
		t.Errorf("lease has empty Target")
	}

	if lease.Password == "" {
		t.Errorf("lease has empty Password")
	}

	t.Logf("Management lease acquired: target=%s", lease.Target.String())
}

func seedQuotaResetAuth(t *testing.T, manager *coreauth.Manager) (authID, modelName, authIndex string) {
	t.Helper()
	next := time.Now().Add(time.Hour)
	authID = "task10-reset-quota-auth"
	modelName = "claude-sonnet-4-6"
	auth := &coreauth.Auth{
		ID:             authID,
		FileName:       "task10-reset-quota-auth.json",
		Provider:       "claude",
		Status:         coreauth.StatusError,
		StatusMessage:  "quota exhausted",
		Unavailable:    true,
		NextRetryAfter: next,
		Quota:          coreauth.QuotaState{Exceeded: true, Reason: "quota", NextRecoverAt: next, BackoffLevel: 2},
		ModelStates: map[string]*coreauth.ModelState{
			modelName: {
				Status:         coreauth.StatusError,
				StatusMessage:  "quota exhausted",
				Unavailable:    true,
				NextRetryAfter: next,
				Quota:          coreauth.QuotaState{Exceeded: true, Reason: "quota", NextRecoverAt: next, BackoffLevel: 2},
			},
		},
	}
	authIndex = auth.EnsureIndex()
	if authIndex == "" {
		t.Fatal("quota reset auth index is empty")
	}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("register quota reset auth: %v", err)
	}
	return authID, modelName, authIndex
}

func assertQuotaCleared(t *testing.T, auth *coreauth.Auth, modelName string) {
	t.Helper()
	if auth.Status != coreauth.StatusActive || auth.StatusMessage != "" || auth.Unavailable || !auth.NextRetryAfter.IsZero() {
		t.Fatalf("auth state after reset = status %q message %q unavailable %v next %v",
			auth.Status, auth.StatusMessage, auth.Unavailable, auth.NextRetryAfter)
	}
	if auth.Quota.Exceeded || auth.Quota.Reason != "" || !auth.Quota.NextRecoverAt.IsZero() || auth.Quota.BackoffLevel != 0 {
		t.Fatalf("auth quota after reset = %+v, want cleared", auth.Quota)
	}
	state := auth.ModelStates[modelName]
	if state == nil {
		t.Fatalf("model state %q missing after reset", modelName)
	}
	if state.Status != coreauth.StatusActive || state.StatusMessage != "" || state.Unavailable || !state.NextRetryAfter.IsZero() {
		t.Fatalf("model state after reset = status %q message %q unavailable %v next %v",
			state.Status, state.StatusMessage, state.Unavailable, state.NextRetryAfter)
	}
	if state.Quota.Exceeded || state.Quota.Reason != "" || !state.Quota.NextRecoverAt.IsZero() || state.Quota.BackoffLevel != 0 {
		t.Fatalf("model quota after reset = %+v, want cleared", state.Quota)
	}
}

func startEmbeddedWithCoreAuth(coreManager *coreauth.Manager) func(string, string) (*EmbedResult, error) {
	return func(configPath, managementPassword string) (*EmbedResult, error) {
		trimmedPath := strings.TrimSpace(configPath)
		if trimmedPath == "" {
			return nil, fmt.Errorf("cpa: config path is empty")
		}
		if strings.TrimSpace(managementPassword) == "" {
			return nil, fmt.Errorf("cpa: runtime management password is empty")
		}
		cfg, err := cpaconfig.LoadConfig(trimmedPath)
		if err != nil {
			return nil, fmt.Errorf("cpa: load config %q failed: %w", trimmedPath, err)
		}
		if cfg == nil {
			return nil, fmt.Errorf("cpa: config %q resolved to nil", trimmedPath)
		}

		port := cfg.Port
		if port <= 0 || port > 65535 {
			return nil, fmt.Errorf("cpa: invalid internal port %d", port)
		}
		cfg.Host = loopbackHost
		cfg.RemoteManagement.AllowRemote = false
		cfg.RemoteManagement.DisableControlPanel = true
		cfg.RemoteManagement.DisableAutoUpdatePanel = true

		builder := cliproxy.NewBuilder().
			WithConfig(cfg).
			WithConfigPath(trimmedPath).
			WithLocalManagementPassword(managementPassword)
		if coreManager != nil {
			builder = builder.WithCoreAuthManager(coreManager)
		}
		service, err := builder.Build()
		if err != nil {
			return nil, fmt.Errorf("cpa: build service failed: %w", err)
		}

		ctx, cancelFn := context.WithCancel(context.Background())
		doneCh := make(chan struct{})
		errorsCh := make(chan error, 1)
		go func() {
			defer close(doneCh)
			defer close(errorsCh)
			if runErr := service.Run(ctx); runErr != nil && !errors.Is(runErr, context.Canceled) {
				errorsCh <- runErr
			}
		}()

		apiKey := ""
		if len(cfg.APIKeys) > 0 {
			apiKey = strings.TrimSpace(cfg.APIKeys[0])
		}
		return &EmbedResult{
			Cancel:  cancelFn,
			Done:    doneCh,
			Errors:  errorsCh,
			BaseURL: fmt.Sprintf("http://%s:%d", loopbackHost, port),
			APIKey:  apiKey,
		}, nil
	}
}

func waitRealCPARunning(t *testing.T, manager *Manager, timeout time.Duration) Status {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status := manager.Status()
		if status.State == StateRunning && status.Ready {
			return status
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("CPA did not reach ready state: %+v", manager.Status())
	return Status{}
}

func getManagementJSON(t *testing.T, client *http.Client, target string, wantStatus int) map[string]any {
	t.Helper()
	resp, err := client.Get(target)
	if err != nil {
		t.Fatalf("GET %s failed: %v", target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		failHTTP(t, resp, "GET "+target)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode JSON from %s: %v", target, err)
	}
	return payload
}

func doManagementJSON(t *testing.T, client *http.Client, method, target, body string, wantStatus int) map[string]any {
	t.Helper()
	req := mustRequest(t, method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		failHTTP(t, resp, method+" "+target)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode JSON from %s %s: %v", method, target, err)
	}
	return payload
}

func downloadAuthFileJSON(t *testing.T, client *http.Client, managementBaseURL, name string) map[string]any {
	t.Helper()
	resp, err := client.Get(managementBaseURL + "/auth-files/download?name=" + url.QueryEscape(name))
	if err != nil {
		t.Fatalf("download auth file %q: %v", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		failHTTP(t, resp, "download auth file "+name)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode downloaded auth file %q: %v", name, err)
	}
	return payload
}

func mustRequest(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, target, err)
	}
	return req
}

func failHTTP(t *testing.T, resp *http.Response, operation string) {
	t.Helper()
	body, _ := io.ReadAll(resp.Body)
	t.Fatalf("%s returned %d: %s", operation, resp.StatusCode, body)
}

func quoteJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func requireAuthFileEntry(t *testing.T, payload map[string]any, name string) map[string]any {
	t.Helper()
	entry := findAuthFileEntry(payload, name)
	if entry == nil {
		t.Fatalf("auth file %q not found in payload: %+v", name, payload)
	}
	return entry
}

func findAuthFileEntry(payload map[string]any, name string) map[string]any {
	files, _ := payload["files"].([]any)
	for _, raw := range files {
		entry, _ := raw.(map[string]any)
		if entry != nil && entry["name"] == name {
			return entry
		}
	}
	return nil
}

func jsonBodiesEqual(left, right []byte) bool {
	var leftValue any
	var rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	leftCanonical, _ := json.Marshal(leftValue)
	rightCanonical, _ := json.Marshal(rightValue)
	return bytes.Equal(leftCanonical, rightCanonical)
}
