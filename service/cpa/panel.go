package cpa

import (
	_ "embed"
	"net/http"
)

// ManagementPanelVersion is the pinned version of the embedded management center.
const ManagementPanelVersion = "v1.18.3"

// ManagementPanelHash is the SHA-256 hash of the embedded panel.
const ManagementPanelHash = "941a49a619a719a59e4c7917c6888a53eb3f41a4fa2fbb5c1cc94f2d1fc9cd4b"

//go:embed assets/management.html
var embeddedManagementPanel []byte

// NewPanelHandler returns an http.Handler that serves the embedded management panel.
// The handler sets secure headers including CSP with frame-ancestors 'self'.
func NewPanelHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data: blob:; font-src 'self' data:; frame-ancestors 'self'")

		w.WriteHeader(http.StatusOK)
		w.Write(embeddedManagementPanel)
	})
}
