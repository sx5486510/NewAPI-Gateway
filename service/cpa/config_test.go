package cpa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"NewAPI-Gateway/common"

	cpaconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// seedOptionMap sets up the in-memory OptionMap with CPA defaults for tests.
func seedOptionMap(enabled bool, apiKeysJSON, authDir, port string) {
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap["CPAEnabled"] = map[bool]string{true: "true", false: "false"}[enabled]
	common.OptionMap["CPAAPIKeys"] = apiKeysJSON
	common.OptionMap["CPAAuthDir"] = authDir
	common.OptionMap["CPAPort"] = port
	common.OptionMapRWMutex.Unlock()
}

func TestLoadCPAConfigFromDB(t *testing.T) {
	seedOptionMap(true, `["key-a","key-b"]`, "/tmp/auth", "29001")

	cfg, err := LoadCPAConfigFromDB()
	if err != nil {
		t.Fatalf("LoadCPAConfigFromDB: %v", err)
	}
	if !cfg.Enabled {
		t.Fatalf("expected enabled=true")
	}
	if len(cfg.APIKeys) != 2 || cfg.APIKeys[0] != "key-a" || cfg.APIKeys[1] != "key-b" {
		t.Fatalf("api keys mismatch: %v", cfg.APIKeys)
	}
	if cfg.AuthDir != "/tmp/auth" {
		t.Fatalf("auth dir mismatch: %s", cfg.AuthDir)
	}
	if cfg.Port != 29001 {
		t.Fatalf("port mismatch: %d", cfg.Port)
	}
}

func TestLoadCPAConfigDefaults(t *testing.T) {
	// Empty/invalid values should fall back to sensible defaults.
	seedOptionMap(false, "", "", "not-a-number")

	cfg, err := LoadCPAConfigFromDB()
	if err != nil {
		t.Fatalf("LoadCPAConfigFromDB: %v", err)
	}
	if cfg.Enabled {
		t.Fatalf("expected enabled=false")
	}
	if len(cfg.APIKeys) != 1 || cfg.APIKeys[0] != "cpa-default-key" {
		t.Fatalf("expected default api key, got %v", cfg.APIKeys)
	}
	if cfg.AuthDir != "~/.cli-proxy-api" {
		t.Fatalf("expected default auth dir, got %s", cfg.AuthDir)
	}
	if cfg.Port != 18317 {
		t.Fatalf("expected default port 18317, got %d", cfg.Port)
	}
}

func TestMaterializeCPAConfigFromDB(t *testing.T) {
	// Run in a temp working dir so the .smoke output doesn't pollute the repo.
	origWd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	seedOptionMap(true, `["mat-key"]`, filepath.Join(tmp, "auth"), "29002")

	path, err := MaterializeCPAConfigFromDB()
	if err != nil {
		t.Fatalf("MaterializeCPAConfigFromDB: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	// The written file must be loadable by CPA and contain our values.
	loaded, err := cpaconfig.LoadConfig(path)
	if err != nil {
		t.Fatalf("reload written config: %v", err)
	}
	if loaded.Port != 29002 {
		t.Fatalf("materialized port = %d, want 29002", loaded.Port)
	}
	if len(loaded.APIKeys) != 1 || loaded.APIKeys[0] != "mat-key" {
		t.Fatalf("materialized api keys = %v", loaded.APIKeys)
	}
	if !strings.Contains(loaded.AuthDir, "auth") {
		t.Fatalf("materialized auth dir = %s", loaded.AuthDir)
	}
}
