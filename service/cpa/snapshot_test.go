package cpa

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type memoryOptions struct {
	mu     sync.Mutex
	values map[string]string
	setErr error
}

func newMemoryOptions(values map[string]string) *memoryOptions {
	copyValues := make(map[string]string, len(values))
	for key, value := range values {
		copyValues[key] = value
	}
	return &memoryOptions{values: copyValues}
}

func (m *memoryOptions) Get(key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.values[key]
}

func (m *memoryOptions) Set(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setErr != nil {
		return m.setErr
	}
	m.values[key] = value
	return nil
}

func newTestSnapshotStore(t *testing.T, options *memoryOptions) *SnapshotStore {
	t.Helper()
	invariants, err := NewRuntimeInvariants(bytes.NewReader(bytes.Repeat([]byte{0x5a}, 64)))
	if err != nil {
		t.Fatalf("NewRuntimeInvariants: %v", err)
	}
	store := NewSnapshotStore(filepath.Join(t.TempDir(), "runtime"), invariants)
	store.options = options
	return store
}

func TestSnapshotStoreMigratesLegacyOptions(t *testing.T) {
	authDir := filepath.Join(t.TempDir(), "auth")
	options := newMemoryOptions(map[string]string{
		"CPAEnabled": "true",
		"CPAAPIKeys": `["key-a","key-b"]`,
		"CPAAuthDir": authDir,
		"CPAPort":    "29001",
	})
	store := newTestSnapshotStore(t, options)

	snapshot, cfg, err := store.LoadOrMigrate()
	if err != nil {
		t.Fatalf("LoadOrMigrate: %v", err)
	}
	if cfg.Host != loopbackHost || cfg.Port != 29001 {
		t.Fatalf("runtime address = %s:%d", cfg.Host, cfg.Port)
	}
	if len(cfg.APIKeys) != 2 || cfg.APIKeys[0] != "key-a" || cfg.AuthDir != authDir {
		t.Fatalf("migrated config = %+v", cfg)
	}
	if cfg.RemoteManagement.AllowRemote {
		t.Fatalf("default migrated config should keep allow-remote false: %+v", cfg.RemoteManagement)
	}
	if cfg.RemoteManagement.SecretKey == "" || !strings.HasPrefix(cfg.RemoteManagement.SecretKey, "$2") {
		t.Fatalf("management sentinel is not bcrypt: %q", cfg.RemoteManagement.SecretKey)
	}
	if got := options.Get(snapshotOptionKey); got != string(snapshot) {
		t.Fatalf("snapshot option mismatch\n got: %s\nwant: %s", got, snapshot)
	}
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("runtime config stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("runtime config mode = %o, want 600", info.Mode().Perm())
	}
}

func TestSnapshotStoreRecoversValidDiskAfterInterruptedPersistence(t *testing.T) {
	options := newMemoryOptions(map[string]string{
		snapshotOptionKey: "host: 127.0.0.1\nport: 29001\napi-keys: [db]\nauth-dir: auth\n",
	})
	store := newTestSnapshotStore(t, options)
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(), []byte("host: 127.0.0.1\nport: 29002\napi-keys: [disk]\nauth-dir: auth\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	snapshot, cfg, err := store.LoadOrMigrate()
	if err != nil {
		t.Fatalf("LoadOrMigrate: %v", err)
	}
	if cfg.Port != 29002 || len(cfg.APIKeys) != 1 || cfg.APIKeys[0] != "disk" {
		t.Fatalf("disk recovery config = %+v", cfg)
	}
	if options.Get(snapshotOptionKey) != string(snapshot) {
		t.Fatal("recovered disk snapshot was not persisted")
	}
}

func TestSnapshotStoreRejectsInvalidDatabaseAndDisk(t *testing.T) {
	store := newTestSnapshotStore(t, newMemoryOptions(map[string]string{snapshotOptionKey: "[not-a-mapping]"}))
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(), []byte("port: nope"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.LoadOrMigrate(); err == nil {
		t.Fatal("expected invalid disk and database snapshots to fail")
	}
}

func TestSnapshotStoreRejectsDuplicateKeys(t *testing.T) {
	store := newTestSnapshotStore(t, newMemoryOptions(map[string]string{
		snapshotOptionKey: "host: 127.0.0.1\nport: 29001\nport: 29002\napi-keys: [key]\nauth-dir: auth\n",
	}))
	if _, _, err := store.LoadOrMigrate(); err == nil || !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Fatalf("expected duplicate-key error, got %v", err)
	}
}

func TestPatchBasicPreservesUnknownAndPluginConfiguration(t *testing.T) {
	raw := "# retained comment\nhost: 0.0.0.0\nport: 18317\nauth-dir: old\napi-keys: [old]\nplugins:\n  instances:\n    demo:\n      custom-field: keep-me\nunknown-future-field: 42\n"
	options := newMemoryOptions(map[string]string{snapshotOptionKey: raw, "CPAEnabled": "true"})
	store := newTestSnapshotStore(t, options)
	if _, _, err := store.LoadOrMigrate(); err != nil {
		t.Fatalf("LoadOrMigrate: %v", err)
	}

	err := store.PatchBasic(CPAConfig{Enabled: false, APIKeys: []string{"new"}, AuthDir: "new-auth", Port: 29003})
	if err != nil {
		t.Fatalf("PatchBasic: %v", err)
	}
	got := options.Get(snapshotOptionKey)
	for _, want := range []string{"# retained comment", "custom-field: keep-me", "unknown-future-field: 42", "port: 18317", "new-auth", "- new"} {
		if !strings.Contains(got, want) {
			t.Fatalf("snapshot lost %q:\n%s", want, got)
		}
	}
	if options.Get("CPAEnabled") != "false" {
		t.Fatalf("CPAEnabled = %q", options.Get("CPAEnabled"))
	}
}

func TestPersistRuntimePreservesAllFieldsExceptHostAndPort(t *testing.T) {
	raw := "host: 127.0.0.1\nport: 29007\napi-keys: [key]\nauth-dir: auth\nremote-management:\n  allow-remote: false\n  secret-key: initial-secret\n  disable-control-panel: true\n"
	options := newMemoryOptions(map[string]string{snapshotOptionKey: raw})
	store := newTestSnapshotStore(t, options)
	if _, _, err := store.LoadOrMigrate(); err != nil {
		t.Fatalf("LoadOrMigrate: %v", err)
	}

	// Simulate CPA writing a full config mutation before Gateway persists the
	// snapshot. Only host and port should be restored to their pinned values.
	mutated := "host: 0.0.0.0\nport: 39007\napi-keys: [new-key]\nauth-dir: new-auth\nremote-management:\n  allow-remote: true\n  secret-key: user-mutated\n  disable-control-panel: false\nunknown-future-field: changed\n"
	if err := os.WriteFile(store.Path(), []byte(mutated), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.PersistRuntime(); err != nil {
		t.Fatalf("PersistRuntime: %v", err)
	}

	got := options.Get(snapshotOptionKey)
	if !strings.Contains(got, "allow-remote: true") {
		t.Fatalf("allow-remote:true was not preserved:\n%s", got)
	}
	if !strings.Contains(got, "host: 127.0.0.1") {
		t.Fatalf("host was not forced to loopback:\n%s", got)
	}
	if !strings.Contains(got, "port: 29007") || strings.Contains(got, "port: 39007") {
		t.Fatalf("port modification was not ignored:\n%s", got)
	}
	for _, want := range []string{"api-keys: [new-key]", "auth-dir: new-auth", "secret-key: user-mutated", "disable-control-panel: false", "unknown-future-field: changed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("editable field %q was not preserved:\n%s", want, got)
		}
	}
}

func TestAtomicWriteLeavesOriginalOnRenameFailure(t *testing.T) {
	options := newMemoryOptions(map[string]string{
		snapshotOptionKey: "host: 127.0.0.1\nport: 29001\napi-keys: [old]\nauth-dir: auth\n",
		"CPAEnabled":      "true",
	})
	store := newTestSnapshotStore(t, options)
	if _, _, err := store.LoadOrMigrate(); err != nil {
		t.Fatal(err)
	}
	originalFile, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	originalOption := options.Get(snapshotOptionKey)
	store.renameFile = func(string, string) error { return errors.New("rename failed") }

	err = store.PatchBasic(CPAConfig{Enabled: true, APIKeys: []string{"new"}, AuthDir: "auth", Port: 29002})
	if err == nil || !strings.Contains(err.Error(), "rename failed") {
		t.Fatalf("expected rename failure, got %v", err)
	}
	after, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, originalFile) {
		t.Fatal("original runtime file changed after failed rename")
	}
	if options.Get(snapshotOptionKey) != originalOption {
		t.Fatal("database snapshot changed after failed rename")
	}
	if _, err := os.Stat(store.Path() + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file remains: %v", err)
	}
}

// TestAtomicWriteRestoresOriginalWhenReplaceFails exercises the recovery path:
// the original is renamed aside to a backup, the temp->target rename then fails,
// and the backup must be restored so the original config survives.
func TestAtomicWriteRestoresOriginalWhenReplaceFails(t *testing.T) {
	options := newMemoryOptions(map[string]string{
		snapshotOptionKey: "host: 127.0.0.1\nport: 29001\napi-keys: [old]\nauth-dir: auth\n",
		"CPAEnabled":      "true",
	})
	store := newTestSnapshotStore(t, options)
	if _, _, err := store.LoadOrMigrate(); err != nil {
		t.Fatal(err)
	}
	originalFile, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	originalOption := options.Get(snapshotOptionKey)

	// Succeed on the backup (target->.bak) and restore (.bak->target) renames,
	// but fail the temp->target replace to trigger the recovery path.
	target := store.Path()
	backup := target + ".bak"
	store.renameFile = func(oldPath, newPath string) error {
		if oldPath == target && newPath == backup {
			return os.Rename(oldPath, newPath)
		}
		if oldPath == backup && newPath == target {
			return os.Rename(oldPath, newPath)
		}
		return errors.New("replace failed")
	}

	err = store.PatchBasic(CPAConfig{Enabled: true, APIKeys: []string{"new"}, AuthDir: "auth", Port: 29002})
	if err == nil || !strings.Contains(err.Error(), "replace failed") {
		t.Fatalf("expected replace failure, got %v", err)
	}
	after, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, originalFile) {
		t.Fatal("original runtime file was not restored after failed replace")
	}
	if options.Get(snapshotOptionKey) != originalOption {
		t.Fatal("database snapshot changed after failed replace")
	}
	if _, err := os.Stat(store.Path() + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file remains: %v", err)
	}
	if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup file remains: %v", err)
	}
}
