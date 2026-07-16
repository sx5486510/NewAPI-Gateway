// Package cpa embeds the CLIProxyAPI (CPA) proxy server into the gateway
// process. It runs CPA in a background goroutine, bound to loopback only, so
// the gateway exposes a single external port while CPA acts as an internal
// upstream provider.
//
// This is a proof-of-concept: it wires the exported CLIProxyAPI SDK
// (sdk/cliproxy.Builder + Service.Run) into the gateway's lifecycle. Automatic
// provider registration and admin configuration are intentionally out of scope
// here.
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
func StartEmbedded(configPath string, port int) (*EmbedResult, error) {
	trimmedPath := strings.TrimSpace(configPath)
	if trimmedPath == "" {
		return nil, fmt.Errorf("cpa: config path is empty")
	}
	if port <= 0 {
		return nil, fmt.Errorf("cpa: invalid internal port %d", port)
	}

	cfg, err := cpaconfig.LoadConfig(trimmedPath)
	if err != nil {
		return nil, fmt.Errorf("cpa: load config %q failed: %w", trimmedPath, err)
	}
	if cfg == nil {
		return nil, fmt.Errorf("cpa: config %q resolved to nil", trimmedPath)
	}

	// Force loopback binding so CPA is never externally reachable.
	cfg.Host = loopbackHost
	cfg.Port = port

	apiKey := ""
	if len(cfg.APIKeys) > 0 {
		apiKey = strings.TrimSpace(cfg.APIKeys[0])
	}

	service, err := cliproxy.NewBuilder().
		WithConfig(cfg).
		WithConfigPath(trimmedPath).
		Build()
	if err != nil {
		return nil, fmt.Errorf("cpa: build service failed: %w", err)
	}

	ctx, cancelFn := context.WithCancel(context.Background())
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		common.SysLog(fmt.Sprintf("embedded CPA starting on %s:%d", loopbackHost, port))
		if runErr := service.Run(ctx); runErr != nil && !errors.Is(runErr, context.Canceled) {
			common.SysLog("embedded CPA exited with error: " + runErr.Error())
			return
		}
		common.SysLog("embedded CPA stopped")
	}()

	return &EmbedResult{
		Cancel:  cancelFn,
		Done:    doneCh,
		BaseURL: fmt.Sprintf("http://%s:%d", loopbackHost, port),
		APIKey:  apiKey,
	}, nil
}
