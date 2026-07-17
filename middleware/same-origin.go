package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// SameOrigin validates that mutation requests (POST, PUT, PATCH, DELETE)
// originate from the same origin as the Gateway.
// Safe methods (GET, HEAD, OPTIONS) are allowed without origin checks.
func SameOrigin() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method

		// Safe methods don't require origin validation
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}

		// For mutations, validate origin
		requestOrigin := c.GetHeader("Origin")
		if requestOrigin == "" {
			// Fallback to Referer if Origin is missing
			referer := c.GetHeader("Referer")
			if referer != "" {
				if parsed, err := url.Parse(referer); err == nil {
					requestOrigin = parsed.Scheme + "://" + parsed.Host
				}
			}
		}

		if requestOrigin == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"code":    "origin_rejected",
				"message": "mutation requests require Origin or Referer header",
			})
			c.Abort()
			return
		}

		// Parse request origin
		originURL, err := url.Parse(requestOrigin)
		if err != nil || originURL.Host == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"code":    "origin_rejected",
				"message": "invalid origin format",
			})
			c.Abort()
			return
		}

		// Determine the Gateway's scheme
		gatewayScheme := "http"
		if c.Request.TLS != nil {
			gatewayScheme = "https"
		} else {
			// Trust single X-Forwarded-Proto value
			forwarded := c.GetHeader("X-Forwarded-Proto")
			if forwarded != "" && !strings.Contains(forwarded, ",") {
				gatewayScheme = strings.TrimSpace(forwarded)
			}
		}

		// Normalize hosts (case-insensitive)
		gatewayHost := strings.ToLower(c.Request.Host)
		originHost := strings.ToLower(originURL.Host)

		// Normalize scheme
		originScheme := strings.ToLower(originURL.Scheme)

		// Get effective ports
		gatewayPort := effectivePort(gatewayHost, gatewayScheme)
		originPort := effectivePort(originHost, originScheme)

		// Strip port from hosts for comparison
		gatewayHostOnly := stripPort(gatewayHost)
		originHostOnly := stripPort(originHost)

		// Compare scheme, host, and port
		if gatewayScheme != originScheme ||
			gatewayHostOnly != originHostOnly ||
			gatewayPort != originPort {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"code":    "origin_rejected",
				"message": "origin does not match Gateway",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// stripPort removes the port from a host string
func stripPort(host string) string {
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		return host[:idx]
	}
	return host
}

// effectivePort returns the port from host, or default for scheme
func effectivePort(host, scheme string) string {
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		return host[idx+1:]
	}

	// Default ports
	if scheme == "https" {
		return "443"
	}
	return "80"
}
