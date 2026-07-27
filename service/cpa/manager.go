package cpa

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// cpaModulePath is the Go module path of the embedded CLIProxyAPI dependency.
const cpaModulePath = "github.com/router-for-me/CLIProxyAPI/v7"

// CPAVersion is the resolved module version of the embedded CLIProxyAPI
// dependency. Prefer runtime/debug build info (keeps pace with go.mod); fall
// back to parsing the local go.mod because `go test` binaries on this toolchain
// often ship with an empty Deps list.
var CPAVersion = resolveCPAVersion()

func resolveCPAVersion() string {
	if version := cpaVersionFromBuildInfo(); version != "" {
		return version
	}
	if version := cpaVersionFromGoMod(); version != "" {
		return version
	}
	return "unknown"
}

func cpaVersionFromBuildInfo() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return ""
	}
	for _, dep := range info.Deps {
		if dep == nil || dep.Path != cpaModulePath {
			continue
		}
		// Prefer the replacement version when go.mod uses a replace directive
		// that still carries a release tag.
		if dep.Replace != nil {
			if version := strings.TrimSpace(dep.Replace.Version); version != "" {
				return version
			}
		}
		if version := strings.TrimSpace(dep.Version); version != "" {
			return version
		}
	}
	return ""
}

func cpaVersionFromGoMod() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 12; i++ {
		data, readErr := os.ReadFile(filepath.Join(dir, "go.mod"))
		if readErr == nil {
			if version := parseCPAVersionFromGoMod(data); version != "" {
				return version
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func parseCPAVersionFromGoMod(data []byte) string {
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		fields := strings.Fields(line)
		idx := 0
		if len(fields) >= 3 && fields[0] == "require" {
			idx = 1
		}
		if len(fields) < idx+2 || fields[idx] != cpaModulePath {
			continue
		}
		version := strings.TrimSpace(fields[idx+1])
		if version == "" || version == "//" {
			continue
		}
		return version
	}
	return ""
}

type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateError    State = "error"
)

type Status struct {
	Enabled   bool   `json:"enabled"`
	State     State  `json:"state"`
	Ready     bool   `json:"ready"`
	Version   string `json:"version"`
	Endpoint  string `json:"endpoint"`
	LastError string `json:"last_error,omitempty"`
}

var (
	ErrTransitionConflict = errors.New("cpa lifecycle transition already in progress")
	ErrUnavailable        = errors.New("cpa management unavailable")
)

type LifecycleHooks interface {
	OnCPAReady(baseURL, apiKey string)
	OnCPAUnavailable()
	ScheduleCPASync()
}

type ManagementLease struct {
	Target   *url.URL
	Password string
	release  func()
	once     sync.Once
}

func (l *ManagementLease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.release != nil {
			l.release()
			l.release = nil
		}
	})
}

type Manager struct {
	mu                 sync.RWMutex
	transition         chan struct{}
	state              State
	ready              bool
	enabled            bool
	lastError          string
	current            *EmbedResult
	target             *url.URL
	managementPassword string
	accepting          bool
	inflight           *sync.WaitGroup
	store              *SnapshotStore
	hooks              LifecycleHooks
	startEmbedded      func(string, string) (*EmbedResult, error)
	healthCheck        func(context.Context, string) error
	readyTimeout       time.Duration
	drainTimeout       time.Duration
	stopTimeout        time.Duration
}

func NewManager(store *SnapshotStore, hooks LifecycleHooks) *Manager {
	enabled := false
	if store != nil && store.options != nil {
		enabled = strings.EqualFold(strings.TrimSpace(store.options.Get("CPAEnabled")), "true")
	}
	return &Manager{
		transition:    make(chan struct{}, 1),
		inflight:      &sync.WaitGroup{},
		state:         StateStopped,
		enabled:       enabled,
		store:         store,
		hooks:         hooks,
		startEmbedded: StartEmbedded,
		healthCheck:   defaultHealthCheck,
		readyTimeout:  30 * time.Second,
		drainTimeout:  10 * time.Second,
		stopTimeout:   35 * time.Second,
	}
}

func (m *Manager) Start(ctx context.Context) error {
	if err := m.beginTransition(); err != nil {
		return err
	}
	defer m.endTransition()
	if err := m.setDesiredEnabled(true); err != nil {
		m.publishError(err, "")
		return err
	}
	return m.startLocked(ctx)
}

func (m *Manager) Stop(ctx context.Context) error {
	if err := m.beginTransition(); err != nil {
		return err
	}
	defer m.endTransition()
	stopErr := m.stopLocked(ctx)
	persistErr := m.setDesiredEnabled(false)
	if persistErr != nil {
		m.publishError(persistErr, "")
	}
	return errors.Join(stopErr, persistErr)
}

func (m *Manager) Shutdown(ctx context.Context) error {
	if err := m.beginTransition(); err != nil {
		return err
	}
	defer m.endTransition()
	return m.stopLocked(ctx)
}

func (m *Manager) Restart(ctx context.Context) error {
	if err := m.beginTransition(); err != nil {
		return err
	}
	defer m.endTransition()
	if err := m.stopLocked(ctx); err != nil {
		return err
	}
	enabled := m.desiredEnabled()
	m.mu.Lock()
	m.enabled = enabled
	m.mu.Unlock()
	if !enabled {
		return nil
	}
	return m.startLocked(ctx)
}

func (m *Manager) StartFromDB(ctx context.Context) error {
	if err := m.beginTransition(); err != nil {
		return err
	}
	defer m.endTransition()
	if m.store == nil {
		err := errors.New("cpa: snapshot store is nil")
		m.publishError(err, "")
		return err
	}
	basic, err := m.store.Basic()
	if err != nil {
		m.publishError(err, "")
		return err
	}
	m.mu.Lock()
	m.enabled = basic.Enabled
	m.mu.Unlock()
	if !basic.Enabled {
		m.publishStopped()
		m.notifyUnavailable()
		return nil
	}
	return m.startLocked(ctx)
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	endpoint := "offline"
	if m.state == StateRunning && m.ready && m.target != nil {
		endpoint = m.target.String()
	}
	return Status{
		Enabled:   m.enabled,
		State:     m.state,
		Ready:     m.ready,
		Version:   CPAVersion,
		Endpoint:  endpoint,
		LastError: m.lastError,
	}
}

func (m *Manager) AcquireManagement() (*ManagementLease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.accepting || m.state != StateRunning || !m.ready || m.target == nil || m.managementPassword == "" {
		return nil, ErrUnavailable
	}
	target := *m.target
	generation := m.inflight
	generation.Add(1)
	return &ManagementLease{
		Target:   &target,
		Password: m.managementPassword,
		release:  generation.Done,
	}, nil
}

func (m *Manager) startLocked(ctx context.Context) error {
	m.mu.RLock()
	alreadyRunning := m.state == StateRunning && m.ready && m.current != nil
	m.mu.RUnlock()
	if alreadyRunning {
		return nil
	}
	if m.store == nil {
		err := errors.New("cpa: snapshot store is nil")
		m.publishError(err, "")
		return err
	}

	m.mu.Lock()
	m.state = StateStarting
	m.ready = false
	m.accepting = false
	m.lastError = ""
	m.target = nil
	m.managementPassword = ""
	m.current = nil
	m.inflight = &sync.WaitGroup{}
	m.mu.Unlock()

	_, cfg, err := m.store.LoadOrMigrate()
	if err != nil {
		m.publishError(err, "")
		m.notifyUnavailable()
		return err
	}
	if err := ensureAuthDir(cfg.AuthDir); err != nil {
		m.publishError(err, "")
		m.notifyUnavailable()
		return err
	}
	if err := os.Chmod(expandHome(cfg.AuthDir), 0o700); err != nil {
		err = fmt.Errorf("cpa: secure auth dir: %w", err)
		m.publishError(err, "")
		m.notifyUnavailable()
		return err
	}

	managementPassword, err := generateManagementPassword()
	if err != nil {
		m.publishError(err, "")
		m.notifyUnavailable()
		return err
	}
	result, err := m.startEmbedded(m.store.Path(), managementPassword)
	if err != nil {
		err = sanitizeRuntimeError(err, managementPassword)
		managementPassword = ""
		m.publishError(err, "")
		m.notifyUnavailable()
		return err
	}
	if result == nil {
		err = errors.New("cpa: embedded start returned no runtime")
		managementPassword = ""
		m.publishError(err, "")
		m.notifyUnavailable()
		return err
	}
	target, err := url.Parse(result.BaseURL)
	if err != nil || target.Scheme != "http" || target.Host == "" {
		if err == nil {
			err = errors.New("invalid loopback target")
		}
		err = fmt.Errorf("cpa: parse embedded target: %w", err)
		return m.failStartedRuntime(ctx, result, managementPassword, err)
	}

	readyCtx, cancel := context.WithTimeout(ctx, m.readyTimeout)
	err = m.healthCheck(readyCtx, result.BaseURL)
	cancel()
	if err != nil {
		err = fmt.Errorf("cpa: readiness check failed: %w", err)
		return m.failStartedRuntime(ctx, result, managementPassword, err)
	}

	m.mu.Lock()
	m.current = result
	m.target = target
	m.managementPassword = managementPassword
	m.accepting = true
	m.ready = true
	m.state = StateRunning
	m.lastError = ""
	m.mu.Unlock()
	m.notifyReady(result.BaseURL, result.APIKey)
	go m.watchRuntime(result)
	return nil
}

func (m *Manager) failStartedRuntime(ctx context.Context, result *EmbedResult, password string, cause error) error {
	result.Cancel()
	if err := waitForSignal(ctx, result.Done, m.stopTimeout, "embedded stop"); err != nil {
		cause = errors.Join(cause, err)
	}
	cause = sanitizeRuntimeError(cause, password)
	password = ""
	m.publishError(cause, "")
	m.notifyUnavailable()
	return cause
}

func (m *Manager) stopLocked(ctx context.Context) error {
	m.mu.Lock()
	m.state = StateStopping
	m.ready = false
	m.accepting = false
	m.lastError = ""
	result := m.current
	password := m.managementPassword
	generation := m.inflight
	m.mu.Unlock()
	m.notifyUnavailable()

	drained := make(chan struct{})
	go func() {
		generation.Wait()
		close(drained)
	}()
	drainErr := waitForSignal(ctx, drained, m.drainTimeout, "management drain")

	var stopErr error
	if result != nil {
		result.Cancel()
		stopErr = waitForSignal(ctx, result.Done, m.stopTimeout, "embedded stop")
	}
	combined := errors.Join(drainErr, stopErr)
	if combined != nil {
		combined = sanitizeRuntimeError(combined, password)
	}

	m.mu.Lock()
	m.current = nil
	m.target = nil
	m.managementPassword = ""
	m.accepting = false
	m.ready = false
	if combined != nil {
		m.state = StateError
		m.lastError = combined.Error()
	} else {
		m.state = StateStopped
		m.lastError = ""
	}
	m.mu.Unlock()
	return combined
}

func (m *Manager) watchRuntime(result *EmbedResult) {
	<-result.Done
	var runtimeErr error
	if result.Errors != nil {
		for err := range result.Errors {
			if err != nil {
				runtimeErr = err
			}
		}
	}
	if runtimeErr == nil {
		runtimeErr = errors.New("cpa: embedded runtime exited unexpectedly")
	}

	m.mu.Lock()
	if m.current != result || m.state != StateRunning {
		m.mu.Unlock()
		return
	}
	runtimeErr = sanitizeRuntimeError(runtimeErr, m.managementPassword)
	m.current = nil
	m.target = nil
	m.managementPassword = ""
	m.accepting = false
	m.ready = false
	m.state = StateError
	m.lastError = runtimeErr.Error()
	m.mu.Unlock()
	m.notifyUnavailable()
}

func (m *Manager) beginTransition() error {
	select {
	case m.transition <- struct{}{}:
		return nil
	default:
		return ErrTransitionConflict
	}
}

func (m *Manager) endTransition() {
	<-m.transition
}

func (m *Manager) setDesiredEnabled(enabled bool) error {
	if m.store == nil || m.store.options == nil {
		return errors.New("cpa: snapshot option store is nil")
	}
	value := "false"
	if enabled {
		value = "true"
	}
	if err := m.store.options.Set("CPAEnabled", value); err != nil {
		return fmt.Errorf("cpa: persist CPAEnabled: %w", err)
	}
	m.mu.Lock()
	m.enabled = enabled
	m.mu.Unlock()
	return nil
}

func (m *Manager) desiredEnabled() bool {
	if m.store == nil || m.store.options == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(m.store.options.Get("CPAEnabled")), "true")
}

func (m *Manager) publishStopped() {
	m.mu.Lock()
	m.state = StateStopped
	m.ready = false
	m.accepting = false
	m.lastError = ""
	m.current = nil
	m.target = nil
	m.managementPassword = ""
	m.mu.Unlock()
}

func (m *Manager) publishError(err error, password string) {
	err = sanitizeRuntimeError(err, password)
	m.mu.Lock()
	m.state = StateError
	m.ready = false
	m.accepting = false
	m.current = nil
	m.target = nil
	m.managementPassword = ""
	m.lastError = err.Error()
	m.mu.Unlock()
}

func (m *Manager) notifyReady(baseURL, apiKey string) {
	if m.hooks != nil {
		m.hooks.OnCPAReady(baseURL, apiKey)
	}
}

func (m *Manager) notifyUnavailable() {
	if m.hooks != nil {
		m.hooks.OnCPAUnavailable()
	}
}

func (m *Manager) managementPasswordForTest() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.managementPassword
}

func generateManagementPassword() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("cpa: generate runtime management password: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(raw)
	clear(raw)
	return password, nil
}

type sanitizedError struct {
	message string
	cause   error
}

func (e *sanitizedError) Error() string { return e.message }
func (e *sanitizedError) Unwrap() error { return e.cause }

func sanitizeRuntimeError(err error, password string) error {
	if err == nil || password == "" || !strings.Contains(err.Error(), password) {
		return err
	}
	return &sanitizedError{
		message: strings.ReplaceAll(err.Error(), password, "[redacted]"),
		cause:   err,
	}
}

func waitForSignal(ctx context.Context, signal <-chan struct{}, timeout time.Duration, operation string) error {
	if signal == nil {
		return fmt.Errorf("cpa: %s signal is nil", operation)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-signal:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("cpa: %s interrupted: %w", operation, ctx.Err())
	case <-timer.C:
		return fmt.Errorf("cpa: %s timeout after %s", operation, timeout)
	}
}

func defaultHealthCheck(ctx context.Context, baseURL string) error {
	target := strings.TrimRight(baseURL, "/") + "/healthz"
	client := &http.Client{}
	var lastErr error
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
				return nil
			}
			lastErr = fmt.Errorf("health endpoint returned %d", response.StatusCode)
		} else {
			lastErr = err
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			if lastErr != nil {
				return fmt.Errorf("%v: %w", lastErr, ctx.Err())
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
