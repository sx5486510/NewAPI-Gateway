package middleware

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSameOriginForMutations(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		origin         string
		referer        string
		useForwarded   bool
		forwardedProto string
		want           int
		wantCode       string
	}{
		{"read without origin", http.MethodGet, "", "", true, "https", http.StatusNoContent, ""},
		{"head without origin", http.MethodHead, "", "", true, "https", http.StatusNoContent, ""},
		{"options without origin", http.MethodOptions, "", "", true, "https", http.StatusNoContent, ""},
		{"matching origin", http.MethodPost, "https://gateway.example", "", true, "https", http.StatusNoContent, ""},
		{"matching referer", http.MethodDelete, "", "https://gateway.example/cpa", true, "https", http.StatusNoContent, ""},
		{"matching origin case insensitive", http.MethodPut, "https://GATEWAY.EXAMPLE", "", true, "https", http.StatusNoContent, ""},
		{"matching with default https port", http.MethodPost, "https://gateway.example:443", "", true, "https", http.StatusNoContent, ""},
		{"matching with default http port", http.MethodPost, "http://gateway.example:80", "", false, "", http.StatusNoContent, ""},
		{"missing both for mutation", http.MethodPatch, "", "", true, "https", http.StatusForbidden, "origin_rejected"},
		{"foreign origin", http.MethodPut, "https://evil.example", "", true, "https", http.StatusForbidden, "origin_rejected"},
		{"foreign referer", http.MethodPost, "", "https://evil.example/cpa", true, "https", http.StatusForbidden, "origin_rejected"},
		{"malformed origin", http.MethodPost, "not-a-url", "", true, "https", http.StatusForbidden, "origin_rejected"},
		{"port mismatch", http.MethodPost, "https://gateway.example:8080", "", true, "https", http.StatusForbidden, "origin_rejected"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(SameOrigin())
			router.Any("/v0/management/config", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(tc.method, "http://gateway.example/v0/management/config", nil)
			req.Host = "gateway.example"
			if tc.useForwarded {
				req.Header.Set("X-Forwarded-Proto", tc.forwardedProto)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tc.want, rec.Body.String())
			}

			if tc.wantCode != "" {
				var payload struct {
					Success bool   `json:"success"`
					Code    string `json:"code"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
					t.Fatalf("unmarshal: %v, body=%s", err, rec.Body.String())
				}
				if payload.Code != tc.wantCode {
					t.Fatalf("code = %s, want %s", payload.Code, tc.wantCode)
				}
				if payload.Success {
					t.Fatalf("success should be false for error response")
				}
			}
		})
	}
}

func TestSameOriginWithDirectTLS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SameOrigin())
	router.POST("/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "https://gateway.example/test", nil)
	req.Host = "gateway.example"
	req.TLS = &tls.ConnectionState{} // non-nil TLS indicates direct HTTPS
	req.Header.Set("Origin", "https://gateway.example")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("direct TLS request failed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSameOriginRefererFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SameOrigin())
	router.POST("/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	// No Origin header, should fallback to Referer
	req := httptest.NewRequest(http.MethodPost, "https://gateway.example/test", nil)
	req.Host = "gateway.example"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Referer", "https://gateway.example/some/page")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("referer fallback failed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSameOriginMultipleForwardedProto(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SameOrigin())
	router.POST("/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	// Multiple X-Forwarded-Proto values should not be trusted
	req := httptest.NewRequest(http.MethodPost, "http://gateway.example/test", nil)
	req.Host = "gateway.example"
	req.Header.Set("X-Forwarded-Proto", "https, http")
	req.Header.Set("Origin", "http://gateway.example") // Should match http (no TLS, invalid forwarded)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("should accept http origin when forwarded-proto is invalid: %d %s", rec.Code, rec.Body.String())
	}
}
