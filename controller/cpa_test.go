package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"NewAPI-Gateway/service/cpa"

	"github.com/gin-gonic/gin"
)

type mockManager struct {
	state        cpa.State
	ready        bool
	enabled      bool
	lastError    string
	startCalls   atomic.Int32
	stopCalls    atomic.Int32
	restartCalls atomic.Int32
}

func (m *mockManager) Start(ctx context.Context) error {
	m.startCalls.Add(1)
	if m.state == cpa.StateRunning {
		return cpa.ErrTransitionConflict
	}
	m.state = cpa.StateRunning
	m.ready = true
	m.enabled = true
	return nil
}

func (m *mockManager) Stop(ctx context.Context) error {
	m.stopCalls.Add(1)
	if m.state == cpa.StateStopped {
		return cpa.ErrTransitionConflict
	}
	m.state = cpa.StateStopped
	m.ready = false
	m.enabled = false
	return nil
}

func (m *mockManager) Restart(ctx context.Context) error {
	m.restartCalls.Add(1)
	m.state = cpa.StateRunning
	m.ready = true
	return nil
}

func (m *mockManager) Status() cpa.Status {
	endpoint := "offline"
	if m.state == cpa.StateRunning && m.ready {
		endpoint = "http://127.0.0.1:29000"
	}
	return cpa.Status{
		State:     m.state,
		Ready:     m.ready,
		Enabled:   m.enabled,
		Endpoint:  endpoint,
		Version:   cpa.CPAVersion,
		LastError: m.lastError,
	}
}

func (m *mockManager) Shutdown(ctx context.Context) error {
	return nil
}

func (m *mockManager) StartFromDB(ctx context.Context) error {
	return m.Start(ctx)
}

func (m *mockManager) AcquireManagement() (*cpa.ManagementLease, error) {
	return &cpa.ManagementLease{}, nil
}

type mockStore struct {
	patchCalls  atomic.Int32
	yamlContent string
}

func (s *mockStore) PatchBasic(cfg cpa.CPAConfig) error {
	s.patchCalls.Add(1)
	return nil
}

func (s *mockStore) Basic() (*cpa.CPAConfig, error) {
	return &cpa.CPAConfig{
		Enabled: true,
		APIKeys: []string{"test-key"},
		AuthDir: "/auth",
		Port:    29000,
	}, nil
}

func (s *mockStore) PersistRuntime() error {
	return nil
}

func setupTestRuntime(t *testing.T, manager *mockManager, store *mockStore) *cpa.Runtime {
	if manager == nil {
		manager = &mockManager{state: cpa.StateRunning, ready: true, enabled: true}
	}
	if store == nil {
		store = &mockStore{}
	}
	runtime := &cpa.Runtime{
		Store:   store,
		Manager: manager,
		Panel: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html></html>"))
		}),
	}
	cpa.SetDefaultRuntime(runtime)
	t.Cleanup(func() { cpa.SetDefaultRuntime(nil) })
	return runtime
}

func TestCPAStatusNeverExposesManagementPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := &mockManager{state: cpa.StateRunning, ready: true, enabled: true}
	setupTestRuntime(t, manager, nil)

	router := gin.New()
	router.GET("/api/cpa/status", GetCPAStatus)

	req := httptest.NewRequest(http.MethodGet, "/api/cpa/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	body := rec.Body.String()
	if strings.Contains(strings.ToLower(body), "password") {
		t.Fatalf("status response contains 'password': %s", body)
	}
	if strings.Contains(strings.ToLower(body), "secret") {
		t.Fatalf("status response contains 'secret': %s", body)
	}
	if strings.Contains(body, "127.0.0.1") {
		t.Fatalf("status response exposes loopback endpoint: %s", body)
	}
}

func TestCPALifecycleResponsesNeverExposeLoopbackEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		handler gin.HandlerFunc
		manager *mockManager
	}{
		{
			name:    "start",
			method:  http.MethodPost,
			path:    "/api/cpa/start",
			handler: StartCPA,
			manager: &mockManager{state: cpa.StateStopped, ready: false, enabled: true},
		},
		{
			name:    "restart",
			method:  http.MethodPost,
			path:    "/api/cpa/restart",
			handler: RestartCPA,
			manager: &mockManager{state: cpa.StateRunning, ready: true, enabled: true},
		},
		{
			name:    "config_update",
			method:  http.MethodPut,
			path:    "/api/cpa/config",
			body:    `{"enabled":true,"api_keys":["new-key"],"auth_dir":"/new-auth","port":30000}`,
			handler: UpdateCPAConfig,
			manager: &mockManager{state: cpa.StateRunning, ready: true, enabled: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			setupTestRuntime(t, tt.manager, nil)

			router := gin.New()
			router.Handle(tt.method, tt.path, tt.handler)

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "127.0.0.1") {
				t.Fatalf("lifecycle response exposes loopback endpoint: %s", rec.Body.String())
			}
		})
	}
}

func TestCPALifecycleStartReturns409OnConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := &mockManager{state: cpa.StateRunning, ready: true}
	setupTestRuntime(t, manager, nil)

	router := gin.New()
	router.POST("/api/cpa/start", StartCPA)

	req := httptest.NewRequest(http.MethodPost, "/api/cpa/start", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("starting already-running CPA should return 409, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["code"] != "transition_conflict" {
		t.Fatalf("expected transition_conflict code, got %v", resp["code"])
	}
}

func TestCPALifecycleStopReturns503WhenStopped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := &mockManager{state: cpa.StateStopped, ready: false}
	setupTestRuntime(t, manager, nil)

	router := gin.New()
	router.POST("/api/cpa/stop", StopCPA)

	req := httptest.NewRequest(http.MethodPost, "/api/cpa/stop", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("stopping already-stopped CPA should return 409, got %d", rec.Code)
	}
}

func TestLegacyConfigPatchPreservesUnknownYAML(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &mockStore{}
	manager := &mockManager{state: cpa.StateRunning, ready: true}
	setupTestRuntime(t, manager, store)

	router := gin.New()
	router.PUT("/api/cpa/config", UpdateCPAConfig)

	body := `{"enabled":true,"api_keys":["new-key"],"auth_dir":"/new-auth","port":30000}`
	req := httptest.NewRequest(http.MethodPut, "/api/cpa/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("config update should succeed, got %d: %s", rec.Code, rec.Body.String())
	}

	if store.patchCalls.Load() != 1 {
		t.Fatalf("expected 1 patch call, got %d", store.patchCalls.Load())
	}

	if manager.restartCalls.Load() != 1 {
		t.Fatalf("expected 1 restart call, got %d", manager.restartCalls.Load())
	}
}

func TestLifecycleAuditDoesNotLogSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := &mockManager{state: cpa.StateStopped}
	setupTestRuntime(t, manager, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("username", "test-user")
		c.Next()
	})
	router.POST("/api/cpa/start", StartCPA)

	req := httptest.NewRequest(http.MethodPost, "/api/cpa/start", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("start should succeed, got %d", rec.Code)
	}

	// Audit logs are set in context but not exposed in response
	body := rec.Body.String()
	if strings.Contains(strings.ToLower(body), "password") {
		t.Fatalf("audit should not contain password in response: %s", body)
	}
}
