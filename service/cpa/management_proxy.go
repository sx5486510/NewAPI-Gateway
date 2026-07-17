package cpa

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httputil"
	"path"
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
	defer lease.Release()

	wrapped := &managementAuditWriter{
		ResponseWriter: w,
	}

	if handled := p.handleAuthFileUpload(wrapped, r, lease); handled {
		p.auditf("management proxy: user=%q method=%s path=%s status=%d duration=%v",
			username, r.Method, normalizePath(r.URL.Path), wrapped.status(), time.Since(start))
		return
	}

	// Handle auth file quota refresh
	if handled := p.handleAuthFileQuota(wrapped, r, lease); handled {
		p.auditf("management proxy: user=%q method=%s path=%s status=%d duration=%v",
			username, r.Method, normalizePath(r.URL.Path), wrapped.status(), time.Since(start))
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
			writeJSONError(w, status, code, message)
		},
	}

	proxy.ServeHTTP(wrapped, r)
	p.auditf("management proxy: user=%q method=%s path=%s status=%d duration=%v",
		username, r.Method, normalizePath(r.URL.Path), wrapped.status(), time.Since(start))
}

func (p *ManagementProxy) handleAuthFileUpload(w http.ResponseWriter, r *http.Request, lease *ManagementLease) bool {
	if !isAuthFileUpload(r) {
		return false
	}

	files, ok, err := uploadedAuthFiles(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_auth_file_upload", err.Error())
		return true
	}
	if !ok || len(files) == 0 {
		return false
	}

	existing, err := p.authFileNames(r.Context(), lease)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "auth_file_list_failed", "failed to check existing auth files")
		return true
	}

	seen := make(map[string]bool, len(existing)+len(files))
	for name := range existing {
		seen[name] = true
	}

	var duplicates []string
	var uploadQueue []authUploadFile
	for _, file := range files {
		if file.Name == "" {
			continue
		}
		if seen[file.Name] {
			duplicates = append(duplicates, file.Name)
			continue
		}
		seen[file.Name] = true
		uploadQueue = append(uploadQueue, file)
	}

	if len(uploadQueue) == 0 {
		if len(files) == 1 && len(duplicates) == 1 {
			writeJSONError(w, http.StatusConflict, "auth_file_exists", fmt.Sprintf("认证文件已存在: %s", duplicates[0]))
			return true
		}
		writeAuthUploadSummary(w, http.StatusOK, false, nil, duplicates)
		return true
	}

	uploaded := make([]string, 0, len(uploadQueue))
	for _, file := range uploadQueue {
		if err := p.uploadAuthFile(r.Context(), lease, file); err != nil {
			writeJSONError(w, http.StatusBadGateway, "auth_file_upload_failed", err.Error())
			return true
		}
		uploaded = append(uploaded, file.Name)
	}

	if p.store != nil {
		if err := p.store.PersistRuntime(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "persistence_failed", "CPA applied the change but durable snapshot persistence failed")
			return true
		}
	}
	if p.scheduleSync != nil {
		p.scheduleSync()
	}

	writeAuthUploadSummary(w, http.StatusOK, true, uploaded, duplicates)
	return true
}

// handleAuthFileQuota handles POST /v0/management/api-call proxy
// This is a transparent proxy to CPA's generic API call endpoint
func (p *ManagementProxy) handleAuthFileQuota(w http.ResponseWriter, r *http.Request, lease *ManagementLease) bool {
	// Only handle POST /v0/management/api-call
	if r.Method != http.MethodPost {
		return false
	}

	path := normalizePath(r.URL.Path)
	if path != "/v0/management/api-call" {
		return false
	}

	// Read request body
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "body_read_failed", "Failed to read request body")
		return true
	}
	defer r.Body.Close()

	// Forward to CPA's /v0/management/api-call endpoint
	target := *lease.Target
	target.Path = "/v0/management/api-call"
	target.RawQuery = ""
	target.Fragment = ""

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "request_creation_failed", err.Error())
		return true
	}

	// Copy headers and inject auth
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+lease.Password)

	resp, err := p.transport.RoundTrip(req)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_request_failed", err.Error())
		return true
	}
	defer resp.Body.Close()

	// Copy response status and headers
	w.Header().Set("Content-Type", "application/json")
	for key, values := range resp.Header {
		if key == "Content-Length" || key == "Transfer-Encoding" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Stream response body
	io.Copy(w, resp.Body)
	return true
}

// authFileNameOnly is the type returned by listAuthFiles
type authFileNameOnly struct {
	Name string `json:"name"`
}

// listAuthFiles returns all auth files from CPA
func (p *ManagementProxy) listAuthFiles(ctx context.Context, lease *ManagementLease) ([]authFileNameOnly, error) {
	target := *lease.Target
	target.Path = "/v0/management/auth-files"
	target.RawQuery = ""
	target.Fragment = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+lease.Password)

	resp, err := p.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if !isSuccess(resp.StatusCode) {
		return nil, fmt.Errorf("auth files list returned %d", resp.StatusCode)
	}

	var payload struct {
		Files []authFileNameOnly `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, err
	}

	return payload.Files, nil
}

type authUploadFile struct {
	Name string
	Body []byte
}

// managementAuditWriter records the final status while preserving common writer capabilities.
type managementAuditWriter struct {
	http.ResponseWriter
	wroteHeader bool
	statusCode  int
}

func (w *managementAuditWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.statusCode = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *managementAuditWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *managementAuditWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *managementAuditWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (w *managementAuditWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *managementAuditWriter) status() int {
	if w.statusCode != 0 {
		return w.statusCode
	}
	return http.StatusOK
}

func isAuthFileUpload(r *http.Request) bool {
	return r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/auth-files")
}

func uploadedAuthFileName(r *http.Request) (string, bool, error) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return "", false, err
	}
	boundary := params["boundary"]
	if boundary == "" {
		return "", false, nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", false, err
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		if part.FormName() == "file" && part.FileName() != "" {
			_ = part.Close()
			return part.FileName(), true, nil
		}
		_ = part.Close()
	}
}

func uploadedAuthFiles(r *http.Request) ([]authUploadFile, bool, error) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return nil, false, nil
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, false, nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, false, err
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var files []authUploadFile
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return files, len(files) > 0, nil
		}
		if err != nil {
			return nil, false, err
		}
		if part.FormName() != "file" || part.FileName() == "" {
			_ = part.Close()
			continue
		}

		partBody, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			return nil, false, err
		}

		if isZipUpload(part.FileName()) {
			zipFiles, err := authFilesFromZip(part.FileName(), partBody)
			if err != nil {
				return nil, false, err
			}
			files = append(files, zipFiles...)
			continue
		}

		files = append(files, authUploadFile{Name: part.FileName(), Body: partBody})
	}
}

func (p *ManagementProxy) authFileExists(ctx context.Context, lease *ManagementLease, name string) (bool, error) {
	target := *lease.Target
	target.Path = "/v0/management/auth-files"
	target.RawQuery = ""
	target.Fragment = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+lease.Password)

	resp, err := p.transport.RoundTrip(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if !isSuccess(resp.StatusCode) {
		return false, fmt.Errorf("auth files list returned %d", resp.StatusCode)
	}

	var payload struct {
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return false, err
	}
	for _, file := range payload.Files {
		if file.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (p *ManagementProxy) authFileNames(ctx context.Context, lease *ManagementLease) (map[string]struct{}, error) {
	target := *lease.Target
	target.Path = "/v0/management/auth-files"
	target.RawQuery = ""
	target.Fragment = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+lease.Password)

	resp, err := p.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if !isSuccess(resp.StatusCode) {
		return nil, fmt.Errorf("auth files list returned %d", resp.StatusCode)
	}

	var payload struct {
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, err
	}

	names := make(map[string]struct{}, len(payload.Files))
	for _, file := range payload.Files {
		if file.Name != "" {
			names[file.Name] = struct{}{}
		}
	}
	return names, nil
}

func authFilesFromZip(filename string, body []byte) ([]authUploadFile, error) {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip %s: %w", filename, err)
	}

	var files []authUploadFile
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		name := path.Base(entry.Name)
		if name == "." || name == "/" || name == "" || !strings.EqualFold(path.Ext(name), ".json") {
			continue
		}

		rc, err := entry.Open()
		if err != nil {
			return nil, fmt.Errorf("read zip entry %s: %w", entry.Name, err)
		}
		entryBody, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read zip entry %s: %w", entry.Name, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close zip entry %s: %w", entry.Name, closeErr)
		}

		files = append(files, authUploadFile{Name: name, Body: entryBody})
	}
	return files, nil
}

func isZipUpload(filename string) bool {
	return strings.EqualFold(path.Ext(filename), ".zip")
}

func (p *ManagementProxy) uploadAuthFile(ctx context.Context, lease *ManagementLease, file authUploadFile) error {
	target := *lease.Target
	target.Path = "/v0/management/auth-files"
	target.RawQuery = ""
	target.Fragment = ""

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", file.Name)
	if err != nil {
		return err
	}
	if _, err := part.Write(file.Body); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), &body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+lease.Password)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.transport.RoundTrip(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if !isSuccess(resp.StatusCode) {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if len(responseBody) > 0 {
			return fmt.Errorf("upload %s returned %d: %s", file.Name, resp.StatusCode, strings.TrimSpace(string(responseBody)))
		}
		return fmt.Errorf("upload %s returned %d", file.Name, resp.StatusCode)
	}
	return nil
}

func writeAuthUploadSummary(w http.ResponseWriter, status int, success bool, uploaded, duplicates []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         success,
		"uploaded":        uploaded,
		"duplicates":      duplicates,
		"uploaded_count":  len(uploaded),
		"duplicate_count": len(duplicates),
	})
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

	const maxAuthStatusBody = 1 << 20
	limited := io.LimitReader(resp.Body, maxAuthStatusBody+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		resp.Body = &prefixReadCloser{
			Reader: io.MultiReader(bytes.NewReader(body), resp.Body),
			Closer: resp.Body,
		}
		return false
	}
	if len(body) > maxAuthStatusBody {
		resp.Body = &prefixReadCloser{
			Reader: io.MultiReader(bytes.NewReader(body), resp.Body),
			Closer: resp.Body,
		}
		return false
	}
	_ = resp.Body.Close()
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

type prefixReadCloser struct {
	io.Reader
	io.Closer
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
