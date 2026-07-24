package cpa

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type xaiQuotaTestStore struct {
	authDir string
}

func (s *xaiQuotaTestStore) PersistRuntime() error { return nil }

func (s *xaiQuotaTestStore) Basic() (*CPAConfig, error) {
	return &CPAConfig{AuthDir: s.authDir}, nil
}

type capturedAPICall struct {
	AuthIndex string            `json:"authIndex"`
	URL       string            `json:"url"`
	Header    map[string]string `json:"header"`
}

func TestManagementProxyXAIQuotaUsesCredentialIdentity(t *testing.T) {
	authDir := t.TempDir()
	const authName = "xai-valid.json"
	credential := map[string]interface{}{
		"type":          "xai",
		"access_token":  "valid-access",
		"refresh_token": "valid-refresh",
		"sub":           "subject-1",
		"expired":       time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	credentialBody, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, authName), credentialBody, 0o600); err != nil {
		t.Fatal(err)
	}

	var forwarded capturedAPICall
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"files": []map[string]interface{}{{
					"name": authName, "provider": "xai", "auth_index": "runtime-index",
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/api-call":
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err := json.Unmarshal(body, &forwarded); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status_code": http.StatusOK,
				"body":        map[string]interface{}{"config": map[string]interface{}{"creditUsagePercent": 25}},
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

	payload := `{"authIndex":"runtime-index","method":"GET","url":"https://cli-chat-proxy.grok.com/v1/billing?format=credits","header":{"Authorization":"Bearer $TOKEN$","x-xai-token-auth":"xai-grok-cli"}}`
	req := httptest.NewRequest(http.MethodPost, "/v0/management/api-call", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := forwarded.Header["Authorization"]; got != "Bearer valid-access" {
		t.Fatalf("Authorization = %q, want valid credential token", got)
	}
	if got := forwarded.Header["x-userid"]; got != "subject-1" {
		t.Fatalf("x-userid = %q, want subject-1", got)
	}
}

func TestIsXAIManagedURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://cli-chat-proxy.grok.com/v1/billing", true},
		{"https://cli-chat-proxy.grok.com/v1/billing?format=credits", true},
		{"https://cli-chat-proxy.grok.com/v1/responses", true},
		{"https://cli-chat-proxy.grok.com/v1/responses/compact", true},
		{"https://cli-chat-proxy.grok.com/v1/chat/completions", true},
		{"http://cli-chat-proxy.grok.com/v1/billing", false},
		{"https://evil.com/v1/responses", false},
		{"https://cli-chat-proxy.grok.com/v1/other", false},
		{"not a url", false},
	}
	for _, tc := range cases {
		if got := isXAIManagedURL(tc.url); got != tc.want {
			t.Errorf("isXAIManagedURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

// A Grok chat test message hits /v1/responses on the same host as the billing
// endpoint. The gateway must refresh/substitute the credential token there too,
// otherwise chat tests would send an unrefreshed $TOKEN$ and fail spuriously.
func TestManagementProxyXAIChatEndpointGetsCredentialToken(t *testing.T) {
	authDir := t.TempDir()
	const authName = "xai-chat.json"
	credential := map[string]interface{}{
		"type":          "xai",
		"access_token":  "chat-access",
		"refresh_token": "chat-refresh",
		"sub":           "chat-subject",
		"expired":       time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	credentialBody, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, authName), credentialBody, 0o600); err != nil {
		t.Fatal(err)
	}

	var forwarded capturedAPICall
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"files": []map[string]interface{}{{
					"name": authName, "provider": "xai", "auth_index": "chat-index",
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/api-call":
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err := json.Unmarshal(body, &forwarded); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status_code": http.StatusOK,
				"body":        map[string]interface{}{"id": "resp-1"},
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

	payload := `{"authIndex":"chat-index","method":"POST","url":"https://cli-chat-proxy.grok.com/v1/responses","header":{"Authorization":"Bearer $TOKEN$"},"data":"{}"}`
	req := httptest.NewRequest(http.MethodPost, "/v0/management/api-call", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := forwarded.Header["Authorization"]; got != "Bearer chat-access" {
		t.Fatalf("Authorization = %q, want credential token substituted on chat endpoint", got)
	}
}

func TestManagementProxyXAIQuotaRefreshesExpiredCredential(t *testing.T) {
	authDir := t.TempDir()
	const authName = "xai-expired.json"
	authPath := filepath.Join(authDir, authName)
	credential := map[string]interface{}{
		"type":           "xai",
		"access_token":   "expired-access",
		"refresh_token":  "old-refresh",
		"sub":            "subject-2",
		"expired":        time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		"token_endpoint": "https://auth.x.ai/oauth2/token",
		"note":           "keep-this-note",
	}
	credentialBody, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authPath, credentialBody, 0o600); err != nil {
		t.Fatal(err)
	}

	var forwarded capturedAPICall
	refreshCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"files": []map[string]interface{}{{
					"name": authName, "provider": "xai", "auth_index": "expired-index",
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/api-call":
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatal(readErr)
			}
			var call capturedAPICall
			if err := json.Unmarshal(body, &call); err != nil {
				t.Fatal(err)
			}
			if call.URL == "https://auth.x.ai/oauth2/token" {
				refreshCalls++
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status_code": http.StatusOK,
					"body": map[string]interface{}{
						"access_token": "fresh-access", "refresh_token": "fresh-refresh",
						"id_token": "fresh-id", "token_type": "Bearer", "expires_in": 3600,
					},
				})
				return
			}
			forwarded = call
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status_code": http.StatusOK,
				"body":        map[string]interface{}{"config": map[string]interface{}{"creditUsagePercent": 25}},
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
	payload := `{"authIndex":"expired-index","method":"GET","url":"https://cli-chat-proxy.grok.com/v1/billing","header":{"Authorization":"Bearer $TOKEN$"}}`
	req := httptest.NewRequest(http.MethodPost, "/v0/management/api-call", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if refreshCalls != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshCalls)
	}
	if got := forwarded.Header["Authorization"]; got != "Bearer fresh-access" {
		t.Fatalf("Authorization = %q, want fresh token", got)
	}
	updatedBody, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatal(err)
	}
	var updated map[string]interface{}
	if err := json.Unmarshal(updatedBody, &updated); err != nil {
		t.Fatal(err)
	}
	if updated["access_token"] != "fresh-access" || updated["refresh_token"] != "fresh-refresh" || updated["id_token"] != "fresh-id" {
		t.Fatalf("unexpected refreshed credential fields: %#v", updated)
	}
	if updated["note"] != "keep-this-note" {
		t.Fatalf("unrelated credential field was not preserved: %#v", updated["note"])
	}
	expiresAt, err := time.Parse(time.RFC3339, updated["expired"].(string))
	if err != nil || !expiresAt.After(time.Now()) {
		t.Fatalf("refreshed expiry = %#v, err = %v", updated["expired"], err)
	}
}

func TestManagementProxyXAIQuotaRefreshesCredentialWithoutAccessToken(t *testing.T) {
	authDir := t.TempDir()
	const authName = "xai-refresh-only.json"
	authPath := filepath.Join(authDir, authName)
	credential := map[string]interface{}{
		"type":           "xai",
		"refresh_token":  "refresh-only",
		"sub":            "subject-refresh-only",
		"expired":        time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		"token_endpoint": "https://auth.x.ai/oauth2/token",
	}
	credentialBody, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authPath, credentialBody, 0o600); err != nil {
		t.Fatal(err)
	}

	var forwarded capturedAPICall
	refreshCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"files": []map[string]interface{}{{
					"name": authName, "provider": "xai", "auth_index": "refresh-only-index",
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/api-call":
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatal(readErr)
			}
			var call capturedAPICall
			if err := json.Unmarshal(body, &call); err != nil {
				t.Fatal(err)
			}
			if call.URL == "https://auth.x.ai/oauth2/token" {
				refreshCalls++
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status_code": http.StatusOK,
					"body": map[string]interface{}{
						"access_token": "fresh-refresh-only", "refresh_token": "fresh-refresh-token",
						"id_token": "fresh-refresh-id", "token_type": "Bearer", "expires_in": 3600,
					},
				})
				return
			}
			forwarded = call
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status_code": http.StatusOK,
				"body":        map[string]interface{}{"id": "resp-2"},
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
	payload := `{"authIndex":"refresh-only-index","method":"GET","url":"https://cli-chat-proxy.grok.com/v1/billing","header":{"Authorization":"Bearer $TOKEN$"}}`
	req := httptest.NewRequest(http.MethodPost, "/v0/management/api-call", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if refreshCalls != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshCalls)
	}
	if got := forwarded.Header["Authorization"]; got != "Bearer fresh-refresh-only" {
		t.Fatalf("Authorization = %q, want fresh token", got)
	}
}

func TestManagementProxyXAIQuotaRefreshFailureLeavesCredentialUntouched(t *testing.T) {
	authDir := t.TempDir()
	const authName = "xai-refresh-failure.json"
	authPath := filepath.Join(authDir, authName)
	original := []byte(`{"type":"xai","access_token":"expired-secret","refresh_token":"refresh-secret","sub":"subject-failure","expired":"2020-01-01T00:00:00Z","token_endpoint":"https://auth.x.ai/oauth2/token","note":"unchanged"}`)
	if err := os.WriteFile(authPath, original, 0o600); err != nil {
		t.Fatal(err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"files": []map[string]interface{}{{
					"name": authName, "provider": "xai", "auth_index": "failure-index",
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/api-call":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status_code": http.StatusUnauthorized,
				"body":        "upstream leaked expired-secret refresh-secret",
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
	payload := `{"authIndex":"failure-index","method":"GET","url":"https://cli-chat-proxy.grok.com/v1/billing","header":{"Authorization":"Bearer $TOKEN$"}}`
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v0/management/api-call", strings.NewReader(payload)))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"auth_token_refresh_failed"`) {
		t.Fatalf("response does not contain stable error code: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"message":"xAI token refresh failed"`) {
		t.Fatalf("response does not contain refresh failure message: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "expired-secret") || strings.Contains(rec.Body.String(), "refresh-secret") {
		t.Fatalf("response leaked credential values: %s", rec.Body.String())
	}
	updated, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(updated, original) {
		t.Fatalf("credential changed after refresh failure\ngot:  %s\nwant: %s", updated, original)
	}
}

func TestSecureAuthFilePathRejectsTraversal(t *testing.T) {
	authDir := t.TempDir()
	for _, name := range []string{
		"../outside.json",
		`..\outside.json`,
		"nested/credential.json",
		filepath.Join(authDir, "absolute.json"),
		"credential.txt",
	} {
		t.Run(name, func(t *testing.T) {
			if path, err := secureAuthFilePath(authDir, name); err == nil {
				t.Fatalf("secureAuthFilePath(%q) = %q, want error", name, path)
			}
		})
	}

	want := filepath.Join(authDir, "credential.json")
	got, err := secureAuthFilePath(authDir, "credential.json")
	if err != nil || got != want {
		t.Fatalf("valid path = %q, %v; want %q", got, err, want)
	}
}

func TestManagementProxyXAIQuotaDeduplicatesConcurrentRefresh(t *testing.T) {
	authDir := t.TempDir()
	const authName = "xai-concurrent.json"
	credential := map[string]interface{}{
		"type": "xai", "access_token": "expired-access", "refresh_token": "old-refresh",
		"sub": "subject-concurrent", "expired": "2020-01-01T00:00:00Z",
		"token_endpoint": "https://auth.x.ai/oauth2/token",
	}
	body, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, authName), body, 0o600); err != nil {
		t.Fatal(err)
	}

	var refreshCalls atomic.Int32
	var billingCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"files": []map[string]interface{}{{
					"name": authName, "provider": "xai", "auth_index": "concurrent-index",
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/api-call":
			var call capturedAPICall
			if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
				t.Fatal(err)
			}
			if call.URL == "https://auth.x.ai/oauth2/token" {
				refreshCalls.Add(1)
				time.Sleep(75 * time.Millisecond)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status_code": http.StatusOK,
					"body": map[string]interface{}{
						"access_token": "fresh-concurrent", "refresh_token": "fresh-refresh", "expires_in": 3600,
					},
				})
				return
			}
			billingCalls.Add(1)
			if got := call.Header["Authorization"]; got != "Bearer fresh-concurrent" {
				t.Errorf("Authorization = %q, want fresh concurrent token", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status_code": http.StatusOK,
				"body":        map[string]interface{}{"config": map[string]interface{}{"creditUsagePercent": 25}},
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
	payload := `{"authIndex":"concurrent-index","method":"GET","url":"https://cli-chat-proxy.grok.com/v1/billing","header":{"Authorization":"Bearer $TOKEN$"}}`

	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v0/management/api-call", strings.NewReader(payload)))
			results <- rec
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for rec := range results {
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	if got := billingCalls.Load(); got != 2 {
		t.Fatalf("billing calls = %d, want 2", got)
	}
}

func TestManagementProxyXAIQuotaDiscoversMissingTokenEndpoint(t *testing.T) {
	authDir := t.TempDir()
	const authName = "xai-discovery.json"
	credential := map[string]interface{}{
		"type": "xai", "access_token": "expired-access", "refresh_token": "old-refresh",
		"sub": "subject-discovery", "expired": "2020-01-01T00:00:00Z",
	}
	body, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, authName), body, 0o600); err != nil {
		t.Fatal(err)
	}

	var discoveryCalls, refreshCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"files": []map[string]interface{}{{
					"name": authName, "provider": "xai", "auth_index": "discovery-index",
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/api-call":
			var call capturedAPICall
			if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
				t.Fatal(err)
			}
			switch call.URL {
			case "https://auth.x.ai/.well-known/openid-configuration":
				discoveryCalls++
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status_code": http.StatusOK,
					"body":        map[string]interface{}{"token_endpoint": "https://auth.x.ai/oauth2/token"},
				})
			case "https://auth.x.ai/oauth2/token":
				refreshCalls++
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status_code": http.StatusOK,
					"body":        map[string]interface{}{"access_token": "fresh-discovery", "expires_in": 3600},
				})
			default:
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status_code": http.StatusOK,
					"body":        map[string]interface{}{"config": map[string]interface{}{"creditUsagePercent": 25}},
				})
			}
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
	payload := `{"authIndex":"discovery-index","method":"GET","url":"https://cli-chat-proxy.grok.com/v1/billing","header":{"Authorization":"Bearer $TOKEN$"}}`
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v0/management/api-call", strings.NewReader(payload)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if discoveryCalls != 1 || refreshCalls != 1 {
		t.Fatalf("discovery calls = %d, refresh calls = %d; want 1 each", discoveryCalls, refreshCalls)
	}
}
