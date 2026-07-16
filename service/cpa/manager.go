// Package cpa - lifecycle manager for the embedded CPA instance.
//
// The manager holds the currently-running instance handle so config updates can
// trigger a graceful restart (stop old, start new) without a full gateway
// restart. All access is guarded by a mutex since reloads may be triggered
// concurrently from the admin API.
package cpa

import (
	"sync"
	"time"

	"NewAPI-Gateway/common"
)

var (
	mgrMu   sync.Mutex
	current *EmbedResult
)

// StartFromDB materializes config from the DB and, if CPA is enabled, starts the
// embedded instance and records the handle. It is a no-op (returns nil) when
// CPAEnabled is false. Safe to call at startup.
//
// onReady, if non-nil, is invoked in a background goroutine with the started
// instance's BaseURL and APIKey once the instance is launched — used by the
// gateway to register/refresh the CPA upstream provider.
func StartFromDB(onReady func(baseURL, apiKey string)) error {
	cfg, err := LoadCPAConfigFromDB()
	if err != nil {
		return err
	}

	// Ensure auth directory exists before starting CPA (it scans this dir on launch).
	if err := ensureAuthDir(cfg.AuthDir); err != nil {
		return err
	}

	configPath, err := MaterializeCPAConfigFromDB()
	if err != nil {
		return err
	}

	if !cfg.Enabled {
		common.SysLog("embedded CPA disabled (CPAEnabled=false); not starting")
		return nil
	}

	res, err := StartEmbedded(configPath, cfg.Port)
	if err != nil {
		return err
	}

	mgrMu.Lock()
	current = res
	mgrMu.Unlock()

	if onReady != nil {
		go onReady(res.BaseURL, res.APIKey)
	}
	return nil
}

// Stop gracefully shuts down the current embedded instance, if any, and waits
// briefly for it to exit. Safe to call when nothing is running.
func Stop() {
	mgrMu.Lock()
	res := current
	current = nil
	mgrMu.Unlock()

	if res == nil {
		return
	}
	res.Cancel()
	select {
	case <-res.Done:
	case <-time.After(35 * time.Second):
		common.SysLog("embedded CPA did not stop within timeout")
	}
}

// Reload stops the running instance (if any), re-materializes config from the
// DB, and starts a fresh instance when CPAEnabled is true. This is how admin
// config changes take effect without restarting the gateway.
//
// onReady is forwarded to StartFromDB.
func Reload(onReady func(baseURL, apiKey string)) error {
	Stop()
	return StartFromDB(onReady)
}

// IsRunning reports whether an embedded instance is currently tracked.
func IsRunning() bool {
	mgrMu.Lock()
	defer mgrMu.Unlock()
	return current != nil
}
