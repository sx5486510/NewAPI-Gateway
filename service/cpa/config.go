// Package cpa - CPA configuration materialization and hot-reload logic.
//
// This file handles the pipeline: DB option storage → physical config.yaml →
// embedded CPA instance. When admin updates CPA settings via the web UI, we
// write the new values to the option table, re-materialize the config file,
// and trigger a CPA reload (graceful restart).
package cpa

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"

	cpaconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// optionGet reads a single option value from the in-memory OptionMap under the
// shared read lock.
func optionGet(key string) string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return common.OptionMap[key]
}

// CPAConfig mirrors the subset of CPA config.yaml fields we expose in the
// gateway admin UI. Only base fields are included; upstream credentials
// live in the auth directory (OAuth login, out of scope here).
type CPAConfig struct {
	// Enabled controls whether the embedded CPA service should start.
	Enabled bool `json:"enabled"`
	// APIKeys is the list of bearer tokens clients use to call CPA (the gateway
	// itself uses the first key when proxying to CPA as an upstream provider).
	APIKeys []string `json:"api_keys"`
	// AuthDir is the directory where CPA's OAuth login state is stored. Defaults
	// to ~/.cli-proxy-api. Must exist and contain valid login credentials for CPA
	// to serve real upstream models.
	AuthDir string `json:"auth_dir"`
	// Port is the loopback port CPA binds to (always 127.0.0.1:<Port>).
	Port int `json:"port"`
}

// LoadCPAConfigFromDB reads the four CPA-related option keys and unmarshals
// them into a CPAConfig struct. Falls back to defaults if keys are missing.
func LoadCPAConfigFromDB() (*CPAConfig, error) {
	enabledStr := optionGet("CPAEnabled")
	enabled := strings.TrimSpace(enabledStr) == "true"

	apiKeysJSON := optionGet("CPAAPIKeys")
	var apiKeys []string
	if strings.TrimSpace(apiKeysJSON) == "" {
		apiKeys = []string{"cpa-default-key"}
	} else {
		if err := json.Unmarshal([]byte(apiKeysJSON), &apiKeys); err != nil {
			return nil, fmt.Errorf("cpa: parse CPAAPIKeys JSON: %w", err)
		}
	}

	authDir := optionGet("CPAAuthDir")
	if authDir == "" {
		authDir = "~/.cli-proxy-api"
	}

	portStr := optionGet("CPAPort")
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		port = 18317
	}

	return &CPAConfig{
		Enabled: enabled,
		APIKeys: apiKeys,
		AuthDir: authDir,
		Port:    port,
	}, nil
}

// SaveCPAConfigToDB writes a CPAConfig back to the option table (persisting to
// the DB and updating the in-memory OptionMap via model.UpdateOption).
func SaveCPAConfigToDB(cfg *CPAConfig) error {
	enabledStr := "false"
	if cfg.Enabled {
		enabledStr = "true"
	}
	if err := model.UpdateOption("CPAEnabled", enabledStr); err != nil {
		return fmt.Errorf("cpa: write CPAEnabled: %w", err)
	}

	apiKeysJSON, err := json.Marshal(cfg.APIKeys)
	if err != nil {
		return fmt.Errorf("cpa: marshal APIKeys: %w", err)
	}
	if err := model.UpdateOption("CPAAPIKeys", string(apiKeysJSON)); err != nil {
		return fmt.Errorf("cpa: write CPAAPIKeys: %w", err)
	}

	if err := model.UpdateOption("CPAAuthDir", cfg.AuthDir); err != nil {
		return fmt.Errorf("cpa: write CPAAuthDir: %w", err)
	}

	if err := model.UpdateOption("CPAPort", strconv.Itoa(cfg.Port)); err != nil {
		return fmt.Errorf("cpa: write CPAPort: %w", err)
	}

	return nil
}

// MaterializeCPAConfigFromDB reads the DB, generates a minimal config.yaml,
// and writes it to disk. Returns the path to the written file.
//
// If CPAEnabled=false, this still writes the file (so it's ready when the
// admin toggles it on), but the caller should skip StartEmbedded.
func MaterializeCPAConfigFromDB() (string, error) {
	cfg, err := LoadCPAConfigFromDB()
	if err != nil {
		return "", err
	}

	// Build a minimal CPA config struct. We only populate the fields managed
	// by the gateway; everything else (plugins, routing, etc.) stays default.
	cpaCfg := &cpaconfig.Config{
		Host:    loopbackHost, // forced at embed time, but set here too
		Port:    cfg.Port,
		AuthDir: cfg.AuthDir,
		SDKConfig: cpaconfig.SDKConfig{
			APIKeys: cfg.APIKeys,
		},
	}

	// Write to .smoke/cpa-config.yaml (reuse .smoke dir from PoC)
	outDir := filepath.Join(".", ".smoke")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("cpa: mkdir %s: %w", outDir, err)
	}

	outPath := filepath.Join(outDir, "cpa-config.yaml")

	// SaveConfigPreserveComments requires the file to exist AND be a valid YAML
	// mapping (it reads + merges first). If it doesn't exist yet, seed it with a
	// minimal valid mapping so the merge can proceed.
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		stub := fmt.Sprintf("# CPA config managed by gateway\nhost: %q\nport: %d\n", loopbackHost, cfg.Port)
		if err := os.WriteFile(outPath, []byte(stub), 0644); err != nil {
			return "", fmt.Errorf("cpa: create stub config %s: %w", outPath, err)
		}
	}

	if err := cpaconfig.SaveConfigPreserveComments(outPath, cpaCfg); err != nil {
		return "", fmt.Errorf("cpa: write config %s: %w", outPath, err)
	}

	return outPath, nil
}
