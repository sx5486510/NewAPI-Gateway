package cpa

import (
	"context"
	"crypto/rand"
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// snapshotStore defines the interface for YAML config persistence.
type snapshotStore interface {
	PatchBasic(cfg CPAConfig) error
	Basic() (*CPAConfig, error)
	PersistRuntime() error
}

// lifecycleManager defines the interface for CPA lifecycle control.
type lifecycleManager interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error
	Shutdown(ctx context.Context) error
	StartFromDB(ctx context.Context) error
	Status() Status
	AcquireManagement() (*ManagementLease, error)
}

// oauthRelay defines the interface for OAuth callback relay.
type oauthRelay interface {
	RelayCallback(c *gin.Context)
}

// Runtime holds all components for embedded CPA management.
type Runtime struct {
	Store   snapshotStore
	Manager lifecycleManager
	Proxy   http.Handler
	OAuth   oauthRelay
	Panel   http.Handler
}

// LifecycleHooks is the interface expected by Manager for coordinator callbacks.
type coordinatorHooks interface {
	OnCPAReady(baseURL, apiKey string)
	OnCPAUnavailable()
	ScheduleCPASync()
}

// NewRuntime constructs a complete runtime with all management components.
func NewRuntime(runtimeDir string, coordinator coordinatorHooks) (*Runtime, error) {
	invariants, err := NewRuntimeInvariants(rand.Reader)
	if err != nil {
		return nil, err
	}

	store := NewSnapshotStore(runtimeDir, invariants)
	manager := NewManager(store, coordinator)

	proxy := NewManagementProxy(manager, store, func() {
		if coordinator != nil {
			coordinator.ScheduleCPASync()
		}
	})

	oauth := NewOAuthRelay(manager)

	return &Runtime{
		Store:   store,
		Manager: manager,
		Proxy:   proxy,
		OAuth:   oauth,
		Panel:   http.NotFoundHandler(), // Task 8 will replace with verified panel
	}, nil
}

var defaultRuntime atomic.Pointer[Runtime]

// SetDefaultRuntime atomically sets the process-wide runtime.
func SetDefaultRuntime(runtime *Runtime) {
	defaultRuntime.Store(runtime)
}

// DefaultRuntime returns the current process-wide runtime.
func DefaultRuntime() *Runtime {
	return defaultRuntime.Load()
}
