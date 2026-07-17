package cpa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"NewAPI-Gateway/common"
)

// managementLeaseProvider abstracts the Manager's lease acquisition for testing.
type managementLeaseProvider interface {
	AcquireManagement() (*ManagementLease, error)
}

// snapshotPersister abstracts runtime config persistence for testing.
type snapshotPersister interface {
	PersistRuntime() error
}

// ManagementProxy is a transparent reverse proxy for the embedded CPA management
// API. It acquires a runtime lease, sanitizes browser credentials, injects the
// ephemeral management password, handles mutation persistence, and audits actions.
type ManagementProxy struct {
	manager      managementLeaseProvider
	store        snapshotPersister
	transport    http.RoundTripper
	scheduleSync func()
	auditf       func(string, ...interface{})
}

// persistenceError signals that CPA applied a mutation but Gateway persistence failed.
type persistenceError struct {
	cause error
}

func (e persistenceError) Error() string {
	return fmt.Sprintf("persistence failed: %v", e.cause)
}

// contextKey for audit username
type contextKey int

const auditUsernameKey contextKey = 1

// NewManagementProxy creates a management reverse proxy with the given lease
// provider, snapshot store, and sync scheduler.
func NewManagementProxy(
	manager managementLeaseProvider,
	store snapshotPersister,
	scheduleSync func(),
) *ManagementProxy {
	// Clone default transport with reasonable timeouts
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &ManagementProxy{
		manager:      manager,
		store:        store,
		transport:    transport,
		scheduleSync: scheduleSync,
		auditf: func(format string, args ...interface{}) {
			common.SysLog(fmt.Sprintf(format, args...))
		},
	}
}

// WithManagementAuditUser attaches the Root username to the request context
// for audit logging. The username is never forwarded to CPA.
func WithManagementAuditUser(r *http.Request, username string) *http.Request {
	ctx := context.WithValue(r.Context(), auditUsernameKey, username)
	return r.WithContext(ctx)
}

// ServeHTTP implements http.Handler. It acquires a lease, forwards the request
// to the embedded CPA with sanitized headers and runtime credentials, handles
// mutation persistence, and releases the lease after streaming completes.
func (p *ManagementProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	username := ""
	if u := r.Context().Value(auditUsernameKey); u != nil {
		username, _ = u.(string)
	}

	// Acquire management lease
	lease, err := p.manager.AcquireManagement()
	if err != nil {
		p.auditf("management proxy: user=%q method=%s path=%s status=503 duration=%v error=%v",
			username, r.Method, normalizePath(r.URL.Path), time.Since(start), err)
		writeJSONError(w, http.StatusServiceUnavailable, "cpa_unavailable", "CPA is not available")
		return
	}

	// Create per-request reverse proxy
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Rewrite target
			req.URL.Scheme = lease.Target.Scheme
			req.URL.Host = lease.Target.Host
			req.Host = lease.Target.Host

			// Sanitize and inject credentials
			sanitizeHeaders(req)
			req.Header.Set("Authorization", "Bearer "+lease.Password)
		},
		Transport: p.transport,
		ModifyResponse: func(resp *http.Response) error {
			// Handle mutation persistence and OAuth polling
			if isMutation(r.Method) && isSuccess(resp.StatusCode) {
				if p.store != nil {
					if err := p.store.PersistRuntime(); err != nil {
						return persistenceError{cause: err}
					}
				}
				if p.scheduleSync != nil {
					p.scheduleSync()
				}
			} else if isAuthStatusPoll(r, resp) {
				if shouldSyncAfterAuthStatus(resp) && p.scheduleSync != nil {
					p.scheduleSync()
				}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			status, code, message := mapProxyError(err)
			p.auditf("management proxy: user=%q method=%s path=%s status=%d duration=%v error=%v",
				username, r.Method, normalizePath(r.URL.Path), status, time.Since(start), err)
			writeJSONError(w, status, code, message)
		},
	}

	// Wrap response writer to release lease after streaming
	wrapped := &leaseReleaseWriter{
		ResponseWriter: w,
		lease:          lease,
		onFirstWrite: func() {
			p.auditf("management proxy: user=%q method=%s path=%s status=%d duration=%v",
				username, r.Method, normalizePath(r.URL.Path), 0, time.Since(start))
		},
	}

	proxy.ServeHTTP(wrapped, r)
}

// leaseReleaseWriter wraps http.ResponseWriter to release the lease after response
type leaseReleaseWriter struct {
	http.ResponseWriter
	lease        *ManagementLease
	onFirstWrite func()
	wroteHeader  bool
}

func (w *leaseReleaseWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		if w.onFirstWrite != nil {
			w.onFirstWrite()
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *leaseReleaseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	if err != nil || n == len(b) {
		// Release lease after successful write or error
		if w.lease != nil {
			w.lease.Release()
			w.lease = nil
		}
	}
	return n, err
}

// sanitizeHeaders removes sensitive browser headers and hop-by-hop headers
func sanitizeHeaders(req *http.Request) {
	// Remove sensitive credentials
	req.Header.Del("Authorization")
	req.Header.Del("X-Management-Key")
	req.Header.Del("Cookie")
	req.Header.Del("Proxy-Authorization")

	// Remove hop-by-hop headers
	req.Header.Del("Connection")
	req.Header.Del("Proxy-Connection")
	req.Header.Del("Keep-Alive")
	req.Header.Del("TE")
	req.Header.Del("Trailer")
	req.Header.Del("Transfer-Encoding")
	req.Header.Del("Upgrade")
}

// isMutation returns true for methods that modify state
func isMutation(method string) bool {
	return method == http.MethodPost || method == http.MethodPut ||
		method == http.MethodPatch || method == http.MethodDelete
}

// isSuccess returns true for 2xx status codes
func isSuccess(status int) bool {
	return status >= 200 && status < 300
}

// isAuthStatusPoll checks if this is a GET /v0/management/get-auth-status request
func isAuthStatusPoll(r *http.Request, resp *http.Response) bool {
	return r.Method == http.MethodGet &&
		strings.HasSuffix(r.URL.Path, "/get-auth-status") &&
		isSuccess(resp.StatusCode)
}

// shouldSyncAfterAuthStatus checks if the OAuth flow completed successfully
func shouldSyncAfterAuthStatus(resp *http.Response) bool {
	if resp.Body == nil || resp.ContentLength > 1<<20 {
		return false
	}

	// Buffer and restore body
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false
	}
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))

	// Check for completed status
	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return false
	}
	return status.Status == "completed"
}

// mapProxyError maps transport errors to stable HTTP status and error codes
func mapProxyError(err error) (status int, code, message string) {
	if err == nil {
		return http.StatusOK, "", ""
	}

	// Check for persistence failure
	var persistErr persistenceError
	if errors.As(err, &persistErr) {
		return http.StatusInternalServerError,
			"persistence_failed",
			"CPA applied the change but durable snapshot persistence failed"
	}

	// Check for specific transport errors
	errMsg := err.Error()
	if strings.Contains(errMsg, "connection refused") {
		return http.StatusBadGateway, "upstream_failure", "CPA connection failed"
	}
	if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline exceeded") {
		return http.StatusGatewayTimeout, "upstream_timeout", "CPA response timeout"
	}

	// Default to bad gateway
	return http.StatusBadGateway, "upstream_failure", "CPA request failed"
}

// normalizePath returns a redacted path for audit logging (no query params)
func normalizePath(path string) string {
	return path
}

// writeJSONError writes a stable Gateway error response
func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"code":    code,
		"message": message,
	})
}
