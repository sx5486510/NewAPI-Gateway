// Package controller - CPA configuration API endpoints.
//
// This file exposes three routes for managing the embedded CPA instance:
// - GET /api/cpa/config - retrieve current configuration
// - PUT /api/cpa/config - update configuration (persists to DB, re-materializes
//   config.yaml, triggers graceful reload if CPA is running)
// - POST /api/cpa/reload - force reload of the embedded CPA instance (reads
//   current DB config, re-materializes, restarts)
package controller

import (
	"fmt"
	"net/http"

	"NewAPI-Gateway/service"
	"NewAPI-Gateway/service/cpa"

	"github.com/gin-gonic/gin"
)

// GetCPAConfig returns the current CPA configuration from the database.
// GET /api/cpa/config
func GetCPAConfig(c *gin.Context) {
	cfg, err := cpa.LoadCPAConfigFromDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Failed to load CPA config: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    cfg,
	})
}

// UpdateCPAConfig accepts a new CPA configuration, validates it, persists to
// the database, re-materializes the config.yaml, and triggers a graceful reload
// of the embedded CPA instance (hot reload — no gateway restart needed).
// PUT /api/cpa/config
func UpdateCPAConfig(c *gin.Context) {
	var req cpa.CPAConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	// Basic validation
	if req.Port <= 0 || req.Port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Port must be between 1 and 65535",
		})
		return
	}
	if len(req.APIKeys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "At least one API key is required",
		})
		return
	}
	if req.AuthDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Auth directory cannot be empty",
		})
		return
	}

	// Save to DB
	if err := cpa.SaveCPAConfigToDB(&req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Failed to save config: %v", err),
		})
		return
	}

	// Re-materialize config.yaml
	if _, err := cpa.MaterializeCPAConfigFromDB(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Config saved but failed to materialize YAML: %v", err),
		})
		return
	}

	// Graceful reload: stops old instance, starts new one, re-registers provider
	if err := cpa.Reload(service.CPAProviderRegistrationCallback()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Config saved but reload failed: %v. Restart gateway manually.", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "CPA config updated and reloaded successfully.",
	})
}

// ReloadCPA forces a graceful reload of the embedded CPA instance: reads the
// current DB config, re-materializes config.yaml, stops the old instance (if
// running), and starts a new one.
// POST /api/cpa/reload
func ReloadCPA(c *gin.Context) {
	if _, err := cpa.MaterializeCPAConfigFromDB(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Failed to materialize config: %v", err),
		})
		return
	}
	if err := cpa.Reload(service.CPAProviderRegistrationCallback()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Reload failed: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "CPA reloaded successfully.",
	})
}
