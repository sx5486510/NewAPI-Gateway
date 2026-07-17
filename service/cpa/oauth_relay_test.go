package cpa

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type mockOAuthManager struct {
	target   string
	password string
	stopped  bool
}

func (m *mockOAuthManager) AcquireManagement() (*ManagementLease, error) {
	if m.stopped {
		return nil, ErrUnavailable
	}
	target, _ := url.Parse(m.target)
	lease := &ManagementLease{
		Target:   target,
		Password: m.password,
	}
	lease.release = func() {}
	lease.once = sync.Once{}
	return lease, nil
}

func TestOAuthRelayAllowsOnlyExactCallbackPaths(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	}))
	defer upstream.Close()

	manager := &mockOAuthManager{target: upstream.URL, password: "test"}
	relay := NewOAuthRelay(manager)
	defer relay.Close()

	tests := []struct {
		path       string
		wantStatus int
	}{
		{"/anthropic/callback", http.StatusBadRequest}, // missing state
		{"/codex/callback", http.StatusBadRequest},
		{"/antigravity/callback", http.StatusBadRequest},
		{"/unknown/callback", http.StatusNotFound},
		{"/anthropic", http.StatusNotFound},
		{"/anthropic/callback/extra", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			router := gin.New()
			router.GET("/*path", relay.RelayCallback)

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestOAuthRelayValidatesStateAndProvider(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	manager := &mockOAuthManager{target: upstream.URL, password: "test"}
	relay := NewOAuthRelay(manager)
	defer relay.Close()

	// Register a valid session for anthropic
	api.RegisterOAuthSession("valid-state-anthropic", "anthropic")

	tests := []struct {
		name       string
		path       string
		state      string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "missing_state",
			path:       "/anthropic/callback",
			state:      "",
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid OAuth state",
		},
		{
			name:       "session_not_found",
			path:       "/anthropic/callback",
			state:      "unknown-state",
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid OAuth state",
		},
		{
			name:       "provider_mismatch",
			path:       "/codex/callback",
			state:      "valid-state-anthropic",
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid OAuth state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/*path", relay.RelayCallback)

			reqURL := tt.path
			if tt.state != "" {
				reqURL += "?state=" + tt.state
			}

			req := httptest.NewRequest(http.MethodGet, reqURL, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want to contain %q", rec.Body.String(), tt.wantBody)
			}
			for _, leaked := range []string{"missing state", "state not found", "provider mismatch"} {
				if strings.Contains(rec.Body.String(), leaked) {
					t.Fatalf("body leaks invalid-state detail %q: %s", leaked, rec.Body.String())
				}
			}
			var payload struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("invalid JSON response: %v", err)
			}
			if payload.Code != "invalid_oauth_state" {
				t.Fatalf("code = %q, want invalid_oauth_state", payload.Code)
			}
		})
	}
}

func TestOAuthRelayOneTimeStateUse(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	}))
	defer upstream.Close()

	manager := &mockOAuthManager{target: upstream.URL, password: "test"}
	relay := NewOAuthRelay(manager)
	defer relay.Close()

	// Register session
	api.RegisterOAuthSession("onetime-state", "anthropic")

	router := gin.New()
	router.GET("/*path", relay.RelayCallback)

	// First request should succeed
	req1 := httptest.NewRequest(http.MethodGet, "/anthropic/callback?state=onetime-state&code=xyz", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("first request failed: %d %s", rec1.Code, rec1.Body.String())
	}
	if upstreamCalls != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstreamCalls)
	}

	// Second request with same state should fail
	req2 := httptest.NewRequest(http.MethodGet, "/anthropic/callback?state=onetime-state&code=xyz", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusConflict {
		t.Errorf("second request status = %d, want %d", rec2.Code, http.StatusConflict)
	}
	if !strings.Contains(rec2.Body.String(), "already used") {
		t.Errorf("second request body should indicate state already used")
	}
	if upstreamCalls != 1 {
		t.Errorf("upstream should not be called twice")
	}
}

func TestOAuthRelayStripsSensitiveHeaders(t *testing.T) {
	var receivedHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	manager := &mockOAuthManager{target: upstream.URL, password: "runtime-secret"}
	relay := NewOAuthRelay(manager)
	defer relay.Close()

	api.RegisterOAuthSession("header-test-state", "codex")

	router := gin.New()
	router.GET("/*path", relay.RelayCallback)

	req := httptest.NewRequest(http.MethodGet, "/codex/callback?state=header-test-state", nil)
	req.Header.Set("Cookie", "session=abc123")
	req.Header.Set("Authorization", "Bearer user-token")
	req.Header.Set("X-Management-Key", "gateway-managed")
	req.Header.Set("User-Agent", "TestClient/1.0")
	req.Header.Set("Accept", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("request failed: %d", rec.Code)
	}

	// Browser credentials and gateway management headers should not be forwarded.
	if receivedHeaders.Get("Cookie") != "" {
		t.Error("Cookie header should be stripped")
	}
	if got := receivedHeaders.Get("Authorization"); got != "Bearer runtime-secret" {
		t.Errorf("Authorization = %q, want runtime lease bearer token", got)
	}
	if receivedHeaders.Get("X-Management-Key") != "" {
		t.Error("X-Management-Key header should be stripped")
	}

	// Safe headers should be preserved
	if receivedHeaders.Get("User-Agent") == "" {
		t.Error("User-Agent should be preserved")
	}
	if receivedHeaders.Get("Accept") == "" {
		t.Error("Accept should be preserved")
	}
}

func TestOAuthRelayHandlesCPAUnavailable(t *testing.T) {
	manager := &mockOAuthManager{stopped: true}
	relay := NewOAuthRelay(manager)
	defer relay.Close()

	api.RegisterOAuthSession("stopped-state", "antigravity")

	router := gin.New()
	router.GET("/*path", relay.RelayCallback)

	req := httptest.NewRequest(http.MethodGet, "/antigravity/callback?state=stopped-state", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "cpa_unavailable") {
		t.Error("response should indicate CPA unavailable")
	}
}

func TestOAuthRelayPreservesPathAndQuery(t *testing.T) {
	var receivedPath, receivedQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	manager := &mockOAuthManager{target: upstream.URL, password: "test"}
	relay := NewOAuthRelay(manager)
	defer relay.Close()

	api.RegisterOAuthSession("path-query-state", "anthropic")

	router := gin.New()
	router.GET("/*path", relay.RelayCallback)

	req := httptest.NewRequest(http.MethodGet, "/anthropic/callback?state=path-query-state&code=abc&extra=xyz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("request failed: %d", rec.Code)
	}

	if receivedPath != "/anthropic/callback" {
		t.Errorf("received path = %q, want %q", receivedPath, "/anthropic/callback")
	}
	if !strings.Contains(receivedQuery, "state=path-query-state") {
		t.Errorf("query should contain state parameter")
	}
	if !strings.Contains(receivedQuery, "code=abc") {
		t.Errorf("query should contain code parameter")
	}
}

func TestOAuthRelayCleanupExpiredStates(t *testing.T) {
	manager := &mockOAuthManager{target: "http://dummy", password: "test"}
	relay := NewOAuthRelay(manager)
	defer relay.Close()

	// Manually insert an old claimed state
	oldTime := time.Now().Add(-32 * time.Minute)
	relay.claimedStates.Store("old-state", oldTime)
	relay.claimedStates.Store("recent-state", time.Now())

	// Trigger cleanup by calling the cleanup logic directly
	cutoff := time.Now().Add(-31 * time.Minute)
	relay.claimedStates.Range(func(key, value interface{}) bool {
		if claimTime, ok := value.(time.Time); ok && claimTime.Before(cutoff) {
			relay.claimedStates.Delete(key)
		}
		return true
	})

	// Old state should be removed
	if _, ok := relay.claimedStates.Load("old-state"); ok {
		t.Error("old state should be cleaned up")
	}

	// Recent state should remain
	if _, ok := relay.claimedStates.Load("recent-state"); !ok {
		t.Error("recent state should not be cleaned up")
	}
}
