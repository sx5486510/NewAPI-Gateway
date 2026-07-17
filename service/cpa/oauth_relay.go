package cpa

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api"
)

// OAuthRelay relays OAuth provider callbacks to the embedded CPA instance.
// It validates state tokens, ensures one-time use, and strips sensitive headers.
type OAuthRelay struct {
	manager managementLeaseProvider

	// claimedStates tracks state tokens that have been claimed to ensure one-time use
	claimedStates sync.Map // string -> time.Time

	// Cleanup goroutine control
	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

// providerCallbackMap maps exact callback paths to their provider names
var providerCallbackMap = map[string]string{
	"/anthropic/callback":   "anthropic",
	"/codex/callback":       "codex",
	"/antigravity/callback": "antigravity",
}

// NewOAuthRelay creates a new OAuth relay with the given management lease provider.
func NewOAuthRelay(manager managementLeaseProvider) *OAuthRelay {
	relay := &OAuthRelay{
		manager:     manager,
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go relay.cleanupExpiredStates()

	return relay
}

// Close stops the cleanup goroutine
func (r *OAuthRelay) Close() {
	close(r.stopCleanup)
	<-r.cleanupDone
}

// RelayCallback handles OAuth provider callbacks
func (r *OAuthRelay) RelayCallback(c *gin.Context) {
	path := c.Request.URL.Path

	// Only accept exact callback paths
	expectedProvider, ok := providerCallbackMap[path]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "unknown callback path",
		})
		return
	}

	// Extract state parameter
	state := c.Query("state")
	if state == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "missing state parameter",
		})
		return
	}

	// Validate state format
	if err := api.ValidateOAuthState(state); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid state format",
		})
		return
	}

	// Get OAuth session
	sessionProvider, status, ok := api.GetOAuthSession(state)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "state not found",
		})
		return
	}

	// Verify provider matches
	if sessionProvider != expectedProvider {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "provider mismatch",
		})
		return
	}

	// Check error status
	if status != "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "OAuth session has error status",
		})
		return
	}

	// Atomically claim the state
	if !r.claimState(state) {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "state already used",
		})
		return
	}

	// Acquire management lease
	lease, err := r.manager.AcquireManagement()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"code":    "cpa_unavailable",
			"message": "CPA not available",
		})
		return
	}
	defer lease.Release()

	// Build upstream URL (preserve path and raw query)
	upstreamURL := fmt.Sprintf("%s%s", lease.Target.String(), path)
	if c.Request.URL.RawQuery != "" {
		upstreamURL += "?" + c.Request.URL.RawQuery
	}

	// Create upstream request without sensitive headers
	upstreamReq, err := http.NewRequest(c.Request.Method, upstreamURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to create upstream request",
		})
		return
	}

	// Copy safe headers only (no Cookie, Authorization, or management keys)
	for k, vv := range c.Request.Header {
		switch k {
		case "Cookie", "Authorization", "X-Management-Key", "Proxy-Authorization":
			// Skip sensitive headers
			continue
		default:
			for _, v := range vv {
				upstreamReq.Header.Add(k, v)
			}
		}
	}

	// Forward to CPA
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"code":    "upstream_failure",
			"message": "failed to reach CPA",
		})
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			c.Header(k, v)
		}
	}

	// Copy status and body
	c.Status(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

// claimState atomically claims a state token, returning true if successful
func (r *OAuthRelay) claimState(state string) bool {
	_, loaded := r.claimedStates.LoadOrStore(state, time.Now())
	return !loaded
}

// cleanupExpiredStates removes claimed states older than 31 minutes
func (r *OAuthRelay) cleanupExpiredStates() {
	defer close(r.cleanupDone)

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCleanup:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-31 * time.Minute)
			r.claimedStates.Range(func(key, value interface{}) bool {
				if claimTime, ok := value.(time.Time); ok && claimTime.Before(cutoff) {
					r.claimedStates.Delete(key)
				}
				return true
			})
		}
	}
}
