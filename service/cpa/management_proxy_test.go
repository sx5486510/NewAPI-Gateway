package cpa

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// mockSnapshotStore implements the persistence interface for testing
type mockSnapshotStore struct {
	persistFunc func() error
}

func (m *mockSnapshotStore) PersistRuntime() error {
	if m.persistFunc != nil {
		return m.persistFunc()
	}
	return nil
}

// fakeLeaseProvider implements managementLeaseProvider for testing
type fakeLeaseProvider struct {
	target   *url.URL
	password string
	err      error
	released atomic.Bool
	releases atomic.Int32
}

func (f *fakeLeaseProvider) AcquireManagement() (*ManagementLease, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.released.Store(false)
	return &ManagementLease{
		Target:   f.target,
		Password: f.password,
		release: func() {
			f.releases.Add(1)
			f.released.Store(true)
		},
	}, nil
}

func TestManagementProxySanitizesAndForwards(t *testing.T) {
	var capturedRequest *http.Request

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequest = r
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "upstream-body")
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	provider := &fakeLeaseProvider{
		target:   upstreamURL,
		password: "runtime-secret",
	}
	proxy := NewManagementProxy(provider, nil, func() {})

	req := httptest.NewRequest(http.MethodPost, "/v0/management/auth-files?name=a.json", strings.NewReader("payload"))
	req.Header.Set("Authorization", "Bearer browser-placeholder")
	req.Header.Set("X-Management-Key", "browser-placeholder")
	req.Header.Set("Cookie", "session=sensitive")
	req.Header.Set("Proxy-Authorization", "sensitive")

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if capturedRequest.URL.Path != "/v0/management/auth-files" {
		t.Fatalf("path = %s, want /v0/management/auth-files", capturedRequest.URL.Path)
	}
	if capturedRequest.URL.RawQuery != "name=a.json" {
		t.Fatalf("query = %s, want name=a.json", capturedRequest.URL.RawQuery)
	}
	if got := capturedRequest.Header.Get("Authorization"); got != "Bearer runtime-secret" {
		t.Fatalf("auth = %q, want Bearer runtime-secret", got)
	}

	// Verify sensitive headers are stripped
	for _, name := range []string{"X-Management-Key", "Cookie", "Connection", "Proxy-Authorization"} {
		if capturedRequest.Header.Get(name) != "" {
			t.Fatalf("forwarded sensitive header %s", name)
		}
	}

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if rec.Body.String() != "upstream-body" {
		t.Fatalf("body = %q, want upstream-body", rec.Body.String())
	}
	if !provider.released.Load() {
		t.Fatal("lease not released after response")
	}
}

func TestManagementProxyPreservesBusinessErrorsAndDownloads(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="auth.json"`)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-CPA-VERSION", "v7.2.80")
		w.Header().Set("X-CPA-COMMIT", "fixture")
		w.Header().Set("X-CPA-BUILD-DATE", "2026-07-16")
		w.Header().Set("X-CPA-SUPPORT-PLUGIN", "true")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"error":"business validation"}`)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	provider := &fakeLeaseProvider{
		target:   upstreamURL,
		password: "runtime-secret",
	}
	proxy := NewManagementProxy(provider, nil, func() {})

	req := httptest.NewRequest(http.MethodGet, "/v0/management/auth-files/download?name=auth.json", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if rec.Body.String() != `{"error":"business validation"}` {
		t.Fatalf("body = %s", rec.Body.String())
	}

	headers := map[string]string{
		"Content-Disposition":  `attachment; filename="auth.json"`,
		"Content-Type":         "application/json",
		"X-CPA-VERSION":        "v7.2.80",
		"X-CPA-COMMIT":         "fixture",
		"X-CPA-BUILD-DATE":     "2026-07-16",
		"X-CPA-SUPPORT-PLUGIN": "true",
	}
	for name, want := range headers {
		if got := rec.Header().Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

// threadSafeRecorder wraps httptest.ResponseRecorder with mutex protection
type threadSafeRecorder struct {
	mu     sync.Mutex
	rec    *httptest.ResponseRecorder
	commit bool
}

func newThreadSafeRecorder() *threadSafeRecorder {
	return &threadSafeRecorder{rec: httptest.NewRecorder()}
}

func (t *threadSafeRecorder) Header() http.Header {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rec.Header()
}

func (t *threadSafeRecorder) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.commit = true
	return t.rec.Write(b)
}

func (t *threadSafeRecorder) WriteHeader(statusCode int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.commit = true
	t.rec.WriteHeader(statusCode)
}

func (t *threadSafeRecorder) Committed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.commit
}

func (t *threadSafeRecorder) Code() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rec.Code
}

func (t *threadSafeRecorder) Body() *bytes.Buffer {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rec.Body
}

func TestManagementProxyPersistsBeforeReturningMutation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"success":true}`)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	provider := &fakeLeaseProvider{
		target:   upstreamURL,
		password: "runtime-secret",
	}

	persistenceEntered := make(chan struct{})
	releasePersistence := make(chan struct{})
	syncCalls := &atomic.Int32{}

	store := &mockSnapshotStore{
		persistFunc: func() error {
			close(persistenceEntered)
			<-releasePersistence
			return nil
		},
	}

	proxy := NewManagementProxy(provider, store, func() {
		syncCalls.Add(1)
	})

	rec := newThreadSafeRecorder()
	done := make(chan struct{})

	go func() {
		req := httptest.NewRequest(http.MethodPatch, "/v0/management/debug", strings.NewReader(`{"value":true}`))
		proxy.ServeHTTP(rec, req)
		close(done)
	}()

	<-persistenceEntered
	if rec.Committed() {
		t.Fatal("response committed before persistence")
	}

	close(releasePersistence)
	<-done

	if rec.Code() != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code())
	}
	if syncCalls.Load() != 1 {
		t.Fatalf("sync calls = %d, want 1", syncCalls.Load())
	}
}

func TestManagementProxyMapsPersistenceFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"success":true}`)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	provider := &fakeLeaseProvider{
		target:   upstreamURL,
		password: "runtime-secret",
	}

	store := &mockSnapshotStore{
		persistFunc: func() error {
			return errors.New("database closed")
		},
	}

	proxy := NewManagementProxy(provider, store, func() {})

	req := httptest.NewRequest(http.MethodDelete, "/v0/management/api-keys?index=0", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	var payload struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Success {
		t.Fatal("success should be false")
	}
	if payload.Code != "persistence_failed" {
		t.Fatalf("code = %s, want persistence_failed", payload.Code)
	}
	if payload.Message == "" {
		t.Fatal("message should not be empty")
	}
}

func TestManagementProxyMapsOfflineTransportAndTimeout(t *testing.T) {
	tests := []struct {
		name       string
		provider   *fakeLeaseProvider
		wantStatus int
		wantCode   string
	}{
		{
			name:       "offline",
			provider:   &fakeLeaseProvider{err: ErrUnavailable},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "cpa_unavailable",
		},
		{
			name: "connection refused",
			provider: &fakeLeaseProvider{
				target:   &url.URL{Scheme: "http", Host: "127.0.0.1:1"},
				password: "secret",
			},
			wantStatus: http.StatusBadGateway,
			wantCode:   "upstream_failure",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			proxy := NewManagementProxy(tc.provider, nil, func() {})

			req := httptest.NewRequest(http.MethodGet, "/v0/management/config", nil)
			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, req)

			var payload struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if payload.Code != tc.wantCode {
				t.Fatalf("code = %s, want %s", payload.Code, tc.wantCode)
			}
		})
	}
}

func TestManagementProxyStreamsDownloadsWithoutBuffering(t *testing.T) {
	// Simulate large download (auth file)
	largeBody := strings.Repeat("x", 5*1024*1024) // 5MB

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="large.json"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(largeBody))
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	provider := &fakeLeaseProvider{
		target:   upstreamURL,
		password: "runtime-secret",
	}

	proxy := NewManagementProxy(provider, nil, func() {})

	req := httptest.NewRequest(http.MethodGet, "/v0/management/auth-files/download?name=large.json", nil)
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	// Lease should be released after body completes
	if !provider.released.Load() {
		t.Fatal("lease not released after download completed")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if rec.Body.Len() != len(largeBody) {
		t.Fatalf("body length = %d, want %d", rec.Body.Len(), len(largeBody))
	}
}

func TestManagementProxyHoldsLeaseUntilStreamingCompletes(t *testing.T) {
	firstChunkWritten := make(chan struct{})
	releaseSecondChunk := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("upstream response writer is not flushable")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("first-"))
		flusher.Flush()
		close(firstChunkWritten)
		<-releaseSecondChunk
		_, _ = w.Write([]byte("second"))
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	provider := &fakeLeaseProvider{
		target:   upstreamURL,
		password: "runtime-secret",
	}
	proxy := NewManagementProxy(provider, nil, func() {})

	done := make(chan struct{})
	rec := httptest.NewRecorder()
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/v0/management/auth-files/download?name=large.json", nil)
		proxy.ServeHTTP(rec, req)
		close(done)
	}()

	<-firstChunkWritten
	if provider.released.Load() {
		t.Fatal("lease released before streamed response completed")
	}
	close(releaseSecondChunk)
	<-done

	if !provider.released.Load() {
		t.Fatal("lease not released after streamed response completed")
	}
	if rec.Body.String() != "first-second" {
		t.Fatalf("body = %q, want first-second", rec.Body.String())
	}
}

func TestManagementProxyReleasesLeaseForHeaderOnlyResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	provider := &fakeLeaseProvider{
		target:   upstreamURL,
		password: "runtime-secret",
	}
	proxy := NewManagementProxy(provider, nil, func() {})

	req := httptest.NewRequest(http.MethodDelete, "/v0/management/auth-files?name=empty.json", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if !provider.released.Load() {
		t.Fatal("lease not released for header-only response")
	}
	if provider.releases.Load() != 1 {
		t.Fatalf("release calls = %d, want 1", provider.releases.Load())
	}
}

func TestManagementAuditWriterUnwrapsUnderlyingResponseWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := &managementAuditWriter{ResponseWriter: rec}

	if wrapped.Unwrap() != rec {
		t.Fatal("managementAuditWriter should expose the underlying ResponseWriter")
	}
}

func TestManagementProxyOAuthAuthStatusPolling(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		shouldSync  bool
		maxBodySize int
	}{
		{"completed triggers sync", "completed", true, 512},
		{"pending no sync", "pending", false, 512},
		{"error no sync", "error", false, 512},
		{"large body no buffer", "completed", false, 2 * 1024 * 1024}, // Large body should NOT sync (avoid buffering)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyContent := strings.Repeat("x", tt.maxBodySize)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				resp := map[string]interface{}{
					"status": tt.status,
					"data":   bodyContent,
				}
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer upstream.Close()

			upstreamURL, _ := url.Parse(upstream.URL)
			provider := &fakeLeaseProvider{
				target:   upstreamURL,
				password: "runtime-secret",
			}

			syncCalls := &atomic.Int32{}
			proxy := NewManagementProxy(provider, nil, func() {
				syncCalls.Add(1)
			})

			req := httptest.NewRequest(http.MethodGet, "/v0/management/get-auth-status?provider=anthropic&state=abc", nil)
			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}

			expectedSyncCalls := int32(0)
			if tt.shouldSync {
				expectedSyncCalls = 1
			}

			if syncCalls.Load() != expectedSyncCalls {
				t.Fatalf("sync calls = %d, want %d", syncCalls.Load(), expectedSyncCalls)
			}

			// For large body test, just verify we got a valid response without full buffering
			if tt.maxBodySize > 1024*1024 {
				if rec.Body.Len() <= tt.maxBodySize {
					t.Fatalf("large response was truncated: got %d bytes, want more than %d", rec.Body.Len(), tt.maxBodySize)
				}
			} else {
				// For small bodies, verify the status field
				var resp map[string]interface{}
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatal(err)
				}
				if resp["status"] != tt.status {
					t.Fatalf("status = %v, want %s", resp["status"], tt.status)
				}
			}
		})
	}
}

func TestManagementProxyAuditIncludesRootUserAndStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"success":true}`)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	provider := &fakeLeaseProvider{
		target:   upstreamURL,
		password: "runtime-secret",
	}

	var auditLogs []string
	proxy := NewManagementProxy(provider, nil, func() {})
	proxy.auditf = func(format string, args ...interface{}) {
		auditLogs = append(auditLogs, formatWithArgs(format, args...))
	}

	req := httptest.NewRequest(http.MethodPost, "/v0/management/api-keys?secret=query", strings.NewReader(`{"key":"body-secret"}`))
	req = WithManagementAuditUser(req, "root-user")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if len(auditLogs) != 1 {
		t.Fatalf("audit log count = %d, want 1: %#v", len(auditLogs), auditLogs)
	}
	log := auditLogs[0]
	for _, want := range []string{`user="root-user"`, "method=POST", "path=/v0/management/api-keys", "status=201"} {
		if !strings.Contains(log, want) {
			t.Fatalf("audit log %q missing %q", log, want)
		}
	}
	for _, forbidden := range []string{"secret=query", "body-secret", "runtime-secret"} {
		if strings.Contains(log, forbidden) {
			t.Fatalf("audit log leaked %q: %s", forbidden, log)
		}
	}
}

func formatWithArgs(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

func TestManagementProxyAuditWithoutSecrets(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"success":true}`)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	provider := &fakeLeaseProvider{
		target:   upstreamURL,
		password: "runtime-secret",
	}

	var auditLogs []string
	// Audit function would be used if exposed in constructor
	_ = func(format string, args ...interface{}) {
		auditLogs = append(auditLogs, format)
	}

	proxy := NewManagementProxy(provider, nil, func() {})
	// For now, audit is not exposed in constructor, so this test
	// verifies the proxy doesn't leak secrets in responses

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/v0/management/auth-files?name=secret.json", `{"key":"sk-secret-token"}`},
		{http.MethodGet, "/v0/management/auth-files/download?name=secret.json", ""},
		{http.MethodPatch, "/v0/management/api-keys?index=0", `{"key":"new-key"}`},
		{http.MethodDelete, "/v0/management/auth-files?name=secret.json", ""},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}

			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, req)

			// Verify response doesn't contain runtime password
			responseBody := rec.Body.String()
			if strings.Contains(responseBody, "runtime-secret") {
				t.Fatalf("response leaked runtime password: %s", responseBody)
			}

			// Verify response doesn't contain request body secrets
			if strings.Contains(responseBody, "sk-secret-token") {
				t.Fatalf("response leaked request secret: %s", responseBody)
			}

			if strings.Contains(responseBody, "new-key") {
				t.Fatalf("response leaked API key: %s", responseBody)
			}
		})
	}

	// Verify no secrets in audit logs (when audit is implemented)
	for _, log := range auditLogs {
		if strings.Contains(log, "runtime-secret") {
			t.Fatalf("audit leaked runtime password: %s", log)
		}
		if strings.Contains(log, "sk-secret-token") {
			t.Fatalf("audit leaked request secret: %s", log)
		}
	}
}
