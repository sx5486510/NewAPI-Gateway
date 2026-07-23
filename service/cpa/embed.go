// Package cpa embeds the CLIProxyAPI (CPA) proxy server into the gateway.
package cpa

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"NewAPI-Gateway/common"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	cpaconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// loopbackHost is the only interface CPA is allowed to bind to when embedded.
// Keeping CPA on loopback guarantees the gateway remains the single externally
// reachable service.
const loopbackHost = "127.0.0.1"

// EmbedResult carries the handles and connection details of a started embedded
// CPA instance.
type EmbedResult struct {
	// Cancel triggers graceful shutdown of the embedded service.
	Cancel func()
	// Done is closed once the service goroutine has exited.
	Done <-chan struct{}
	// Errors receives a non-cancellation runtime error before Done closes.
	Errors <-chan error
	// BaseURL is the loopback base URL other components use to reach CPA, e.g.
	// "http://127.0.0.1:18317".
	BaseURL string
	// APIKey is the first configured CPA client API key, used by the gateway to
	// authenticate to the embedded instance. Empty if none is configured.
	APIKey string
}

// StartEmbedded loads a CPA configuration file, forces it to listen on
// loopback only, and starts the CPA service in a background goroutine.
//
// It returns an EmbedResult with shutdown handles and the loopback base URL /
// API key. On configuration/build failure it returns an error and does not
// start anything.
func StartEmbedded(configPath, managementPassword string) (*EmbedResult, error) {
	trimmedPath := strings.TrimSpace(configPath)
	if trimmedPath == "" {
		return nil, fmt.Errorf("cpa: config path is empty")
	}
	if strings.TrimSpace(managementPassword) == "" {
		return nil, fmt.Errorf("cpa: runtime management password is empty")
	}
	cfg, err := cpaconfig.LoadConfig(trimmedPath)
	if err != nil {
		return nil, fmt.Errorf("cpa: load config %q failed: %w", trimmedPath, err)
	}
	if cfg == nil {
		return nil, fmt.Errorf("cpa: config %q resolved to nil", trimmedPath)
	}

	port := cfg.Port
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("cpa: invalid internal port %d", port)
	}
	// Reassert the only Gateway-owned value after parsing. All other CPA
	// configuration fields are read from the validated YAML snapshot as-is.
	cfg.Host = loopbackHost

	// Expand a leading "~" in the auth dir. CPA's own entrypoint
	// (cmd/server/main.go) resolves this via util.ResolveAuthDir, but the SDK
	// builder path we use for embedding does not. Without this, CPA writes and
	// lists auth files under a literal "~" directory in the process CWD, which
	// diverges from the real home directory and surfaces as duplicate/desynced
	// auth-file entries.
	cfg.AuthDir = expandHome(cfg.AuthDir)

	apiKey := ""
	if len(cfg.APIKeys) > 0 {
		apiKey = strings.TrimSpace(cfg.APIKeys[0])
	}

	service, err := cliproxy.NewBuilder().
		WithConfig(cfg).
		WithConfigPath(trimmedPath).
		WithLocalManagementPassword(managementPassword).
		Build()
	if err != nil {
		return nil, fmt.Errorf("cpa: build service failed: %w", err)
	}

	ctx, cancelFn := context.WithCancel(context.Background())
	doneCh := make(chan struct{})
	errorsCh := make(chan error, 1)

	go func() {
		defer close(doneCh)
		defer close(errorsCh)
		common.SysLog(fmt.Sprintf("embedded CPA starting on %s:%d", loopbackHost, port))
		if runErr := service.Run(ctx); runErr != nil && !errors.Is(runErr, context.Canceled) {
			errorsCh <- runErr
			common.SysLog("embedded CPA exited with an error")
			return
		}
		common.SysLog("embedded CPA stopped")
	}()

	return &EmbedResult{
		Cancel:  cancelFn,
		Done:    doneCh,
		Errors:  errorsCh,
		BaseURL: fmt.Sprintf("http://%s:%d", loopbackHost, port),
		APIKey:  apiKey,
	}, nil
}
