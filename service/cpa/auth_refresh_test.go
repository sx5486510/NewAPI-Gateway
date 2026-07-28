package cpa

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagementProxyRefreshesXAIAuthFileByName(t *testing.T) {
	authDir := t.TempDir()
	const authName = "xai-manual.json"
	authPath := filepath.Join(authDir, authName)
	oldExpired := "2020-01-01T00:00:00Z"
	credential := map[string]interface{}{
		"type":           "xai",
		"access_token":   "expired-access",
		"refresh_token":  "old-refresh",
		"sub":            "subject-manual",
		"expired":        oldExpired,
		"token_endpoint": "https://auth.x.ai/oauth2/token",
		"note":           "preserve-me",
	}
	body, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authPath, body, 0o600); err != nil {
		t.Fatal(err)
	}

	refreshCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"files": []map[string]interface{}{{
					"name": authName, "provider": "xai", "auth_index": "manual-index",
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/api-call":
			var call capturedAPICall
			if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
				t.Fatal(err)
			}
			if call.URL != "https://auth.x.ai/oauth2/token" {
				t.Fatalf("api-call URL = %q, want token endpoint", call.URL)
			}
			refreshCalls++
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status_code": http.StatusOK,
				"body": map[string]interface{}{
					"access_token": "fresh-access", "refresh_token": "fresh-refresh",
					"id_token": "fresh-id", "token_type": "Bearer", "expires_in": 3600,
				},
			})
		default:
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.String())
		}
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := NewManagementProxy(
		&fakeLeaseProvider{target: target, password: "runtime-secret"},
		&xaiQuotaTestStore{authDir: authDir},
		nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/refresh", strings.NewReader(`{"filename":"`+authName+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Filename   string `json:"filename"`
			OldExpired string `json:"old_expired"`
			NewExpired string `json:"new_expired"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success {
		t.Fatalf("success = false, body = %s", rec.Body.String())
	}
	if resp.Data.Filename != authName || resp.Data.OldExpired != oldExpired {
		t.Fatalf("unexpected response data: %#v", resp.Data)
	}
	newExpired, err := time.Parse(time.RFC3339, resp.Data.NewExpired)
	if err != nil || !newExpired.After(time.Now()) {
		t.Fatalf("new_expired = %q, err = %v", resp.Data.NewExpired, err)
	}
	if refreshCalls != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshCalls)
	}

	updated, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatal(err)
	}
	var updatedCredential map[string]interface{}
	if err := json.Unmarshal(updated, &updatedCredential); err != nil {
		t.Fatal(err)
	}
	if updatedCredential["access_token"] != "fresh-access" || updatedCredential["refresh_token"] != "fresh-refresh" {
		t.Fatalf("credential was not refreshed: %#v", updatedCredential)
	}
	if updatedCredential["note"] != "preserve-me" {
		t.Fatalf("unrelated field not preserved: %#v", updatedCredential)
	}
}

func TestManagementProxyRefreshAuthFileRejectsNonXAIProvider(t *testing.T) {
	authDir := t.TempDir()
	const authName = "claude.json"
	if err := os.WriteFile(filepath.Join(authDir, authName), []byte(`{"type":"claude","expired":"2020-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v0/management/auth-files" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"files": []map[string]interface{}{{
				"name": authName, "provider": "claude", "auth_index": "claude-index",
			}},
		})
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := NewManagementProxy(
		&fakeLeaseProvider{target: target, password: "runtime-secret"},
		&xaiQuotaTestStore{authDir: authDir},
		nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/refresh", strings.NewReader(`{"filename":"`+authName+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"unsupported_provider"`) {
		t.Fatalf("response missing unsupported provider code: %s", rec.Body.String())
	}
}

func TestManagementProxyRefreshAuthFileRejectsMissingFile(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v0/management/auth-files" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.String())
		}
		_, _ = io.WriteString(w, `{"files":[]}`)
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := NewManagementProxy(
		&fakeLeaseProvider{target: target, password: "runtime-secret"},
		&xaiQuotaTestStore{authDir: t.TempDir()},
		nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/refresh", strings.NewReader(`{"filename":"missing.json"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"not_found"`) {
		t.Fatalf("response missing not_found code: %s", rec.Body.String())
	}
}

func TestManagementProxyRefreshAuthFileFiltersListByName(t *testing.T) {
	authDir := t.TempDir()
	const authName = "xai-filtered.json"
	authPath := filepath.Join(authDir, authName)
	credential := map[string]interface{}{
		"type":           "xai",
		"access_token":   "expired-access",
		"refresh_token":  "old-refresh",
		"expired":        "2020-01-01T00:00:00Z",
		"token_endpoint": "https://auth.x.ai/oauth2/token",
	}
	body, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authPath, body, 0o600); err != nil {
		t.Fatal(err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files":
			if got := r.URL.Query().Get("name"); got != authName {
				http.Error(w, "full auth file list is too large", http.StatusRequestEntityTooLarge)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"files": []map[string]interface{}{{
					"name": authName, "provider": "xai", "auth_index": "filtered-index",
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/api-call":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status_code": http.StatusOK,
				"body": map[string]interface{}{
					"access_token": "fresh-filtered", "expires_in": 3600,
				},
			})
		default:
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.String())
		}
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := NewManagementProxy(
		&fakeLeaseProvider{target: target, password: "runtime-secret"},
		&xaiQuotaTestStore{authDir: authDir},
		nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/refresh", strings.NewReader(`{"filename":"`+authName+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestManagementProxyRefreshAuthFileReportsListFailureCause(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v0/management/auth-files" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.String())
		}
		http.Error(w, "bad management credential", http.StatusUnauthorized)
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := NewManagementProxy(
		&fakeLeaseProvider{target: target, password: "runtime-secret"},
		&xaiQuotaTestStore{authDir: t.TempDir()},
		nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/refresh", strings.NewReader(`{"filename":"xai.json"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "auth files list returned status 401") {
		t.Fatalf("response did not include list failure cause: %s", rec.Body.String())
	}
}
