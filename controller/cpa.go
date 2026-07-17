package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"NewAPI-Gateway/service/cpa"

	"github.com/gin-gonic/gin"
)

// GetCPAStatus returns the current CPA lifecycle status.
// GET /api/cpa/status
func GetCPAStatus(c *gin.Context) {
	runtime := cpa.DefaultRuntime()
	if runtime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"code":    "runtime_unavailable",
			"message": "CPA runtime not initialized",
		})
		return
	}

	status := runtime.Manager.Status()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
	})
}

// StartCPA starts the embedded CPA instance.
// POST /api/cpa/start
func StartCPA(c *gin.Context) {
	runtime := cpa.DefaultRuntime()
	if runtime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"code":    "runtime_unavailable",
			"message": "CPA runtime not initialized",
		})
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := runtime.Manager.Start(ctx)
	duration := time.Since(start)

	auditLifecycleAction(c, "start", err, duration)

	if err != nil {
		handleLifecycleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "CPA started successfully",
		"data":    runtime.Manager.Status(),
	})
}

// StopCPA stops the embedded CPA instance.
// POST /api/cpa/stop
func StopCPA(c *gin.Context) {
	runtime := cpa.DefaultRuntime()
	if runtime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"code":    "runtime_unavailable",
			"message": "CPA runtime not initialized",
		})
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := runtime.Manager.Stop(ctx)
	duration := time.Since(start)

	auditLifecycleAction(c, "stop", err, duration)

	if err != nil {
		handleLifecycleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "CPA stopped successfully",
		"data":    runtime.Manager.Status(),
	})
}

// RestartCPA restarts the embedded CPA instance.
// POST /api/cpa/restart
func RestartCPA(c *gin.Context) {
	runtime := cpa.DefaultRuntime()
	if runtime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"code":    "runtime_unavailable",
			"message": "CPA runtime not initialized",
		})
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	err := runtime.Manager.Restart(ctx)
	duration := time.Since(start)

	auditLifecycleAction(c, "restart", err, duration)

	if err != nil {
		handleLifecycleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "CPA restarted successfully",
		"data":    runtime.Manager.Status(),
	})
}

// GetCPAConfig returns the basic CPA configuration.
// GET /api/cpa/config
func GetCPAConfig(c *gin.Context) {
	runtime := cpa.DefaultRuntime()
	if runtime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"code":    "runtime_unavailable",
			"message": "CPA runtime not initialized",
		})
		return
	}

	cfg, err := runtime.Store.Basic()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"code":    "config_load_failed",
			"message": fmt.Sprintf("Failed to load config: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":  cfg.Enabled,
			"api_keys": cfg.APIKeys,
			"auth_dir": cfg.AuthDir,
			"port":     cfg.Port,
		},
	})
}

// UpdateCPAConfig updates the basic CPA configuration and restarts.
// PUT /api/cpa/config
func UpdateCPAConfig(c *gin.Context) {
	runtime := cpa.DefaultRuntime()
	if runtime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"code":    "runtime_unavailable",
			"message": "CPA runtime not initialized",
		})
		return
	}

	var req struct {
		Enabled bool     `json:"enabled"`
		APIKeys []string `json:"api_keys"`
		AuthDir string   `json:"auth_dir"`
		Port    int      `json:"port"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    "invalid_request",
			"message": fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	// Validate
	if req.Port < 1 || req.Port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    "invalid_port",
			"message": "Port must be between 1 and 65535",
		})
		return
	}

	hasNonBlank := false
	for _, key := range req.APIKeys {
		if key != "" {
			hasNonBlank = true
			break
		}
	}
	if !hasNonBlank {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    "invalid_api_keys",
			"message": "At least one nonblank API key is required",
		})
		return
	}

	if req.AuthDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    "invalid_auth_dir",
			"message": "Auth directory cannot be empty",
		})
		return
	}

	// Patch basic config (preserves unknown YAML)
	nextCfg := cpa.CPAConfig{
		Enabled: req.Enabled,
		APIKeys: req.APIKeys,
		AuthDir: req.AuthDir,
		Port:    req.Port,
	}

	if err := runtime.Store.PatchBasic(nextCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"code":    "patch_failed",
			"message": fmt.Sprintf("Failed to patch config: %v", err),
		})
		return
	}

	// Restart to apply changes
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	err := runtime.Manager.Restart(ctx)
	duration := time.Since(start)

	auditLifecycleAction(c, "config_update", err, duration)

	if err != nil {
		handleLifecycleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "CPA config updated and restarted successfully",
		"data":    runtime.Manager.Status(),
	})
}

// ReloadCPA is an alias for restart.
// POST /api/cpa/reload
func ReloadCPA(c *gin.Context) {
	RestartCPA(c)
}

// ServeCPAPanel serves the embedded management panel.
// GET /api/cpa/panel
func ServeCPAPanel(c *gin.Context) {
	runtime := cpa.DefaultRuntime()
	if runtime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"code":    "runtime_unavailable",
			"message": "CPA runtime not initialized",
		})
		return
	}

	runtime.Panel.ServeHTTP(c.Writer, c.Request)
}

// ProxyCPAManagement proxies complete /v0/management API to embedded CPA.
// ANY /v0/management/*
func ProxyCPAManagement(c *gin.Context) {
	runtime := cpa.DefaultRuntime()
	if runtime == nil || runtime.Proxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"code":    "runtime_unavailable",
			"message": "CPA runtime not initialized",
		})
		return
	}

	runtime.Proxy.ServeHTTP(c.Writer, c.Request)
}

// RelayCPAOAuthCallback relays OAuth provider callbacks to embedded CPA.
// GET /anthropic/callback, /codex/callback, /antigravity/callback
func RelayCPAOAuthCallback(c *gin.Context) {
	runtime := cpa.DefaultRuntime()
	if runtime == nil || runtime.OAuth == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"code":    "cpa_unavailable",
			"message": "CPA runtime not initialized",
		})
		return
	}
	runtime.OAuth.RelayCallback(c)
}

// handleLifecycleError maps lifecycle errors to stable HTTP responses.
func handleLifecycleError(c *gin.Context, err error) {
	if errors.Is(err, cpa.ErrTransitionConflict) {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"code":    "transition_conflict",
			"message": "Another lifecycle operation is in progress",
		})
		return
	}

	if errors.Is(err, cpa.ErrUnavailable) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"code":    "cpa_unavailable",
			"message": "CPA is not available",
		})
		return
	}

	c.JSON(http.StatusInternalServerError, gin.H{
		"success": false,
		"code":    "lifecycle_failed",
		"message": "Lifecycle operation failed",
	})
}

// auditLifecycleAction logs lifecycle operations without secrets.
func auditLifecycleAction(c *gin.Context, action string, err error, duration time.Duration) {
	username := c.GetString("username")
	if username == "" {
		username = "unknown"
	}

	status := "success"
	if err != nil {
		status = "failed"
	}

	// Log without sensitive details
	c.Set("cpa_audit", fmt.Sprintf("user=%s action=%s status=%s duration=%dms",
		username, action, status, duration.Milliseconds()))
}
