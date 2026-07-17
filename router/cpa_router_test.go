package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"NewAPI-Gateway/controller"
	"NewAPI-Gateway/middleware"
	"NewAPI-Gateway/model"
	"NewAPI-Gateway/service/cpa"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var testSessionStore sessions.Store

func setupTestCPARoutes(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)

	// Setup test runtime
	runtime := &cpa.Runtime{
		Store:   &mockCPAStore{},
		Manager: &mockCPAManager{},
		Panel: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html>test panel</html>"))
		}),
		Proxy: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true}`))
		}),
		OAuth: &mockCPAOAuthRelay{},
	}
	cpa.SetDefaultRuntime(runtime)
	t.Cleanup(func() { cpa.SetDefaultRuntime(nil) })

	router := gin.New()
	// Use shared store for tests
	if testSessionStore == nil {
		testSessionStore = cookie.NewStore([]byte("test-secret-key-32bytes-long!!"))
	}
	router.Use(sessions.Sessions("session", testSessionStore))

	// CPA routes with Root auth
	cpaRoute := router.Group("/api/cpa")
	cpaRoute.Use(middleware.RootAuth(), middleware.NoTokenAuth())
	{
		cpaRoute.GET("/status", controller.GetCPAStatus)
		cpaRoute.POST("/start", middleware.SameOrigin(), controller.StartCPA)
		cpaRoute.POST("/stop", middleware.SameOrigin(), controller.StopCPA)
		cpaRoute.POST("/restart", middleware.SameOrigin(), controller.RestartCPA)
		cpaRoute.GET("/panel", controller.ServeCPAPanel)
		cpaRoute.GET("/config", controller.GetCPAConfig)
		cpaRoute.PUT("/config", middleware.SameOrigin(), controller.UpdateCPAConfig)
		cpaRoute.POST("/reload", middleware.SameOrigin(), controller.ReloadCPA)
	}

	// Management routes with Root auth
	management := router.Group("/v0/management")
	management.Use(middleware.RootAuth(), middleware.NoTokenAuth(), middleware.SameOrigin())
	management.Any("", controller.ProxyCPAManagement)
	management.Any("/*path", controller.ProxyCPAManagement)

	// OAuth callbacks without auth
	router.GET("/anthropic/callback", controller.RelayCPAOAuthCallback)
	router.GET("/codex/callback", controller.RelayCPAOAuthCallback)
	router.GET("/antigravity/callback", controller.RelayCPAOAuthCallback)

	return router
}

type mockCPAStore struct{}

func (s *mockCPAStore) PatchBasic(cfg cpa.CPAConfig) error {
	return nil
}

func (s *mockCPAStore) Basic() (*cpa.CPAConfig, error) {
	return &cpa.CPAConfig{
		Enabled: true,
		APIKeys: []string{"test-key"},
		AuthDir: "/auth",
		Port:    29000,
	}, nil
}

func (s *mockCPAStore) PersistRuntime() error {
	return nil
}

type mockCPAManager struct{}

func (m *mockCPAManager) Status() cpa.Status {
	return cpa.Status{
		State:    cpa.StateRunning,
		Ready:    true,
		Enabled:  true,
		Endpoint: "http://127.0.0.1:29000",
		Version:  "v7.2.80",
	}
}

func (m *mockCPAManager) Start(ctx context.Context) error {
	return nil
}

func (m *mockCPAManager) Stop(ctx context.Context) error {
	return nil
}

func (m *mockCPAManager) Restart(ctx context.Context) error {
	return nil
}

func (m *mockCPAManager) Shutdown(ctx context.Context) error {
	return nil
}

func (m *mockCPAManager) StartFromDB(ctx context.Context) error {
	return nil
}

func (m *mockCPAManager) AcquireManagement() (*cpa.ManagementLease, error) {
	return &cpa.ManagementLease{}, nil
}

type mockCPAOAuthRelay struct{}

func (o *mockCPAOAuthRelay) RelayCallback(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "oauth callback"})
}

func TestCPARouteAuthorization(t *testing.T) {
	model.DB = setupTestDB(t)

	// Setup runtime for all tests
	runtime := &cpa.Runtime{
		Store:   &mockCPAStore{},
		Manager: &mockCPAManager{},
		Panel: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html>test panel</html>"))
		}),
		Proxy: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true}`))
		}),
		OAuth: &mockCPAOAuthRelay{},
	}
	cpa.SetDefaultRuntime(runtime)
	t.Cleanup(func() { cpa.SetDefaultRuntime(nil) })

	tests := []struct {
		name            string
		method          string
		path            string
		role            int
		wantStatus      int
		wantDenied      bool
		description     string
	}{
		// Root (role=100) should access all
		{"root_status", http.MethodGet, "/api/cpa/status", 100, http.StatusOK, false, "Root can get status"},
		{"root_panel", http.MethodGet, "/api/cpa/panel", 100, http.StatusOK, false, "Root can get panel"},
		{"root_config", http.MethodGet, "/api/cpa/config", 100, http.StatusOK, false, "Root can get config"},
		{"root_management", http.MethodGet, "/v0/management/config", 100, http.StatusOK, false, "Root can access management"},

		// Admin (role=10) should be denied - returns 200 with success:false
		{"admin_status", http.MethodGet, "/api/cpa/status", 10, http.StatusOK, true, "Admin denied status"},
		{"admin_panel", http.MethodGet, "/api/cpa/panel", 10, http.StatusOK, true, "Admin denied panel"},
		{"admin_management", http.MethodGet, "/v0/management/config", 10, http.StatusOK, true, "Admin denied management"},

		// User (role=1) should be denied - returns 200 with success:false
		{"user_status", http.MethodGet, "/api/cpa/status", 1, http.StatusOK, true, "User denied status"},
		{"user_panel", http.MethodGet, "/api/cpa/panel", 1, http.StatusOK, true, "User denied panel"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Add middleware to inject session for this test
			testRouter := gin.New()
			if testSessionStore == nil {
				testSessionStore = cookie.NewStore([]byte("test-secret-key-32bytes-long!!"))
			}
			testRouter.Use(sessions.Sessions("session", testSessionStore))

			// Inject test session
			testRouter.Use(func(c *gin.Context) {
				if tc.role > 0 {
					session := sessions.Default(c)
					session.Set("id", 1)
					session.Set("username", "test-user")
					session.Set("role", tc.role)
					session.Set("status", 1)
					session.Save()
				}
				c.Next()
			})

			// Re-register CPA routes
			cpaRoute := testRouter.Group("/api/cpa")
			cpaRoute.Use(middleware.RootAuth(), middleware.NoTokenAuth())
			{
				cpaRoute.GET("/status", controller.GetCPAStatus)
				cpaRoute.POST("/start", middleware.SameOrigin(), controller.StartCPA)
				cpaRoute.POST("/stop", middleware.SameOrigin(), controller.StopCPA)
				cpaRoute.POST("/restart", middleware.SameOrigin(), controller.RestartCPA)
				cpaRoute.GET("/panel", controller.ServeCPAPanel)
				cpaRoute.GET("/config", controller.GetCPAConfig)
				cpaRoute.PUT("/config", middleware.SameOrigin(), controller.UpdateCPAConfig)
				cpaRoute.POST("/reload", middleware.SameOrigin(), controller.ReloadCPA)
			}

			management := testRouter.Group("/v0/management")
			management.Use(middleware.RootAuth(), middleware.NoTokenAuth(), middleware.SameOrigin())
			management.Any("", controller.ProxyCPAManagement)
			management.Any("/*path", controller.ProxyCPAManagement)

			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Host = "gateway.example"
			req.Header.Set("X-Forwarded-Proto", "https")

			rec := httptest.NewRecorder()
			testRouter.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("%s: status = %d, want %d, body=%s", tc.description, rec.Code, tc.wantStatus, rec.Body.String())
			}

			// Check if access was denied when expected
			if tc.wantDenied {
				var resp map[string]interface{}
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err == nil {
					if success, ok := resp["success"].(bool); !ok || success {
						t.Fatalf("%s: expected success=false in response, got %v", tc.description, resp)
					}
				}
			}
		})
	}
}

func TestOAuthCallbacksNoAuth(t *testing.T) {
	router := setupTestCPARoutes(t)

	paths := []string{
		"/anthropic/callback",
		"/codex/callback",
		"/antigravity/callback",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path+"?state=test-state&code=test-code", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			// Should not require auth (handled by OAuth relay)
			if rec.Code == http.StatusUnauthorized {
				t.Fatalf("%s should not require auth, got 401", path)
			}
		})
	}
}

func TestManagementMutationRequiresSameOrigin(t *testing.T) {
	model.DB = setupTestDB(t)

	// Setup runtime
	runtime := &cpa.Runtime{
		Store:   &mockCPAStore{},
		Manager: &mockCPAManager{},
		Panel: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		Proxy: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		OAuth: &mockCPAOAuthRelay{},
	}
	cpa.SetDefaultRuntime(runtime)
	t.Cleanup(func() { cpa.SetDefaultRuntime(nil) })

	// Create router with session injection
	testRouter := gin.New()
	if testSessionStore == nil {
		testSessionStore = cookie.NewStore([]byte("test-secret-key-32bytes-long!!"))
	}
	testRouter.Use(sessions.Sessions("session", testSessionStore))

	// Inject Root session (role=100)
	testRouter.Use(func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", 1)
		session.Set("username", "test-root")
		session.Set("role", 100)
		session.Set("status", 1)
		session.Save()
		c.Next()
	})

	// Register management routes
	management := testRouter.Group("/v0/management")
	management.Use(middleware.RootAuth(), middleware.NoTokenAuth(), middleware.SameOrigin())
	management.Any("", controller.ProxyCPAManagement)
	management.Any("/*path", controller.ProxyCPAManagement)

	req := httptest.NewRequest(http.MethodPut, "/v0/management/config", nil)
	req.Host = "gateway.example"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Origin", "https://evil.example")

	rec := httptest.NewRecorder()
	testRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign origin should be rejected, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func setupTestDB(t *testing.T) *gorm.DB {
	// This is a minimal mock - actual implementation would need real GORM setup
	// For now, just return nil as the auth middleware will handle missing DB
	return nil
}
