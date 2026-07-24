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

		gatewayScheme, gatewayHost := resolveGatewayOrigin(c)
		originScheme := strings.ToLower(originURL.Scheme)
		originHost := strings.ToLower(originURL.Host)

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

// resolveGatewayOrigin reconstructs the public Gateway origin as seen by the
// browser. Behind reverse proxies Request.Host may be rewritten to the
// upstream loopback address, so we prefer a single trusted
// X-Forwarded-Host / X-Forwarded-Proto when present.
func resolveGatewayOrigin(c *gin.Context) (scheme, host string) {
	scheme = "http"
	if c.Request.TLS != nil {
		scheme = "https"
	} else if forwarded := singleForwardedValue(c.GetHeader("X-Forwarded-Proto")); forwarded != "" {
		scheme = forwarded
	}

	host = strings.ToLower(strings.TrimSpace(c.Request.Host))
	if forwardedHost := singleForwardedValue(c.GetHeader("X-Forwarded-Host")); forwardedHost != "" {
		host = strings.ToLower(forwardedHost)
	}
	return scheme, host
}

func singleForwardedValue(header string) string {
	value := strings.TrimSpace(header)
	if value == "" || strings.Contains(value, ",") {
		return ""
	}
	return value
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
