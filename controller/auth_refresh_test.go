package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"NewAPI-Gateway/service/cpa"

	"github.com/gin-gonic/gin"
)

func TestRefreshAuthTokenDelegatesDirectlyToManagementRefresh(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var seenMethod string
	var seenPath string
	runtime := &cpa.Runtime{
		Proxy: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seenMethod = r.Method
			seenPath = r.URL.Path
			if r.Method != http.MethodPost || r.URL.Path != "/v0/management/auth-files/refresh" {
				http.Error(w, "unexpected management request", http.StatusTeapot)
				return
			}
			var req struct {
				Filename string `json:"filename"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename != "xai.json" {
				http.Error(w, "bad request body", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"message":"Token refreshed successfully","data":{"filename":"xai.json","old_expired":"2020-01-01T00:00:00Z","new_expired":"2026-07-28T03:00:00Z","refreshed_at":"2026-07-28T02:00:00Z"}}`))
		}),
	}
	cpa.SetDefaultRuntime(runtime)
	t.Cleanup(func() { cpa.SetDefaultRuntime(nil) })

	router := gin.New()
	router.POST("/api/auth/refresh", RefreshAuthToken)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(`{"filename":"xai.json"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, management = %s %s", rec.Code, rec.Body.String(), seenMethod, seenPath)
	}
	if seenMethod != http.MethodPost || seenPath != "/v0/management/auth-files/refresh" {
		t.Fatalf("management request = %s %s", seenMethod, seenPath)
	}
	if !strings.Contains(rec.Body.String(), `"new_expired":"2026-07-28T03:00:00Z"`) {
		t.Fatalf("refresh response was not returned: %s", rec.Body.String())
	}
}
