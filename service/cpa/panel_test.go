package cpa

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedManagementPanelHash(t *testing.T) {
	const expectedHash = "941a49a619a719a59e4c7917c6888a53eb3f41a4fa2fbb5c1cc94f2d1fc9cd4b"

	actualHash := fmt.Sprintf("%x", sha256.Sum256(embeddedManagementPanel))

	if actualHash != expectedHash {
		t.Fatalf("embedded panel SHA-256 mismatch:\nexpected: %s\ngot:      %s", expectedHash, actualHash)
	}
}

func TestEmbeddedManagementPanelVersion(t *testing.T) {
	const expectedVersion = "v1.18.3"

	if ManagementPanelVersion != expectedVersion {
		t.Fatalf("panel version mismatch: expected %s, got %s", expectedVersion, ManagementPanelVersion)
	}
}

func TestPanelHandlerHeaders(t *testing.T) {
	handler := NewPanelHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/cpa/panel", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Content-Type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", contentType, "text/html; charset=utf-8")
	}

	// Cache-Control
	cacheControl := rec.Header().Get("Cache-Control")
	if cacheControl != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", cacheControl, "no-store")
	}

	// X-Content-Type-Options
	nosniff := rec.Header().Get("X-Content-Type-Options")
	if nosniff != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want %q", nosniff, "nosniff")
	}

	// CSP with frame-ancestors 'self'
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Errorf("CSP missing frame-ancestors 'self': %q", csp)
	}

	// Body should be the embedded panel
	if rec.Body.Len() != len(embeddedManagementPanel) {
		t.Errorf("body length = %d, want %d", rec.Body.Len(), len(embeddedManagementPanel))
	}
}

func TestPanelHandlerReturnsEmbeddedContent(t *testing.T) {
	handler := NewPanelHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/cpa/panel", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.Bytes()
	if len(body) == 0 {
		t.Fatal("panel handler returned empty body")
	}

	// Verify it's HTML
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "<!DOCTYPE html>") && !strings.Contains(bodyStr, "<html") {
		t.Error("response does not appear to be HTML")
	}
}

func TestPanelHandlerImmutable(t *testing.T) {
	handler1 := NewPanelHandler()
	handler2 := NewPanelHandler()

	// Both handlers should serve the same content
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	handler1.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	handler2.ServeHTTP(rec2, req2)

	if rec1.Body.String() != rec2.Body.String() {
		t.Error("panel handlers returned different content")
	}
}
