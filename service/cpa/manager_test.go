package cpa

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cpaconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func TestManagerHasNoPackageLevelLegacyLifecycleWrappers(t *testing.T) {
	source, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatalf("read manager.go: %v", err)
	}
	text := string(source)
	for _, forbidden := range []string{
		"func StartFromDB(",
		"func Stop()",
		"func Reload(",
		"func IsRunning()",
		"legacyManager",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("manager.go still contains legacy package-level lifecycle symbol %q", forbidden)
		}
	}
}

func TestManagerLifecycleAndSecretLifetime(t *testing.T) {
	m, fake := newFakeManager(t)
	if got := m.Status(); got.State != StateStopped || got.Ready || got.Endpoint != "offline" || got.Version != CPAVersion {
		t.Fatalf("initial status = %+v", got)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := m.Status(); got.State != StateRunning || !got.Ready || !got.Enabled || got.LastError != "" {
		t.Fatalf("started = %+v", got)
	}

	lease, err := m.AcquireManagement()
	if err != nil {
		t.Fatal(err)
	}
	if lease.Password == "" || lease.Target.String() != fake.baseURLValue() {
		t.Fatalf("bad lease: %+v", lease)
	}
	lease.Release()

	if err := m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AcquireManagement(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("acquire after stop = %v", err)
	}
	if m.managementPasswordForTest() != "" {
		t.Fatal("password retained after stop")
	}
	if got := m.Status(); got.State != StateStopped || got.Enabled || got.Endpoint != "offline" {
		t.Fatalf("stopped = %+v", got)
	}
}

func TestManagerRejectsConcurrentTransition(t *testing.T) {
	m, fake := newFakeManager(t)
	fake.blockStart()
	done := make(chan error, 1)
	go func() { done <- m.Start(context.Background()) }()
	fake.waitStartEntered(t)
	if err := m.Stop(context.Background()); !errors.Is(err, ErrTransitionConflict) {
		t.Fatalf("Stop = %v", err)
	}
	fake.releaseStart()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestManagerStopRejectsNewRequestsAndDrainsExistingLease(t *testing.T) {
	m, _ := newRunningFakeManager(t)
	lease, err := m.AcquireManagement()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- m.Stop(context.Background()) }()
	waitForManagerState(t, m, StateStopping)
	if _, err := m.AcquireManagement(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("new acquire = %v", err)
	}
	select {
	case err := <-done:
		t.Fatalf("stop did not wait for lease: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	lease.Release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestManagerLeaseDoubleRelease(t *testing.T) {
	m, _ := newRunningFakeManager(t)
	lease, err := m.AcquireManagement()
	if err != nil {
		t.Fatal(err)
	}
	lease.Release()
	lease.Release()
	if err := m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestManagerStartFailurePublishesErrorWithoutSecret(t *testing.T) {
	m, fake := newFakeManager(t)
	fake.leakPasswordInHealthError = true
	startErr := m.Start(context.Background())
	if startErr == nil {
		t.Fatal("expected start failure")
	}
	status := m.Status()
	if status.State != StateError || !strings.Contains(status.LastError, "not ready") {
		t.Fatalf("status = %+v", status)
	}
	if password := fake.lastPasswordValue(); password == "" || strings.Contains(status.LastError, password) || strings.Contains(startErr.Error(), password) {
		t.Fatalf("generated password leaked: status=%q error=%q", status.LastError, startErr)
	}
	if m.managementPasswordForTest() != "" {
		t.Fatal("password retained after failed start")
	}
}

func TestManagerHealthTimeoutIsBounded(t *testing.T) {
	m, fake := newFakeManager(t)
	m.readyTimeout = 30 * time.Millisecond
	fake.blockHealth = true
	started := time.Now()
	if err := m.Start(context.Background()); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Start = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("health timeout took %s", elapsed)
	}
	if got := m.Status(); got.State != StateError || got.Ready {
		t.Fatalf("status = %+v", got)
	}
}

func TestManagerDrainTimeoutIsBounded(t *testing.T) {
	m, _ := newRunningFakeManager(t)
	m.drainTimeout = 30 * time.Millisecond
	lease, err := m.AcquireManagement()
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err = m.Stop(context.Background())
	lease.Release()
	if err == nil || !strings.Contains(err.Error(), "drain") {
		t.Fatalf("Stop = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("drain timeout took %s", elapsed)
	}
	if got := m.Status(); got.State != StateError || got.Ready {
		t.Fatalf("status = %+v", got)
	}
}

func TestManagerStartAfterDrainTimeoutUsesNewLeaseGeneration(t *testing.T) {
	m, _ := newRunningFakeManager(t)
	m.drainTimeout = 20 * time.Millisecond
	oldLease, err := m.AcquireManagement()
	if err != nil {
		t.Fatal(err)
	}
	defer oldLease.Release()
	if err := m.Stop(context.Background()); err == nil || !strings.Contains(err.Error(), "drain") {
		t.Fatalf("first Stop = %v", err)
	}

	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	newLease, err := m.AcquireManagement()
	if err != nil {
		t.Fatal(err)
	}
	newLease.Release()
	m.drainTimeout = 100 * time.Millisecond
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("new runtime waited on old lease: %v", err)
	}
}

func TestManagerFailedStopEndsInError(t *testing.T) {
	m, fake := newRunningFakeManager(t)
	m.stopTimeout = 30 * time.Millisecond
	fake.setStopHangs(true)
	started := time.Now()
	err := m.Stop(context.Background())
	if err == nil || !strings.Contains(err.Error(), "stop") {
		t.Fatalf("Stop = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("stop timeout took %s", elapsed)
	}
	if got := m.Status(); got.State != StateError || got.Ready || got.Endpoint != "offline" {
		t.Fatalf("status = %+v", got)
	}
	fake.forceLatestExit(nil)
}

func TestManagerUnexpectedExitClearsPublishedRuntime(t *testing.T) {
	m, fake := newRunningFakeManager(t)
	fake.forceLatestExit(errors.New("unexpected failure"))
	waitForManagerState(t, m, StateError)
	if got := m.Status(); got.Ready || got.Endpoint != "offline" || !strings.Contains(got.LastError, "unexpected failure") {
		t.Fatalf("status = %+v", got)
	}
	if m.managementPasswordForTest() != "" {
		t.Fatal("password retained after unexpected exit")
	}
	if _, err := m.AcquireManagement(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("AcquireManagement = %v", err)
	}
}

func TestManagerStartFromDBDisabled(t *testing.T) {
	m, fake := newFakeManager(t)
	if err := m.StartFromDB(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.startCountValue() != 0 {
		t.Fatalf("start count = %d", fake.startCountValue())
	}
	if got := m.Status(); got.State != StateStopped || got.Enabled || got.Ready {
		t.Fatalf("status = %+v", got)
	}
	if fake.hooks.unavailable.Load() != 1 {
		t.Fatalf("unavailable hooks = %d", fake.hooks.unavailable.Load())
	}
}

func TestManagerRestartReadsChangedTargetPort(t *testing.T) {
	m, fake := newRunningFakeManager(t)
	before := m.Status().Endpoint
	beforePassword := fake.lastPasswordValue()
	basic, err := m.store.Basic()
	if err != nil {
		t.Fatal(err)
	}
	basic.Port = freePort(t)
	if err := m.store.PatchBasic(*basic); err != nil {
		t.Fatal(err)
	}
	if err := m.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := m.Status().Endpoint
	if before == after || after != fake.baseURLValue() {
		t.Fatalf("endpoints before=%q after=%q fake=%q", before, after, fake.baseURLValue())
	}
	if fake.startCountValue() != 2 {
		t.Fatalf("start count = %d", fake.startCountValue())
	}
	if afterPassword := fake.lastPasswordValue(); afterPassword == "" || afterPassword == beforePassword {
		t.Fatal("restart did not rotate the runtime management password")
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestManagerRestartRemainsStoppedWhenDisabled(t *testing.T) {
	m, fake := newRunningFakeManager(t)
	if err := m.store.options.Set("CPAEnabled", "false"); err != nil {
		t.Fatal(err)
	}
	if err := m.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := m.Status(); got.State != StateStopped || got.Enabled || got.Ready {
		t.Fatalf("status = %+v", got)
	}
	if fake.startCountValue() != 1 {
		t.Fatalf("restart started disabled CPA: %d", fake.startCountValue())
	}
}

func TestManagerShutdownPreservesDesiredEnabledState(t *testing.T) {
	m, _ := newRunningFakeManager(t)
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := m.Status(); got.State != StateStopped || !got.Enabled || got.Ready {
		t.Fatalf("status = %+v", got)
	}
	if value := m.store.options.Get("CPAEnabled"); value != "true" {
		t.Fatalf("CPAEnabled = %q", value)
	}
}

type fakeManagerRuntime struct {
	mu                        sync.Mutex
	baseURL                   string
	lastPassword              string
	startCount                int
	startEntered              chan struct{}
	startRelease              chan struct{}
	healthErr                 error
	leakPasswordInHealthError bool
	blockHealth               bool
	stopHangs                 bool
	processes                 []*fakeManagerProcess
	hooks                     *fakeLifecycleHooks
}

type fakeManagerProcess struct {
	done       chan struct{}
	errors     chan error
	cancelOnce sync.Once
	exitOnce   sync.Once
	owner      *fakeManagerRuntime
}

type fakeLifecycleHooks struct {
	ready       atomic.Int32
	unavailable atomic.Int32
	syncs       atomic.Int32
}

func (h *fakeLifecycleHooks) OnCPAReady(string, string) { h.ready.Add(1) }
func (h *fakeLifecycleHooks) OnCPAUnavailable()         { h.unavailable.Add(1) }
func (h *fakeLifecycleHooks) ScheduleCPASync()          { h.syncs.Add(1) }

func newFakeManager(t *testing.T) (*Manager, *fakeManagerRuntime) {
	t.Helper()
	port := freePort(t)
	authDir := filepath.Join(t.TempDir(), "auth")
	raw := fmt.Sprintf("host: 127.0.0.1\nport: %d\nauth-dir: %q\napi-keys: [manager-key]\n", port, authDir)
	options := newMemoryOptions(map[string]string{snapshotOptionKey: raw, "CPAEnabled": "false"})
	store := newTestSnapshotStore(t, options)
	hooks := &fakeLifecycleHooks{}
	fake := &fakeManagerRuntime{hooks: hooks}
	m := NewManager(store, hooks)
	m.startEmbedded = fake.startEmbedded
	m.healthCheck = fake.healthCheck
	m.readyTimeout = 200 * time.Millisecond
	m.drainTimeout = 200 * time.Millisecond
	m.stopTimeout = 200 * time.Millisecond
	return m, fake
}

func newRunningFakeManager(t *testing.T) (*Manager, *fakeManagerRuntime) {
	t.Helper()
	m, fake := newFakeManager(t)
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return m, fake
}

func (f *fakeManagerRuntime) startEmbedded(path, password string) (*EmbedResult, error) {
	f.mu.Lock()
	f.startCount++
	f.lastPassword = password
	entered, release := f.startEntered, f.startRelease
	f.mu.Unlock()
	if entered != nil {
		close(entered)
		<-release
	}

	cfg, err := cpaconfig.LoadConfig(path)
	if err != nil {
		return nil, err
	}
	target := (&url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", cfg.Port)}).String()
	process := &fakeManagerProcess{done: make(chan struct{}), errors: make(chan error, 1), owner: f}
	processCancel := func() {
		process.cancelOnce.Do(func() {
			f.mu.Lock()
			hangs := f.stopHangs
			f.mu.Unlock()
			if !hangs {
				process.exit(nil)
			}
		})
	}
	f.mu.Lock()
	f.baseURL = target
	f.processes = append(f.processes, process)
	f.mu.Unlock()
	apiKey := ""
	if len(cfg.APIKeys) > 0 {
		apiKey = cfg.APIKeys[0]
	}
	return &EmbedResult{Cancel: processCancel, Done: process.done, Errors: process.errors, BaseURL: target, APIKey: apiKey}, nil
}

func (f *fakeManagerRuntime) healthCheck(ctx context.Context, _ string) error {
	f.mu.Lock()
	block := f.blockHealth
	err := f.healthErr
	leakPassword := f.leakPasswordInHealthError
	password := f.lastPassword
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	if leakPassword {
		return fmt.Errorf("not ready with credential %s", password)
	}
	return err
}

func (f *fakeManagerRuntime) blockStart() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startEntered = make(chan struct{})
	f.startRelease = make(chan struct{})
}

func (f *fakeManagerRuntime) waitStartEntered(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	entered := f.startEntered
	f.mu.Unlock()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("start was not entered")
	}
}

func (f *fakeManagerRuntime) releaseStart() {
	f.mu.Lock()
	release := f.startRelease
	f.startEntered = nil
	f.startRelease = nil
	f.mu.Unlock()
	close(release)
}

func (f *fakeManagerRuntime) setStopHangs(value bool) {
	f.mu.Lock()
	f.stopHangs = value
	f.mu.Unlock()
}

func (f *fakeManagerRuntime) forceLatestExit(err error) {
	f.mu.Lock()
	if len(f.processes) == 0 {
		f.mu.Unlock()
		return
	}
	process := f.processes[len(f.processes)-1]
	f.mu.Unlock()
	process.exit(err)
}

func (p *fakeManagerProcess) exit(err error) {
	p.exitOnce.Do(func() {
		if err != nil {
			p.errors <- err
		}
		close(p.errors)
		close(p.done)
	})
}

func (f *fakeManagerRuntime) baseURLValue() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.baseURL
}

func (f *fakeManagerRuntime) lastPasswordValue() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastPassword
}

func (f *fakeManagerRuntime) startCountValue() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startCount
}

func waitForManagerState(t *testing.T, manager *Manager, want State) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if manager.Status().State == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("manager state = %s, want %s", manager.Status().State, want)
}
