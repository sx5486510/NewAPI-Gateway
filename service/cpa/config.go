package cpa

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CPAConfig is the backward-compatible Gateway view of the embedded CPA
// configuration. The complete CPA document is stored and managed by
// SnapshotStore; this type patches only these four legacy fields.
type CPAConfig struct {
	Enabled bool     `json:"enabled"`
	APIKeys []string `json:"api_keys"`
	AuthDir string   `json:"auth_dir"`
	Port    int      `json:"port"`
}

func expandHome(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return p
	}
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func ensureAuthDir(authDir string) error {
	resolved := expandHome(authDir)
	if resolved == "" {
		return fmt.Errorf("cpa: auth dir is empty")
	}
	if err := os.MkdirAll(resolved, 0o700); err != nil {
		return fmt.Errorf("cpa: create auth dir %q: %w", resolved, err)
	}
	return nil
}

var (
	defaultSnapshotMu          sync.Mutex
	defaultInvariantsOnce      sync.Once
	defaultRuntimeInvariants   *RuntimeInvariants
	defaultRuntimeInvariantErr error
)

func newDefaultSnapshotStore() (*SnapshotStore, error) {
	defaultInvariantsOnce.Do(func() {
		defaultRuntimeInvariants, defaultRuntimeInvariantErr = NewRuntimeInvariants(nil)
	})
	if defaultRuntimeInvariantErr != nil {
		return nil, defaultRuntimeInvariantErr
	}
	return NewSnapshotStore("", defaultRuntimeInvariants), nil
}

func LoadCPAConfigFromDB() (*CPAConfig, error) {
	defaultSnapshotMu.Lock()
	defer defaultSnapshotMu.Unlock()
	store, err := newDefaultSnapshotStore()
	if err != nil {
		return nil, err
	}
	return store.Basic()
}

func SaveCPAConfigToDB(cfg *CPAConfig) error {
	if cfg == nil {
		return fmt.Errorf("cpa: config is nil")
	}
	defaultSnapshotMu.Lock()
	defer defaultSnapshotMu.Unlock()
	store, err := newDefaultSnapshotStore()
	if err != nil {
		return err
	}
	return store.PatchBasic(*cfg)
}

func MaterializeCPAConfigFromDB() (string, error) {
	defaultSnapshotMu.Lock()
	defer defaultSnapshotMu.Unlock()
	store, err := newDefaultSnapshotStore()
	if err != nil {
		return "", err
	}
	if _, _, err := store.LoadOrMigrate(); err != nil {
		return "", err
	}
	return store.Path(), nil
}
