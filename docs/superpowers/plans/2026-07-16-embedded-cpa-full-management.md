# Embedded CPA Full Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give Gateway Root users complete, durable control of the loopback-only embedded CPA v7.2.80 through the pinned official CPA Management Center v1.18.3.

**Architecture:** A `cpa.Runtime` owns full-YAML persistence, lifecycle state, the in-memory management password, request draining, the management reverse proxy, OAuth callback relays, and the embedded panel asset. Gateway keeps the existing basic CPA API compatible, adds Root-only lifecycle and panel APIs, and exposes CPA's complete `/v0/management` tree through a same-origin bridge that strips browser credentials and injects the private runtime password. Provider route availability is tracked separately from the operator's persisted provider status.

**Tech Stack:** Go 1.26, Gin, GORM options, CPA SDK v7.2.80, `gopkg.in/yaml.v3`, `golang.org/x/crypto/bcrypt`, React 18, React Router 6, Jest/React DOM test utilities, Playwright, official CPA Management Center v1.18.3.

---

## File Map

- `service/cpa/config.go`: basic four-field compatibility view and patching over the full YAML snapshot.
- `service/cpa/snapshot.go`: complete YAML validation, migration, Gateway-owned invariants, disk recovery, atomic materialization, and `CPAConfigYAML` persistence.
- `service/cpa/embed.go`: CPA SDK construction with loopback enforcement and the local management password.
- `service/cpa/manager.go`: serialized lifecycle state machine, readiness, target leases, request draining, and secret lifetime.
- `service/cpa/runtime.go`: construction and process-wide registration of the CPA runtime used by controllers.
- `service/cpa/management_proxy.go`: transparent management bridge, header sanitation, error mapping, persistence hook, audit, and sync callback.
- `service/cpa/oauth_relay.go`: exact callback allowlist and pending-state one-time relay.
- `service/cpa/panel.go`: embedded, hash-checked official panel handler.
- `service/cpa/assets/management.html`: reviewed upstream v1.18.3 single-file panel.
- `common/provider_runtime.go`: process-local provider availability registry; absence means available.
- `model/model_route.go`: filters runtime-unavailable providers during route selection.
- `service/cpa_provider.go`: embedded provider upsert plus lifecycle-aware, debounced synchronization coordinator.
- `middleware/same-origin.go`: Origin/Referer validation for state-changing browser requests.
- `controller/cpa.go`: basic config compatibility, lifecycle, status, panel, management, and callback handlers.
- `router/api-router.go`: Root-only CPA API and management routes plus the three public callback relays.
- `main.go`: creates one runtime, wires provider hooks, starts it, and shuts it down.
- `web/src/components/RootRoute.js`: client-side Root role guard.
- `web/src/pages/CPA/index.js`: lifecycle bar, offline/error states, panel session bootstrap, and iframe.
- `web/src/components/Layout.js`, `web/src/App.js`, `web/src/index.css`: Root-only navigation, route, and responsive full-height layout.
- Tests live beside each file named above; real-CPA tests use free loopback ports and temporary directories only.

## Fixed Contracts

Use these names and shapes consistently in every task:

```go
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
```

Gateway-generated failures use:

```json
{"success":false,"code":"cpa_unavailable","message":"CPA is not running"}
```

with stable codes `transition_conflict`, `cpa_unavailable`, `upstream_failure`, `upstream_timeout`, `persistence_failed`, `origin_rejected`, and `invalid_oauth_state`. CPA responses themselves remain byte-for-byte and status-for-status unchanged.

### Task 1: Full YAML Snapshot, Migration, And Recovery

**Files:**
- Modify: `model/option.go`
- Create: `model/option_test.go`
- Rewrite: `service/cpa/config.go`
- Create: `service/cpa/snapshot.go`
- Rewrite: `service/cpa/config_test.go`
- Create: `service/cpa/snapshot_test.go`
- Delete: `service/cpa/config_helpers_test.go` only after its useful path-expansion cases move into the new tests

- [ ] **Step 1: Write failing snapshot tests**

Create table-driven tests with an in-memory option store and temporary runtime directory. The test body must cover all of these exact assertions:

```go
func TestSnapshotStoreMigratesLegacyOptions(t *testing.T) {
	opts := newMemoryOptions(map[string]string{
		"CPAEnabled": "true", "CPAAPIKeys": `["key-a","key-b"]`,
		"CPAAuthDir": t.TempDir(), "CPAPort": "29001",
	})
	store := newTestSnapshotStore(t, opts)

	snapshot, cfg, err := store.LoadOrMigrate()
	if err != nil { t.Fatal(err) }
	if cfg.Host != loopbackHost || cfg.Port != 29001 { t.Fatalf("runtime address = %s:%d", cfg.Host, cfg.Port) }
	if cfg.RemoteManagement.AllowRemote || !cfg.RemoteManagement.DisableControlPanel || !cfg.RemoteManagement.DisableAutoUpdatePanel {
		t.Fatalf("runtime invariants missing: %+v", cfg.RemoteManagement)
	}
	if cfg.RemoteManagement.SecretKey == "" || !strings.HasPrefix(cfg.RemoteManagement.SecretKey, "$2") {
		t.Fatal("management sentinel is not a bcrypt hash")
	}
	if strings.Contains(string(snapshot), "user-selected-management-key") {
		t.Fatal("user-selected management key survived normalization")
	}
	if got := opts.Get("CPAConfigYAML"); got != string(snapshot) { t.Fatalf("snapshot not persisted") }
	assertFileMode(t, store.Path(), 0o600)
}

func TestSnapshotStoreRecoversValidDiskAfterInterruptedPersistence(t *testing.T) {
	store := newTestSnapshotStore(t, newMemoryOptions(map[string]string{"CPAConfigYAML": "host: 127.0.0.1\nport: 29001\napi-keys: [db]\nauth-dir: auth\n"}))
	disk := []byte("host: 127.0.0.1\nport: 29002\napi-keys: [disk]\nauth-dir: auth\n")
	writeTestRuntimeFile(t, store.Path(), disk)

	got, cfg, err := store.LoadOrMigrate()
	if err != nil { t.Fatal(err) }
	if cfg.Port != 29002 || cfg.APIKeys[0] != "disk" { t.Fatalf("did not recover disk: %s", got) }
	if store.options.Get("CPAConfigYAML") != string(got) { t.Fatal("recovered disk was not saved to DB") }
}

func TestSnapshotStoreRejectsInvalidDatabaseAndDisk(t *testing.T) {
	store := newTestSnapshotStore(t, newMemoryOptions(map[string]string{"CPAConfigYAML": "[not-a-mapping]"}))
	writeTestRuntimeFile(t, store.Path(), []byte("port: nope"))
	if _, _, err := store.LoadOrMigrate(); err == nil { t.Fatal("expected invalid snapshot error") }
}

func TestPatchBasicPreservesUnknownAndPluginConfiguration(t *testing.T) {
	raw := []byte("host: 0.0.0.0\nport: 18317\nauth-dir: old\napi-keys: [old]\nplugins:\n  instances:\n    demo:\n      custom-field: keep-me\nunknown-future-field: 42\n")
	store := newTestSnapshotStore(t, newMemoryOptions(map[string]string{"CPAConfigYAML": string(raw)}))
	err := store.PatchBasic(CPAConfig{Enabled: false, APIKeys: []string{"new"}, AuthDir: "new-auth", Port: 29003})
	if err != nil { t.Fatal(err) }
	got := store.options.Get("CPAConfigYAML")
	for _, want := range []string{"custom-field: keep-me", "unknown-future-field: 42", "port: 29003", "new-auth"} {
		if !strings.Contains(got, want) { t.Fatalf("snapshot lost %q:\n%s", want, got) }
	}
}

func TestAtomicWriteLeavesOriginalOnRenameFailure(t *testing.T) {
	store := newTestSnapshotStore(t, newMemoryOptions(map[string]string{"CPAConfigYAML": "host: 127.0.0.1\nport: 29001\napi-keys: [old]\nauth-dir: auth\n"}))
	if _, _, err := store.LoadOrMigrate(); err != nil { t.Fatal(err) }
	original, err := os.ReadFile(store.Path())
	if err != nil { t.Fatal(err) }
	store.renameFile = func(string, string) error { return errors.New("rename failed") }
	if err := store.PatchBasic(CPAConfig{Enabled: true, APIKeys: []string{"new"}, AuthDir: t.TempDir(), Port: 29002}); err == nil {
		t.Fatal("expected rename error")
	}
	after, err := os.ReadFile(store.Path())
	if err != nil { t.Fatal(err) }
	if !bytes.Equal(after, original) { t.Fatal("original runtime file changed") }
	if _, err := os.Stat(store.Path()+".tmp"); !errors.Is(err, os.ErrNotExist) { t.Fatalf("temporary file remains: %v", err) }
}
```

Also retain tests for `expandHome`, empty auth directory rejection, default legacy values, invalid port, empty API keys, and YAML documents with duplicate keys.

- [ ] **Step 2: Run the tests and verify the expected failure**

Run: `go test ./model ./service/cpa -run 'Test(SnapshotStore|PatchBasic|AtomicWrite|ExpandHome|UpdateOptionReturnsDatabaseError)' -count=1`

Expected: FAIL because `SnapshotStore`, `LoadOrMigrate`, `PatchBasic`, and `CPAConfigYAML` do not exist.

- [ ] **Step 3: Implement the snapshot store and compatibility view**

Add `CPAConfigYAML` to `model.InitOptionMap()` with an empty default. Implement these exact public contracts:

```go
const snapshotOptionKey = "CPAConfigYAML"

type optionStore interface {
	Get(key string) string
	Set(key, value string) error
}

type SnapshotStore struct {
	options     optionStore
	runtimeDir string
	invariants *RuntimeInvariants
	renameFile func(oldPath, newPath string) error
}

func NewSnapshotStore(runtimeDir string, invariants *RuntimeInvariants) *SnapshotStore
func (s *SnapshotStore) Path() string
func (s *SnapshotStore) LoadOrMigrate() ([]byte, *cpaconfig.Config, error)
func (s *SnapshotStore) PersistRuntime() error
func (s *SnapshotStore) PatchBasic(next CPAConfig) error
func (s *SnapshotStore) Basic() (*CPAConfig, error)

type RuntimeInvariants struct {
	managementSentinelHash string
}

func NewRuntimeInvariants(random io.Reader) (*RuntimeInvariants, error)
func (i *RuntimeInvariants) ApplyYAML(raw []byte) ([]byte, *cpaconfig.Config, error)
```

Implement the constructor in this task, not later:

```go
func NewRuntimeInvariants(random io.Reader) (*RuntimeInvariants, error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(random, raw); err != nil { return nil, fmt.Errorf("cpa: generate management sentinel: %w", err) }
	encoded := make([]byte, base64.RawURLEncoding.EncodedLen(len(raw)))
	base64.RawURLEncoding.Encode(encoded, raw)
	clear(raw)
	hash, err := bcrypt.GenerateFromPassword(encoded, bcrypt.DefaultCost)
	clear(encoded)
	if err != nil { return nil, fmt.Errorf("cpa: hash management sentinel: %w", err) }
	return &RuntimeInvariants{managementSentinelHash: string(hash)}, nil
}
```

`ApplyYAML` replaces any user-supplied management secret with `managementSentinelHash`; no plaintext corresponding to that hash is retained after the constructor returns.

`NewSnapshotStore` resolves an empty runtime directory from `CPA_RUNTIME_DIR`, then defaults to `cpa`. `Path()` is `<runtimeDir>/config.yaml`. `LoadOrMigrate` must:

1. parse YAML as a single mapping node and reject aliases, duplicate keys, and multiple documents;
2. validate the normalized bytes with `cpaconfig.ParseConfigBytes`;
3. prefer a valid existing disk file over a different valid DB snapshot and persist that recovery copy;
4. fall back to a valid DB snapshot when disk is absent or invalid;
5. migrate the four legacy options only when `CPAConfigYAML` is empty;
6. apply Gateway invariants through `RuntimeInvariants.ApplyYAML` before both DB and disk writes;
7. create the runtime directory with mode `0700`, write `config.yaml.tmp` with mode `0600`, sync, close, rename, and remove the temp file on failure.

`PatchBasic` changes only `port`, `auth-dir`, and `api-keys` YAML nodes, stores `CPAEnabled` separately, and leaves every other node and comment intact. `Basic` reads enabled from `CPAEnabled` and the other fields from the validated full snapshot. Keep `SaveCPAConfigToDB`, `LoadCPAConfigFromDB`, and `MaterializeCPAConfigFromDB` as compatibility wrappers around the default store until Task 7 switches callers.

`model.UpdateOption` must return errors from both `FirstOrCreate` and `Save` instead of always returning nil. `model/option_test.go` closes a temporary SQLite database before calling `UpdateOption("CPAConfigYAML", "x")` and asserts a nonnil error; this makes the proxy's later `persistence_failed` branch observable rather than theoretical.

- [ ] **Step 4: Run focused and race tests**

Run: `go test -race ./model ./service/cpa -run 'Test(SnapshotStore|PatchBasic|AtomicWrite|ExpandHome|LoadCPAConfig|UpdateOptionReturnsDatabaseError)' -count=1`

Expected: PASS. Confirm no test writes `.smoke/cpa-config.yaml`.

- [ ] **Step 5: Commit**

```bash
git add model/option.go model/option_test.go service/cpa/config.go service/cpa/config_test.go service/cpa/snapshot.go service/cpa/snapshot_test.go service/cpa/config_helpers_test.go
git commit -m "feat(cpa): persist complete CPA configuration"
```

### Task 2: Runtime Invariants And Secure Embedded Management

**Files:**
- Modify: `service/cpa/snapshot.go`
- Rewrite: `service/cpa/embed.go`
- Create: `service/cpa/embed_test.go`
- Remove: `service/cpa/embed_poc_test.go` after moving its real-start coverage

- [ ] **Step 1: Write failing invariant and real-CPA authentication tests**

```go
func TestRuntimeInvariantsForceGatewayOwnedFields(t *testing.T) {
	inv, err := NewRuntimeInvariants(rand.Reader)
	if err != nil { t.Fatal(err) }
	raw := []byte("host: 0.0.0.0\nport: 29004\nremote-management:\n  allow-remote: true\n  secret-key: user-value\n  disable-control-panel: false\n  disable-auto-update-panel: false\n")
	got, cfg, err := inv.ApplyYAML(raw)
	if err != nil { t.Fatal(err) }
	if cfg.Host != "127.0.0.1" || cfg.RemoteManagement.AllowRemote || !cfg.RemoteManagement.DisableControlPanel || !cfg.RemoteManagement.DisableAutoUpdatePanel {
		t.Fatalf("invariants not applied: %s", got)
	}
	if strings.Contains(string(got), "user-value") { t.Fatal("browser-selected management key survived") }
}

func TestStartEmbeddedAcceptsOnlyRuntimeManagementPassword(t *testing.T) {
	path, port := writeRealCPAConfig(t)
	res, err := StartEmbedded(path, "runtime-secret")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { res.Cancel(); <-res.Done })
	waitHealth(t, res.BaseURL)

	assertManagementStatus(t, res.BaseURL, "runtime-secret", http.StatusOK)
	assertManagementStatus(t, res.BaseURL, "gateway-managed", http.StatusUnauthorized)
	if _, exposed := reflect.TypeOf(*res).FieldByName("ManagementPassword"); exposed {
		t.Fatal("EmbedResult exposes management password")
	}
}
```

The normal config fixture must contain a nonempty bcrypt `remote-management.secret-key`. Add a regression test that deliberately bypasses `StartEmbedded`, builds CPA directly with `cliproxy.NewBuilder().WithConfig(emptySecretConfig).WithLocalManagementPassword("runtime-secret")`, and asserts CPA v7.2.80 returns `403 remote management key not set`. Separately assert `StartEmbedded` rejects an empty sentinel before launching. This documents why the invariant is mandatory without weakening the production entry point.

- [ ] **Step 2: Run the tests and verify failure**

Run: `go test ./service/cpa -run 'Test(RuntimeInvariants|StartEmbedded)' -count=1 -v`

Expected: FAIL because the current builder omits `WithLocalManagementPassword` and accepts a caller-supplied port.

- [ ] **Step 3: Implement secure invariants and embedding**

`RuntimeInvariants` and `NewRuntimeInvariants` were fully implemented in Task 1 so every snapshot written from the first commit is safe. Do not redefine them here; the new tests lock their behavior. Rewrite `EmbedResult` and `StartEmbedded` with these contracts:

```go
type EmbedResult struct {
	Cancel  func()
	Done    <-chan struct{}
	Errors  <-chan error
	BaseURL string
	APIKey  string
}

func StartEmbedded(configPath, managementPassword string) (*EmbedResult, error)
```

`ApplyYAML` forces `host: 127.0.0.1`, `remote-management.allow-remote: false`, both panel-disable flags to `true`, and `remote-management.secret-key` to the generated bcrypt hash. It never stores the discarded random plaintext. `StartEmbedded` rejects an empty management password, loads the already-normalized file, rejects an empty or non-bcrypt management sentinel, reasserts host/remote/panel invariants in memory, derives the port from config, and builds with:

```go
service, err := cliproxy.NewBuilder().
	WithConfig(cfg).
	WithConfigPath(configPath).
	WithLocalManagementPassword(managementPassword).
	Build()
```

Return runtime errors on a buffered `Errors` channel; never include the management password in `EmbedResult`, logs, or formatted errors.

- [ ] **Step 4: Run focused tests**

Run: `go test -race ./service/cpa -run 'Test(RuntimeInvariants|StartEmbedded)' -count=1 -v`

Expected: PASS, including the real loopback management call.

- [ ] **Step 5: Commit**

```bash
git add service/cpa/snapshot.go service/cpa/embed.go service/cpa/embed_test.go service/cpa/embed_poc_test.go
git commit -m "feat(cpa): secure embedded management runtime"
```

### Task 3: Lifecycle State Machine, Secret Lifetime, And Request Draining

**Files:**
- Rewrite: `service/cpa/manager.go`
- Rewrite: `service/cpa/manager_test.go`

- [ ] **Step 1: Write failing manager tests with injected start and health functions**

Cover this sequence precisely:

```go
func TestManagerLifecycleAndSecretLifetime(t *testing.T) {
	m, fake := newFakeManager(t)
	if got := m.Status(); got.State != StateStopped || got.Ready { t.Fatalf("initial status = %+v", got) }
	if err := m.Start(context.Background()); err != nil { t.Fatal(err) }
	if got := m.Status(); got.State != StateRunning || !got.Ready || got.LastError != "" { t.Fatalf("started = %+v", got) }

	lease, err := m.AcquireManagement()
	if err != nil { t.Fatal(err) }
	if lease.Password == "" || lease.Target.String() != fake.baseURL { t.Fatalf("bad lease: %+v", lease) }
	lease.Release()

	if err := m.Stop(context.Background()); err != nil { t.Fatal(err) }
	if _, err := m.AcquireManagement(); !errors.Is(err, ErrUnavailable) { t.Fatalf("acquire after stop = %v", err) }
	if m.managementPasswordForTest() != "" { t.Fatal("password retained after stop") }
}

func TestManagerRejectsConcurrentTransition(t *testing.T) {
	m, fake := newFakeManager(t)
	fake.blockStart()
	go func() { _ = m.Start(context.Background()) }()
	fake.waitStartEntered(t)
	if err := m.Stop(context.Background()); !errors.Is(err, ErrTransitionConflict) { t.Fatalf("Stop = %v", err) }
	fake.releaseStart()
}

func TestManagerStopRejectsNewRequestsAndDrainsExistingLease(t *testing.T) {
	m, _ := newRunningFakeManager(t)
	lease, _ := m.AcquireManagement()
	done := make(chan error, 1)
	go func() { done <- m.Stop(context.Background()) }()
	waitForState(t, m, StateStopping)
	if _, err := m.AcquireManagement(); !errors.Is(err, ErrUnavailable) { t.Fatalf("new acquire = %v", err) }
	select { case <-done: t.Fatal("stop did not wait for lease"); case <-time.After(50 * time.Millisecond): }
	lease.Release()
	if err := <-done; err != nil { t.Fatal(err) }
}

func TestManagerStartFailurePublishesErrorWithoutSecret(t *testing.T) {
	m, fake := newFakeManager(t)
	fake.healthErr = errors.New("not ready")
	if err := m.Start(context.Background()); err == nil { t.Fatal("expected start failure") }
	status := m.Status()
	if status.State != StateError || !strings.Contains(status.LastError, "not ready") { t.Fatalf("status = %+v", status) }
	if strings.Contains(status.LastError, m.lastGeneratedPasswordForTest()) { t.Fatal("status leaked password") }
}
```

Also test disabled startup, restart changing the target port, bounded health timeout, bounded drain timeout, unexpected CPA exit, double `Release`, and a failed stop ending in `StateError`.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./service/cpa -run 'TestManager' -count=1`

Expected: FAIL because the current global mutex manager has no states, target leases, readiness, or request draining.

- [ ] **Step 3: Implement manager and runtime contracts**

```go
type ManagementLease struct {
	Target   *url.URL
	Password string
	release  func()
}

func (l *ManagementLease) Release() { if l != nil && l.release != nil { l.release(); l.release = nil } }

type Manager struct {
	mu sync.RWMutex
	transition chan struct{}
	state State
	ready bool
	lastError string
	current *EmbedResult
	target *url.URL
	managementPassword string
	accepting bool
	inflight sync.WaitGroup
	store *SnapshotStore
	hooks LifecycleHooks
	startEmbedded func(string, string) (*EmbedResult, error)
	healthCheck func(context.Context, string) error
	readyTimeout time.Duration
	drainTimeout time.Duration
	stopTimeout time.Duration
}

func NewManager(store *SnapshotStore, hooks LifecycleHooks) *Manager
func (m *Manager) Start(ctx context.Context) error
func (m *Manager) Stop(ctx context.Context) error
func (m *Manager) Shutdown(ctx context.Context) error
func (m *Manager) Restart(ctx context.Context) error
func (m *Manager) StartFromDB(ctx context.Context) error
func (m *Manager) Status() Status
func (m *Manager) AcquireManagement() (*ManagementLease, error)

```

Generate each start password from 32 random bytes encoded with base64url. `Start` loads and materializes the snapshot, creates the auth directory with mode `0700`, starts CPA, polls `/healthz` for at most 30 seconds, then publishes `target`, `password`, `accepting`, and `StateRunning` atomically before calling `OnCPAReady`. `Stop` first sets `StateStopping` and `accepting=false`, calls `OnCPAUnavailable`, drains leases for at most 10 seconds, cancels CPA, waits at most 35 seconds, clears target/password/current, and publishes `StateStopped`. `Restart` owns one transition token while calling unexported `stopLocked` and `startLocked`; public concurrent lifecycle operations return `ErrTransitionConflict` immediately.

`StartFromDB` calls `OnCPAUnavailable` and stays stopped when `CPAEnabled` is false. Public `Start` stores `CPAEnabled=true` before starting; public `Stop` stores `CPAEnabled=false` after stopping. `Shutdown` performs the same drain and shutdown sequence without changing `CPAEnabled`, so a normal Gateway process exit does not disable CPA for the next boot. Restart does not change the desired enabled flag.

`Restart` always stops the current instance, then reads `CPAEnabled`: when true it starts and waits for readiness; when false it remains stopped. Set `Status.Endpoint` to the current loopback base URL only while running and to `"offline"` otherwise. Set `Status.Version` from `const CPAVersion = "v7.2.80"`; do not derive it from the Gateway version.

- [ ] **Step 4: Run package tests under the race detector**

Run: `go test -race ./service/cpa -run 'TestManager' -count=1`

Expected: PASS with no data race and no leaked goroutine reported by the tests' cleanup assertions.

- [ ] **Step 5: Commit**

```bash
git add service/cpa/manager.go service/cpa/manager_test.go
git commit -m "feat(cpa): add managed lifecycle state machine"
```

### Task 4: Runtime Provider Availability And Debounced Synchronization

**Files:**
- Create: `common/provider_runtime.go`
- Create: `common/provider_runtime_test.go`
- Modify: `model/model_route.go`
- Modify: `model/model_route_test.go`
- Rewrite: `service/cpa_provider.go`
- Rewrite: `service/cpa_provider_test.go`
- Modify: `service/cpa_provider_integration_test.go`

- [ ] **Step 1: Write failing availability and coordinator tests**

```go
func TestProviderRuntimeAvailabilityDefaultsToAvailable(t *testing.T) {
	ResetProviderRuntimeAvailabilityForTest()
	if !IsProviderRuntimeAvailable(41) { t.Fatal("unregistered provider must remain available") }
	SetProviderRuntimeAvailable(41, false)
	if IsProviderRuntimeAvailable(41) { t.Fatal("provider should be unavailable") }
	ClearProviderRuntimeAvailability(41)
	if !IsProviderRuntimeAvailable(41) { t.Fatal("cleared provider should use default") }
}

func TestBuildRouteAttemptsSkipsRuntimeUnavailableProvider(t *testing.T) {
	setupModelRouteTestDB(t)
	provider, token := insertSelectableRouteFixture(t, "embedded")
	common.SetProviderRuntimeAvailable(provider.Id, false)
	t.Cleanup(func() { common.ClearProviderRuntimeAvailability(provider.Id) })
	if _, err := BuildRouteAttemptsByPriority("fixture-model", ""); err == nil { t.Fatal("expected no route") }
	common.SetProviderRuntimeAvailable(provider.Id, true)
	assertAttemptUsesToken(t, token.Id)
}

func TestCPACoordinatorPreservesDesiredProviderStatusAndDebounces(t *testing.T) {
	coord, provider, syncCalls := newCoordinatorFixture(t, common.UserStatusDisabled)
	coord.OnCPAReady("http://127.0.0.1:29005", "api-key")
	if provider.Status != common.UserStatusDisabled { t.Fatal("operator status overwritten") }
	if !common.IsProviderRuntimeAvailable(provider.Id) { t.Fatal("running CPA not available") }
	coord.ScheduleCPASync(); coord.ScheduleCPASync(); coord.ScheduleCPASync()
	waitFor(t, func() bool { return syncCalls.Load() == 1 })
	coord.OnCPAUnavailable()
	if common.IsProviderRuntimeAvailable(provider.Id) { t.Fatal("stopped CPA remained selectable") }
}
```

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./common ./model ./service -run 'Test(ProviderRuntime|BuildRouteAttemptsSkipsRuntime|CPACoordinator)' -count=1`

Expected: FAIL because the availability registry and coordinator do not exist.

- [ ] **Step 3: Implement the generic registry and CPA coordinator**

```go
var providerRuntimeAvailability sync.Map

func SetProviderRuntimeAvailable(providerID int, available bool)
func ClearProviderRuntimeAvailability(providerID int)
func IsProviderRuntimeAvailable(providerID int) bool

type CPAProviderCoordinator struct {
	mu sync.Mutex
	providerID int
	timer *time.Timer
	debounce time.Duration
	syncProvider func(*model.Provider) error
}

func NewCPAProviderCoordinator() *CPAProviderCoordinator
func (c *CPAProviderCoordinator) OnCPAReady(baseURL, apiKey string)
func (c *CPAProviderCoordinator) OnCPAUnavailable()
func (c *CPAProviderCoordinator) ScheduleCPASync()
func (c *CPAProviderCoordinator) Close()
```

Add `!common.IsProviderRuntimeAvailable(route.ProviderId)` beside the persisted provider/token status checks in `BuildRouteAttemptsByPriority`. Do not change provider rows or route `enabled` fields when CPA stops. Coordinator upsert updates only `BaseURL`, `ApiKey`, `ProviderType`, and `CheckinEnabled`, preserving `Status`, `Priority`, and `Weight`. `OnCPAReady` marks runtime available before one immediate sync; `ScheduleCPASync` collapses calls within 750 ms; `OnCPAUnavailable` cancels the timer and marks the known or looked-up `__embedded_cpa__` provider unavailable.

- [ ] **Step 4: Run focused and existing provider tests**

Run: `go test -race ./common ./model ./service -run 'Test(ProviderRuntime|BuildRouteAttempts|CPACoordinator|RegisterEmbeddedCPAProvider)' -count=1`

Expected: PASS. Existing providers without registry entries remain selectable.

- [ ] **Step 5: Commit**

```bash
git add common/provider_runtime.go common/provider_runtime_test.go model/model_route.go model/model_route_test.go service/cpa_provider.go service/cpa_provider_test.go service/cpa_provider_integration_test.go
git commit -m "feat(cpa): gate provider routes on runtime availability"
```

### Task 5: Transparent Management Reverse Proxy

**Files:**
- Create: `service/cpa/management_proxy.go`
- Create: `service/cpa/management_proxy_test.go`

- [ ] **Step 1: Write failing proxy contract tests**

Use an `httptest.Server` upstream and a fake lease provider. One table must exercise `GET`, `POST`, `PUT`, `PATCH`, and `DELETE`; JSON, YAML, and multipart bodies; query strings; a streamed download; and CPA headers. Core assertions:

```go
func TestManagementProxySanitizesAndForwards(t *testing.T) {
	upstream := captureUpstream(t, func(r *http.Request, body []byte) {
		if r.URL.Path != "/v0/management/auth-files" || r.URL.RawQuery != "name=a.json" { t.Fatalf("URL = %s", r.URL) }
		if got := r.Header.Get("Authorization"); got != "Bearer runtime-secret" { t.Fatalf("auth = %q", got) }
		for _, name := range []string{"X-Management-Key", "Cookie", "Connection", "Proxy-Authorization"} {
			if r.Header.Get(name) != "" { t.Fatalf("forwarded %s", name) }
		}
	})
	proxy := newProxyFixture(t, upstream.URL)
	req := httptest.NewRequest(http.MethodPost, "/v0/management/auth-files?name=a.json", strings.NewReader("payload"))
	req.Header.Set("Authorization", "Bearer browser-placeholder")
	req.Header.Set("X-Management-Key", "browser-placeholder")
	req.Header.Set("Cookie", "session=sensitive")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || rec.Body.String() != "upstream-body" { t.Fatalf("response = %d %q", rec.Code, rec.Body.String()) }
}

func TestManagementProxyPreservesBusinessErrorsAndDownloads(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="auth.json"`)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-CPA-VERSION", "v7.2.80")
		w.Header().Set("X-CPA-COMMIT", "fixture")
		w.Header().Set("X-CPA-BUILD-DATE", "2026-07-16")
		w.Header().Set("X-CPA-SUPPORT-PLUGIN", "true")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"error":"business validation"}`)
	}))
	defer upstream.Close()
	rec := serveProxy(t, newProxyFixture(t, upstream.URL), http.MethodGet, "/v0/management/auth-files/download?name=auth.json", nil)
	if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != `{"error":"business validation"}` { t.Fatalf("response = %d %s", rec.Code, rec.Body.String()) }
	for name, want := range map[string]string{"Content-Disposition": `attachment; filename="auth.json"`, "Content-Type": "application/json", "X-CPA-VERSION": "v7.2.80", "X-CPA-COMMIT": "fixture", "X-CPA-BUILD-DATE": "2026-07-16", "X-CPA-SUPPORT-PLUGIN": "true"} {
		if got := rec.Header().Get(name); got != want { t.Fatalf("%s = %q, want %q", name, got, want) }
	}
}

func TestManagementProxyPersistsBeforeReturningMutation(t *testing.T) {
	proxy, persistenceEntered, releasePersistence, syncCalls := newBlockingPersistenceProxy(t, http.StatusOK)
	rec := newThreadSafeRecorder()
	done := make(chan struct{})
	go func() { proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/v0/management/debug", strings.NewReader(`{"value":true}`))); close(done) }()
	<-persistenceEntered
	if rec.Committed() { t.Fatal("response committed before persistence") }
	close(releasePersistence)
	<-done
	if rec.Code() != http.StatusOK || syncCalls.Load() != 1 { t.Fatalf("response=%d sync=%d", rec.Code(), syncCalls.Load()) }
}

func TestManagementProxyMapsPersistenceFailure(t *testing.T) {
	proxy := newFailingPersistenceProxy(t, errors.New("database closed"))
	rec := serveProxy(t, proxy, http.MethodDelete, "/v0/management/api-keys?index=0", nil)
	want := `{"success":false,"code":"persistence_failed","message":"CPA applied the change but durable snapshot persistence failed"}`
	if rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != want { t.Fatalf("response = %d %s", rec.Code, rec.Body.String()) }
}

func TestManagementProxyMapsOfflineTransportAndTimeout(t *testing.T) {
	tests := []struct{ name string; proxy *ManagementProxy; wantStatus int; wantCode string }{
		{"offline", newUnavailableProxy(t), http.StatusServiceUnavailable, "cpa_unavailable"},
		{"connection refused", newRefusedConnectionProxy(t), http.StatusBadGateway, "upstream_failure"},
		{"response header timeout", newTimeoutProxy(t), http.StatusGatewayTimeout, "upstream_timeout"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveProxy(t, tc.proxy, http.MethodGet, "/v0/management/config", nil)
			var payload struct { Code string `json:"code"` }
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil { t.Fatal(err) }
			if rec.Code != tc.wantStatus || payload.Code != tc.wantCode { t.Fatalf("response = %d %s", rec.Code, rec.Body.String()) }
		})
	}
}
```

Add an audit test that captures the log string and asserts it contains Root username, method, normalized path, status, and duration but excludes body, raw query values, authorization, cookie, OAuth code, and uploaded credential content. Add `get-auth-status` response tests: schedule sync only when CPA returns a successful completed status.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./service/cpa -run 'TestManagementProxy' -count=1`

Expected: FAIL because `ManagementProxy` does not exist.

- [ ] **Step 3: Implement the reverse proxy**

```go
type managementLeaseProvider interface {
	AcquireManagement() (*ManagementLease, error)
}

type ManagementProxy struct {
	manager managementLeaseProvider
	store *SnapshotStore
	transport http.RoundTripper
	scheduleSync func()
	auditf func(string, ...any)
}

func NewManagementProxy(manager managementLeaseProvider, store *SnapshotStore, scheduleSync func()) *ManagementProxy
func WithManagementAuditUser(r *http.Request, username string) *http.Request
func (p *ManagementProxy) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

Acquire one lease per request and release it only after streaming finishes. Configure `httputil.ReverseProxy` per request, preserve the original path and query, and explicitly delete `Authorization`, `X-Management-Key`, `Cookie`, `Proxy-Authorization`, `Connection`, `Proxy-Connection`, `Keep-Alive`, `TE`, `Trailer`, `Transfer-Encoding`, and `Upgrade` before setting `Authorization: Bearer <runtime password>`. Use a cloned default transport with 10-second dial/TLS timeouts and 30-second response-header timeout, but no whole-response timeout.

`WithManagementAuditUser` stores the username in a private typed request-context key. `controller.ProxyCPAManagement` calls it with `c.GetString("username")` before invoking the proxy; the context value is never converted into an upstream header.

For successful `POST`, `PUT`, `PATCH`, or `DELETE`, call `PersistRuntime` from `ModifyResponse` before the response is committed. Return a typed `persistenceError`; `ErrorHandler` maps it to the stable 500 JSON. Schedule sync only after successful persistence. For `GET /v0/management/get-auth-status`, buffer at most 1 MiB, restore the body, decode the CPA status, and schedule only when it reports completion. All other upstream statuses and bodies pass through unchanged.

- [ ] **Step 4: Run proxy tests under race detection**

Run: `go test -race ./service/cpa -run 'TestManagementProxy' -count=1`

Expected: PASS, including streamed responses and audit redaction.

- [ ] **Step 5: Commit**

```bash
git add service/cpa/management_proxy.go service/cpa/management_proxy_test.go
git commit -m "feat(cpa): proxy complete management API"
```

### Task 6: Same-Origin Protection And Exact OAuth Callback Relays

**Files:**
- Create: `middleware/same-origin.go`
- Create: `middleware/same-origin_test.go`
- Create: `service/cpa/oauth_relay.go`
- Create: `service/cpa/oauth_relay_test.go`

- [ ] **Step 1: Write failing same-origin tests**

```go
func TestSameOriginForMutations(t *testing.T) {
	tests := []struct{ name, method, origin, referer string; want int }{
		{"read without origin", http.MethodGet, "", "", http.StatusNoContent},
		{"matching origin", http.MethodPost, "https://gateway.example", "", http.StatusNoContent},
		{"matching referer", http.MethodDelete, "", "https://gateway.example/cpa", http.StatusNoContent},
		{"missing both", http.MethodPatch, "", "", http.StatusForbidden},
		{"foreign origin", http.MethodPut, "https://evil.example", "", http.StatusForbidden},
		{"foreign referer", http.MethodPost, "", "https://evil.example/cpa", http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://gateway.example/v0/management/config", nil)
			req.Host = "gateway.example"
			req.Header.Set("X-Forwarded-Proto", "https")
			if tc.origin != "" { req.Header.Set("Origin", tc.origin) }
			if tc.referer != "" { req.Header.Set("Referer", tc.referer) }
			rec := serveSameOrigin(t, req)
			if rec.Code != tc.want { t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String()) }
			if tc.want == http.StatusForbidden {
				var payload struct { Code string `json:"code"` }
				if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil { t.Fatal(err) }
				if payload.Code != "origin_rejected" { t.Fatalf("body = %s", rec.Body.String()) }
			}
		})
	}
}
```

Test malformed origins, default ports, mixed-case hosts, direct TLS requests, and `OPTIONS` as a safe method.

- [ ] **Step 2: Write failing OAuth relay tests**

```go
func TestOAuthRelayAllowsOnlyPendingMatchingStateOnce(t *testing.T) {
	api.RegisterOAuthSession("state-1", "anthropic")
	relay := newOAuthRelayFixture(t)
	first := relayRequest(t, relay, "/anthropic/callback?state=state-1&code=secret-code")
	if first.Code != http.StatusOK { t.Fatalf("first = %d %s", first.Code, first.Body.String()) }
	second := relayRequest(t, relay, "/anthropic/callback?state=state-1&code=secret-code")
	if second.Code != http.StatusBadRequest { t.Fatalf("replay = %d", second.Code) }
}

func TestOAuthRelayRejectsMismatchInvalidExpiredAndStopped(t *testing.T) {
	api.RegisterOAuthSession("codex-state", "codex")
	relay, audit := newOAuthRelayFixtureWithAudit(t)
	for _, target := range []string{
		"/anthropic/callback?state=codex-state&code=secret-code",
		"/codex/callback?state=..%2Fstate&code=secret-code",
		"/codex/callback?state=unknown&code=secret-code",
	} {
		if rec := relayRequest(t, relay, target); rec.Code != http.StatusBadRequest { t.Fatalf("%s = %d", target, rec.Code) }
	}
	if rec := relayRequest(t, newStoppedOAuthRelay(t), "/codex/callback?state=codex-state&code=secret-code"); rec.Code != http.StatusServiceUnavailable { t.Fatalf("stopped = %d", rec.Code) }
	if strings.Contains(audit.String(), "secret-code") || strings.Contains(audit.String(), "codex-state") { t.Fatalf("audit leaked callback values: %s", audit.String()) }
}
```

- [ ] **Step 3: Run tests and verify failure**

Run: `go test ./middleware ./service/cpa -run 'Test(SameOrigin|OAuthRelay)' -count=1`

Expected: FAIL because the middleware and relay do not exist.

- [ ] **Step 4: Implement origin validation and relay**

```go
func SameOrigin() gin.HandlerFunc

var oauthCallbackProviders = map[string]string{
	"/anthropic/callback":   "anthropic",
	"/codex/callback":       "codex",
	"/antigravity/callback": "antigravity",
}

type OAuthRelay struct {
	manager managementLeaseProvider
	mu sync.Mutex
	claimed map[string]time.Time
	transport http.RoundTripper
}

func NewOAuthRelay(manager managementLeaseProvider) *OAuthRelay
func (r *OAuthRelay) ServeHTTP(w http.ResponseWriter, req *http.Request)
```

For mutation requests, `SameOrigin` parses `Origin`, falling back to `Referer`, and compares normalized scheme, hostname, and effective port with the Gateway request (`TLS` first, then a single trusted `X-Forwarded-Proto` value, then `http`). OAuth relay maps only the three literal paths, validates `state` through `api.ValidateOAuthState`, verifies `api.GetOAuthSession` returns the expected provider with empty error status, and atomically claims the state before forwarding the exact callback path/query to CPA without cookies or any management header. Retain claimed states for 31 minutes and purge opportunistically. Do not log query strings or response bodies. Release the manager lease after the upstream body is copied.

- [ ] **Step 5: Run focused tests**

Run: `go test -race ./middleware ./service/cpa -run 'Test(SameOrigin|OAuthRelay)' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add middleware/same-origin.go middleware/same-origin_test.go service/cpa/oauth_relay.go service/cpa/oauth_relay_test.go
git commit -m "feat(cpa): protect mutations and relay OAuth callbacks"
```

### Task 7: Root API, Route Wiring, Compatibility, And Startup

**Files:**
- Create: `service/cpa/runtime.go`
- Create: `service/cpa/runtime_test.go`
- Rewrite: `controller/cpa.go`
- Create: `controller/cpa_test.go`
- Modify: `router/api-router.go`
- Create: `router/cpa_router_test.go`
- Modify: `main.go`

- [ ] **Step 1: Write failing controller and route authorization tests**

Build a Gin engine with cookie sessions and the real middleware chain. The route fixture replaces `Runtime.Panel` with a deterministic 200 `text/html` handler; the production hash-verified handler arrives in Task 8. Assert this route matrix:

| Route | Root session | Admin session | User token | No session |
|---|---:|---:|---:|---:|
| `GET /api/cpa/status` | 200 | denied | denied | 401 |
| `POST /api/cpa/start` | 200/409 | denied | denied | 401 |
| `POST /api/cpa/stop` | 200/409 | denied | denied | 401 |
| `POST /api/cpa/restart` | 200/409 | denied | denied | 401 |
| `GET /api/cpa/panel` | 200 | denied | denied | 401 |
| `GET /v0/management/config` | upstream status | denied | denied | 401 |
| `GET /v0/management/config` with Root session plus `Authorization: Bearer gateway-managed` | upstream status | n/a | n/a | n/a |
| `PUT /v0/management/config` foreign Origin | 403 | 403 | 403 | 401 |
| `GET /anthropic/callback` valid state | CPA status | CPA status | CPA status | CPA status |

Controller assertions:

```go
func TestCPAStatusNeverExposesManagementPassword(t *testing.T) {
	rec := requestCPAController(t, http.MethodGet, "/api/cpa/status", nil)
	if strings.Contains(rec.Body.String(), "runtime-secret") { t.Fatal("secret leaked") }
}

func TestLegacyConfigPatchPreservesFullYAMLAndReloadAlias(t *testing.T) {
	runtime, calls := installControllerRuntimeFixture(t, "plugins:\n  instances:\n    demo:\n      custom: keep\n")
	body := `{"enabled":true,"api_keys":["new-key"],"auth_dir":"new-auth","port":29010}`
	rec := requestCPAController(t, http.MethodPut, "/api/cpa/config", strings.NewReader(body))
	if rec.Code != http.StatusOK { t.Fatalf("PUT = %d %s", rec.Code, rec.Body.String()) }
	got, err := os.ReadFile(runtime.Store.Path())
	if err != nil { t.Fatal(err) }
	if !bytes.Contains(got, []byte("custom: keep")) { t.Fatalf("plugin config lost:\n%s", got) }
	if calls.restart.Load() != 1 { t.Fatalf("restart calls = %d", calls.restart.Load()) }
	rec = requestCPAController(t, http.MethodPost, "/api/cpa/reload", nil)
	if rec.Code != http.StatusOK || calls.restart.Load() != 2 { t.Fatalf("reload alias = %d calls=%d", rec.Code, calls.restart.Load()) }
}

func TestLifecycleErrorMapping(t *testing.T) {
	tests := []struct{ err error; status int; code string }{
		{ErrTransitionConflict, http.StatusConflict, "transition_conflict"},
		{ErrUnavailable, http.StatusServiceUnavailable, "cpa_unavailable"},
		{errors.New("start failed"), http.StatusInternalServerError, "lifecycle_failed"},
	}
	for _, tc := range tests {
		rec := invokeLifecycleError(t, tc.err)
		var payload struct { Code string `json:"code"` }
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil { t.Fatal(err) }
		if rec.Code != tc.status || payload.Code != tc.code {
			t.Fatalf("err %v => %d %s", tc.err, rec.Code, rec.Body.String())
		}
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./service/cpa ./controller ./router -run 'Test(Runtime|CPA|LegacyConfig|Lifecycle)' -count=1`

Expected: FAIL because status/start/stop/restart/panel/proxy routes do not exist.

- [ ] **Step 3: Construct and register one runtime**

```go
type Runtime struct {
	Store   *SnapshotStore
	Manager *Manager
	Proxy   *ManagementProxy
	OAuth   *OAuthRelay
	Panel   http.Handler
}

func NewRuntime(runtimeDir string, hooks LifecycleHooks) (*Runtime, error) {
	invariants, err := NewRuntimeInvariants(rand.Reader)
	if err != nil { return nil, err }
	store := NewSnapshotStore(runtimeDir, invariants)
	manager := NewManager(store, hooks)
	return &Runtime{
		Store: store,
		Manager: manager,
		Proxy: NewManagementProxy(manager, store, hooks.ScheduleCPASync),
		OAuth: NewOAuthRelay(manager),
		Panel: http.NotFoundHandler(),
	}, nil
}

var defaultRuntime atomic.Pointer[Runtime]

func SetDefaultRuntime(runtime *Runtime) { defaultRuntime.Store(runtime) }
func DefaultRuntime() *Runtime { return defaultRuntime.Load() }
```

`NewManager` is the Task 3 constructor that installs production `StartEmbedded`, health, and timeout dependencies. `runtime_test.go` asserts all fields are nonnil, two calls to `SetDefaultRuntime` atomically replace the pointer, and concurrent reads under `-race` are safe. `Panel` is deliberately `http.NotFoundHandler()` for this compilable intermediate commit; Task 8 replaces it with the verified embedded asset.

- [ ] **Step 4: Implement controllers and exact routes**

Expose these handlers in `controller/cpa.go`:

```go
func GetCPAStatus(c *gin.Context)
func StartCPA(c *gin.Context)
func StopCPA(c *gin.Context)
func RestartCPA(c *gin.Context)
func GetCPAConfig(c *gin.Context)
func UpdateCPAConfig(c *gin.Context)
func ReloadCPA(c *gin.Context)
func ServeCPAPanel(c *gin.Context)
func ProxyCPAManagement(c *gin.Context)
func RelayCPAOAuthCallback(c *gin.Context)
```

`GetCPAConfig` returns `Runtime.Store.Basic()`. `UpdateCPAConfig` binds `CPAConfig`, requires port 1-65535, at least one nonblank API key, and a nonblank auth directory, then calls `PatchBasic` followed by `Manager.Restart`; because Restart honors the stored enabled flag, disabling through the legacy body stops and does not relaunch CPA. `ReloadCPA` calls the same `Manager.Restart` without rewriting the snapshot. Lifecycle success responses contain `Runtime.Manager.Status()` and never contain a management key.

Routes must be literal:

```go
cpaRoute := apiRouter.Group("/cpa")
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

management := router.Group("/v0/management")
management.Use(middleware.RootAuth(), middleware.NoTokenAuth(), middleware.SameOrigin())
management.Any("", controller.ProxyCPAManagement)
management.Any("/*path", controller.ProxyCPAManagement)

router.GET("/anthropic/callback", controller.RelayCPAOAuthCallback)
router.GET("/codex/callback", controller.RelayCPAOAuthCallback)
router.GET("/antigravity/callback", controller.RelayCPAOAuthCallback)
```

Do not add a generic CPA root proxy or `/v0/resource` proxy. The pinned panel v1.18.3 was checked and has no `/v0/resource/plugins` reference.

Each lifecycle controller records one audit line after completion with `user=<c.GetString("username")>`, `action=start|stop|restart|config_update`, result status, and duration. The line contains no request body, API key, auth directory contents, runtime password, or error text that has not first passed through a secret-redaction helper. Add controller assertions that the audit capture contains the username/action/status and excludes the submitted API key.

- [ ] **Step 5: Wire process startup and shutdown**

Replace global `StartFromDB` calls in `main.go` with:

```go
coordinator := service.NewCPAProviderCoordinator()
defer coordinator.Close()

cpaRuntime, err := cpa.NewRuntime(os.Getenv("CPA_RUNTIME_DIR"), coordinator)
if err != nil { common.FatalLog("initialize embedded CPA: " + err.Error()) }
cpa.SetDefaultRuntime(cpaRuntime)
if err := cpaRuntime.Manager.StartFromDB(context.Background()); err != nil {
	common.SysLog("embedded CPA startup failed: " + err.Error())
}
defer func() { _ = cpaRuntime.Manager.Shutdown(context.Background()) }()
```

Call this after `model.InitOptionMap()` and before router registration. `Runtime.Panel`, `Runtime.Proxy`, and `Runtime.OAuth` are nonnil when `SetDefaultRuntime` is called; the panel responds 404 only until Task 8 installs the hash-verified asset.

- [ ] **Step 6: Run backend route and compatibility tests**

Run: `go test -race ./controller ./router ./service/cpa -run 'Test(Runtime|CPA|LegacyConfig|Lifecycle)' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add service/cpa/runtime.go service/cpa/runtime_test.go controller/cpa.go controller/cpa_test.go router/api-router.go router/cpa_router_test.go main.go
git commit -m "feat(cpa): expose Root lifecycle and management routes"
```

### Task 8: Pin, Verify, Embed, And Serve Official Panel v1.18.3

**Files:**
- Create: `service/cpa/assets/management.html`
- Create: `service/cpa/panel.go`
- Create: `service/cpa/panel_test.go`
- Modify: `service/cpa/runtime.go`

- [ ] **Step 1: Write the failing panel integrity and response tests**

```go
func TestEmbeddedManagementPanelIntegrity(t *testing.T) {
	sum := sha256.Sum256(managementHTML)
	if got := hex.EncodeToString(sum[:]); got != ManagementPanelSHA256 {
		t.Fatalf("panel SHA-256 = %s, want %s", got, ManagementPanelSHA256)
	}
}

func TestPanelHandlerHeadersAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	NewPanelHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/cpa/panel", nil))
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), managementHTML) { t.Fatal("panel response mismatch") }
	if rec.Header().Get("Content-Type") != "text/html; charset=utf-8" { t.Fatal("wrong content type") }
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'self'") { t.Fatal("missing frame policy") }
	if rec.Header().Get("Cache-Control") != "no-store" { t.Fatal("panel must not be stale") }
}
```

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./service/cpa -run 'Test(EmbeddedManagementPanel|PanelHandler)' -count=1`

Expected: FAIL because the embedded asset and handler do not exist.

- [ ] **Step 3: Download the reviewed release asset and verify its hash**

Run from repository root:

```powershell
New-Item -ItemType Directory -Force service/cpa/assets
Invoke-WebRequest -UseBasicParsing 'https://github.com/router-for-me/Cli-Proxy-API-Management-Center/releases/download/v1.18.3/management.html' -OutFile 'service/cpa/assets/management.html'
(Get-FileHash 'service/cpa/assets/management.html' -Algorithm SHA256).Hash.ToLower()
```

Expected hash: `941a49a619a719a59e4c7917c6888a53eb3f41a4fa2fbb5c1cc94f2d1fc9cd4b`. Stop immediately if it differs; do not commit or serve a mismatched asset.

- [ ] **Step 4: Implement the embedded handler**

```go
import _ "embed"

const (
	ManagementPanelVersion = "v1.18.3"
	ManagementPanelSHA256  = "941a49a619a719a59e4c7917c6888a53eb3f41a4fa2fbb5c1cc94f2d1fc9cd4b"
)

//go:embed assets/management.html
var managementHTML []byte

func NewPanelHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self' data: blob:; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data: blob:; font-src 'self' data:; frame-ancestors 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(managementHTML)
	})
}
```

Change `NewRuntime` from `Panel: http.NotFoundHandler()` to `Panel: NewPanelHandler()`. The CPA runtime config already disables CPA's own control-panel serving and update loop; Gateway never downloads this asset at runtime.

- [ ] **Step 5: Run integrity tests**

Run: `go test ./service/cpa -run 'Test(EmbeddedManagementPanel|PanelHandler)' -count=1`

Expected: PASS and exact hash match.

- [ ] **Step 6: Commit**

```bash
git add service/cpa/assets/management.html service/cpa/panel.go service/cpa/panel_test.go service/cpa/runtime.go
git commit -m "feat(cpa): embed pinned official management panel"
```

### Task 9: Root-Only CPA Page, Navigation, Session Bootstrap, And Responsive Layout

**Files:**
- Create: `web/src/components/RootRoute.js`
- Create: `web/src/components/RootRoute.test.js`
- Create: `web/src/pages/CPA/index.js`
- Create: `web/src/pages/CPA/index.test.js`
- Modify: `web/src/components/Layout.js`
- Modify: `web/src/components/Layout.test.js`
- Modify: `web/src/App.js`
- Modify: `web/src/index.css`

- [ ] **Step 1: Write the failing Root route and navigation tests**

`RootRoute.test.js` must render with roles 1, 10, and 100 and assert only role 100 sees children. Extend `Layout.test.js`:

```js
it('shows CPA navigation only to Root users', async () => {
  await renderLayout(root, 10);
  expect(container.querySelector('a[href="/cpa"]')).toBeNull();
  await renderLayout(root, 100);
  expect(container.querySelector('a[href="/cpa"]')).not.toBeNull();
});
```

- [ ] **Step 2: Write failing CPA page behavior tests**

Mock `API.get` and `API.post`. Cover running, stopped, starting, stopping, error, action failure, and polling cleanup. Required assertions:

```js
it('bootstraps a harmless panel session and mounts iframe only when running', async () => {
  localStorage.setItem('cli-proxy-auth', '{"managementKey":"old-secret"}');
  API.get.mockResolvedValue({ data: { success: true, data: { enabled: true, state: 'running', ready: true, version: 'v7.2.80', endpoint: 'healthy' } } });
  await renderCPA();
  expect(localStorage.getItem('cli-proxy-auth')).toBeNull();
  expect(localStorage.getItem('apiBase')).toBe(window.location.origin);
  expect(localStorage.getItem('managementKey')).toBe('gateway-managed');
  expect(localStorage.getItem('isLoggedIn')).toBe('true');
  expect(container.querySelector('iframe').getAttribute('src')).toBe('/api/cpa/panel');
  expect(document.body.textContent).not.toContain('runtime-secret');
});

it('starts an offline CPA without mounting the panel first', async () => {
  API.get.mockResolvedValue({ data: { success: true, data: { enabled: false, state: 'stopped', ready: false, version: 'v7.2.80', endpoint: 'offline' } } });
  API.post.mockResolvedValue({ data: { success: true } });
  await renderCPA();
  expect(container.querySelector('iframe')).toBeNull();
  clickButton('Start');
  expect(API.post).toHaveBeenCalledWith('/api/cpa/start');
});
```

Use DOM property checks instead of `@testing-library`, which is not a declared dependency.

- [ ] **Step 3: Run frontend tests and verify failure**

Run from `web`: `$env:CI='true'; npm test -- --runInBand RootRoute.test.js Layout.test.js CPA/index.test.js`

Expected: FAIL because the route and page do not exist.

- [ ] **Step 4: Implement Root guard, page, route, and navigation**

Implement `RootRoute.js` exactly with context-first and local-storage fallback behavior:

```jsx
import React, { useContext } from 'react';
import { Navigate } from 'react-router-dom';
import { UserContext } from '../context/User';

const storedUser = () => {
  try { return JSON.parse(localStorage.getItem('user')); } catch (error) { return null; }
};

function RootRoute({ children }) {
  const [userState] = useContext(UserContext);
  const role = Number((userState.user || storedUser())?.role);
  return Number.isFinite(role) && role >= 100 ? children : <Navigate to='/' replace />;
}

export { RootRoute };
```

Add `const CPA = lazy(() => import('./pages/CPA'));` and:

```jsx
<Route path='/cpa' element={<RootRoute><CPA /></RootRoute>} />
```

Add a `Cpu` icon navigation item with `{ name: 'CPA Management', path: '/cpa', root: true }`, compute `isRoot` from context role, and filter root items independently of admin items.

Use this page state and bootstrap contract:

```js
export const bootstrapPanelSession = () => {
  localStorage.removeItem('cli-proxy-auth');
  localStorage.setItem('apiBase', window.location.origin);
  localStorage.setItem('managementKey', 'gateway-managed');
  localStorage.setItem('isLoggedIn', 'true');
};

const actionPaths = { start: '/api/cpa/start', stop: '/api/cpa/stop', restart: '/api/cpa/restart' };
```

Fetch `/api/cpa/status` on mount and every 2 seconds. Disable all action buttons during starting/stopping or an in-flight action. Use icon buttons with `Play`, `Square`, and `RefreshCw`, visible text labels beside them in the lifecycle bar, and `title` attributes. Render the iframe only for `state === 'running' && ready`; call `bootstrapPanelSession()` immediately before mounting it. Show `last_error` in an alert band without rendering any secret-like data.

- [ ] **Step 5: Add stable responsive CSS**

Add unframed page rules with fixed control dimensions and no nested cards:

```css
.cpa-page { display: flex; flex-direction: column; min-height: calc(100vh - 4.5rem); width: 100%; }
.cpa-lifecycle-bar { display: flex; align-items: center; gap: .75rem; min-height: 3.5rem; padding: .65rem 1rem; border-bottom: 1px solid var(--border-color); flex-wrap: wrap; }
.cpa-status-group { display: flex; align-items: center; gap: .5rem; min-width: 12rem; }
.cpa-actions { display: flex; gap: .5rem; margin-left: auto; }
.cpa-action { min-width: 6.75rem; height: 2.25rem; display: inline-flex; align-items: center; justify-content: center; gap: .4rem; }
.cpa-panel-frame { width: 100%; flex: 1 1 auto; min-height: 42rem; border: 0; background: #fff; }
.cpa-offline { flex: 1; display: grid; place-items: center; min-height: 24rem; padding: 1.5rem; text-align: center; }
@media (max-width: 640px) {
  .cpa-page { min-height: calc(100vh - 4rem); }
  .cpa-actions { width: 100%; margin-left: 0; }
  .cpa-action { flex: 1 1 0; min-width: 0; }
  .cpa-panel-frame { min-height: 36rem; }
}
```

Reuse existing CSS variables; inspect the final palette and do not introduce a single-hue page theme.

- [ ] **Step 6: Run frontend tests and build**

Run from `web`:

```powershell
$env:CI='true'; npm test -- --runInBand RootRoute.test.js Layout.test.js CPA/index.test.js
npm run build
```

Expected: all selected tests PASS; production build completes without ESLint or overflow-related warnings.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/RootRoute.js web/src/components/RootRoute.test.js web/src/pages/CPA/index.js web/src/pages/CPA/index.test.js web/src/components/Layout.js web/src/components/Layout.test.js web/src/App.js web/src/index.css web/build
git commit -m "feat(cpa): add Root management workspace"
```

### Task 10: Real CPA Integration, Browser Verification, And Full Regression

**Files:**
- Create: `service/cpa/full_management_integration_test.go`
- Modify: `web/package.json`
- Modify: `web/package-lock.json`
- Create: `web/playwright.config.js`
- Create: `web/e2e/start-gateway.ps1`
- Create: `web/e2e/cpa-management.spec.js`

- [ ] **Step 1: Write a real-CPA full management integration test**

Start a real CPA on a free loopback port with a temporary auth directory and exercise requests through `ManagementProxy`, never directly except health setup. The test must perform in order:

```go
func TestFullManagementRoundTripAgainstRealCPA(t *testing.T) {
	runtime := startRealTestRuntime(t)
	client := newGatewayProxyClient(t, runtime)

	assertJSONStatus(t, client.Get("/v0/management/config"), http.StatusOK)
	assertJSONStatus(t, client.PatchJSON("/v0/management/debug", `{"value":true}`), http.StatusOK)
	uploadAuthFile(t, client, "fixture.json", `{"type":"codex","access_token":"fixture-token","disabled":true}`)
	assertAuthFileListed(t, client, "fixture.json")
	assertDownloadedFile(t, client, "fixture.json")
	assertJSONStatus(t, client.PatchJSON("/v0/management/auth-files/status", `{"name":"fixture.json","disabled":false}`), http.StatusOK)
	assertJSONStatus(t, client.PatchJSON("/v0/management/auth-files/fields", `{"name":"fixture.json","updates":{"label":"integration"}}`), http.StatusOK)
	assertJSONStatus(t, client.Delete("/v0/management/auth-files?name=fixture.json"), http.StatusOK)

	before := runtime.Manager.Status()
	basic, err := runtime.Store.Basic()
	if err != nil { t.Fatal(err) }
	basic.Port = freePort(t)
	if err := runtime.Store.PatchBasic(*basic); err != nil { t.Fatal(err) }
	if err := runtime.Manager.Restart(context.Background()); err != nil { t.Fatal(err) }
	after := runtime.Manager.Status()
	if before.Endpoint == after.Endpoint { t.Fatal("restart did not publish the new proxy target") }
	assertConfigValueAfterRestart(t, runtime, "debug", true)

	if err := runtime.Manager.Stop(context.Background()); err != nil { t.Fatal(err) }
	assertGatewayError(t, client.Get("/v0/management/config"), http.StatusServiceUnavailable, "cpa_unavailable")
}
```

For quota reset, the integration fixture injects a custom `Manager.startEmbedded` closure that uses the normal CPA builder plus `WithCoreAuthManager(coreManager)`, where `coreManager := coreauth.NewManager(nil, nil, nil)`. Register a public `coreauth.Auth` with `StatusError`, `Unavailable=true`, future `NextRetryAfter`, exceeded `QuotaState`, and one exceeded `ModelState`; call `authIndex := auth.EnsureIndex()` before `coreManager.Register(context.Background(), auth)`. Send `POST /v0/management/reset-quota` with `{"auth_index":"<authIndex>"}` through Gateway, then use `coreManager.GetByID(auth.ID)` to assert auth and model status are active, retry timestamps are zero, and quota fields are cleared. This follows CPA v7.2.80's own `internal/api/handlers/management/quota_test.go` using only exported SDK types and methods, and requires no external account. Add separate multipart plugin upload forwarding only when the pinned CPA build reports `X-CPA-SUPPORT-PLUGIN`; otherwise assert the header and management plugin status route pass through without treating unsupported native loading as a test failure.

- [ ] **Step 2: Run real integration tests**

Run: `go test ./service/cpa -run 'TestFullManagementRoundTripAgainstRealCPA' -count=1 -v -timeout 3m`

Expected: PASS. Logs contain no fixture token, management password, OAuth code, or uploaded file body.

- [ ] **Step 3: Add reproducible Playwright setup**

From `web`, run `npm install --save-dev @playwright/test`, then add:

```js
// playwright.config.js
const { defineConfig, devices } = require('@playwright/test');
module.exports = defineConfig({
  testDir: './e2e',
  timeout: 90_000,
  use: { baseURL: 'http://127.0.0.1:3031', trace: 'retain-on-failure' },
  webServer: {
    command: 'powershell -NoProfile -ExecutionPolicy Bypass -File e2e/start-gateway.ps1',
    url: 'http://127.0.0.1:3031/api/status',
    timeout: 180_000,
    reuseExistingServer: false,
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile', use: { ...devices['Pixel 7'] } },
  ],
});
```

`start-gateway.ps1` must resolve the repository root, create a unique directory below `$env:TEMP`, set `PORT=3031` and `CPA_RUNTIME_DIR=<temp>/cpa`, then set the process working directory to the temp directory so the default `gateway-aggregator.db` is isolated there. Build `web/build`, build the Gateway executable into the temp directory, change to that directory, run it in the foreground, and remove the directory in `finally`.

```powershell
$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$runDir = Join-Path $env:TEMP ("newapi-cpa-e2e-" + [guid]::NewGuid().ToString('N'))
$exe = Join-Path $runDir 'gateway.exe'
New-Item -ItemType Directory -Force $runDir | Out-Null
try {
    Push-Location (Join-Path $repo 'web')
    try { npm run build } finally { Pop-Location }
    Push-Location $repo
    try { go build -o $exe . } finally { Pop-Location }
    $env:PORT = '3031'
    $env:GIN_MODE = 'release'
    $env:CPA_RUNTIME_DIR = Join-Path $runDir 'cpa'
    Set-Location $runDir
    & $exe
} finally {
    Set-Location $env:TEMP
    $tempRoot = [IO.Path]::GetFullPath($env:TEMP).TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    $resolvedRunDir = [IO.Path]::GetFullPath($runDir)
    if ($resolvedRunDir.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -and (Split-Path $resolvedRunDir -Leaf) -like 'newapi-cpa-e2e-*' -and (Test-Path $resolvedRunDir)) {
        Remove-Item -LiteralPath $resolvedRunDir -Recurse -Force
    }
}
```

- [ ] **Step 4: Write the browser security and layout test**

```js
const { test, expect } = require('@playwright/test');

test('Root reaches embedded CPA panel without a second secret', async ({ page }, testInfo) => {
  await page.context().request.post('/api/user/login', { data: { username: 'root', password: '123456' } });
  await page.goto('/cpa');
  await page.getByRole('button', { name: /Start/i }).click();
  await expect(page.locator('.cpa-status')).toContainText(/running/i, { timeout: 45_000 });

  const managementResponses = [];
  page.on('response', response => {
    if (response.url().includes('/v0/management')) managementResponses.push(response.status());
  });
  const frame = page.frameLocator('iframe[title="CPA Management Center"]');
  await expect(frame.locator('body')).not.toHaveText('', { timeout: 30_000 });
  await expect.poll(() => managementResponses.some(status => status === 200)).toBeTruthy();

  const storage = await page.evaluate(() => Object.fromEntries(Object.entries(localStorage)));
  expect(storage.managementKey).toBe('gateway-managed');
  if (storage['cli-proxy-auth']) expect(storage['cli-proxy-auth']).toContain('gateway-managed');
  expect(JSON.stringify(storage)).not.toContain('runtime-secret');

  const secretSeen = await page.evaluate(() => performance.getEntriesByType('resource').some(entry => entry.name.includes('runtime-secret')));
  expect(secretSeen).toBeFalsy();
  await expect(page.locator('.cpa-lifecycle-bar')).toBeInViewport();
  await page.screenshot({ path: testInfo.outputPath('cpa.png'), fullPage: true });
});
```

Use `page.context().request` exactly as shown so the login cookie reaches the page. Capture `page.on('request')` headers for management calls and assert any browser-supplied `authorization` or `x-management-key` value contains only `gateway-managed`; the panel may migrate the harmless placeholder into `cli-proxy-auth`, which is allowed. Add assertions that role 10 is redirected from `/cpa`, lifecycle controls do not overlap at mobile width, iframe bounding box has positive width/height, and a foreign-Origin `PUT /v0/management/config` returns 403.

- [ ] **Step 5: Run Playwright on desktop and mobile**

Run from `web`:

```powershell
npx playwright install chromium
npx playwright test
```

Expected: desktop and mobile projects PASS; screenshots show a nonblank official dashboard, visible Gateway navigation, no overlap, and no second login screen. Inspect the captured management request headers and confirm the browser sends only `gateway-managed`, never the real runtime password.

- [ ] **Step 6: Run complete backend and frontend verification**

Run from repository root:

```powershell
go test -race ./common ./model ./service/cpa ./service ./middleware ./controller ./router -count=1 -timeout 8m
go test ./... -count=1 -timeout 8m
Set-Location web
$env:CI='true'; npm test -- --runInBand --watchAll=false
npm run build
Set-Location ..
git diff --check
```

Expected: all tests and build PASS; `git diff --check` prints nothing. If `go test ./...` reports the known root embed prerequisite, verify `web/build/index.html` exists from the immediately preceding frontend build and rerun; do not waive any real compile or test failure.

- [ ] **Step 7: Security and acceptance audit**

Run these repository scans:

```powershell
rg -n "runtime-secret|ManagementPassword|X-Management-Key|Authorization" service/cpa controller/cpa.go router/api-router.go web/src/pages/CPA
rg -n "0\.0\.0\.0|allow-remote|disable-control-panel|disable-auto-update-panel" service/cpa
rg -n "v0/management|anthropic/callback|codex/callback|antigravity/callback" router controller service/cpa
```

Expected: secrets appear only in test fixtures and the server-side injection code; no API response or frontend code contains the runtime password; loopback and panel-disable invariants are enforced; exactly three public OAuth callback paths exist; no generic CPA root proxy exists.

- [ ] **Step 8: Commit**

```bash
git add service/cpa/full_management_integration_test.go web/package.json web/package-lock.json web/playwright.config.js web/e2e/start-gateway.ps1 web/e2e/cpa-management.spec.js web/build
git commit -m "test(cpa): verify full embedded management workflow"
```

## Completion Checklist

- [ ] Root opens `/cpa` and reaches Management Center v1.18.3 without a second login.
- [ ] Credential upload, list, field/status patch, download, delete, OAuth callback, quota reset, config, logs, API keys, provider, and plugin management requests reach CPA.
- [ ] Full YAML and unknown/plugin nodes survive management changes and restart.
- [ ] CPA stop drains in-flight management calls and immediately removes its provider from route selection without changing desired provider status.
- [ ] Admin/user/token/cross-origin callers cannot reach management; only the three exact callback relays are public.
- [ ] CPA binds only `127.0.0.1`; the real management password exists only inside `Manager` and upstream request headers.
- [ ] The embedded panel hash is exactly `941a49a619a719a59e4c7917c6888a53eb3f41a4fa2fbb5c1cc94f2d1fc9cd4b`.
- [ ] Backend race tests, frontend unit tests, production build, real CPA integration, and Playwright desktop/mobile tests all pass.
