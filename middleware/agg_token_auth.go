package middleware

import (
	"NewAPI-Gateway/model"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AggTokenAuth authenticates requests using aggregated tokens (ag-xxx)
func AggTokenAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 1. Extract token from various sources
		key := extractAggToken(c)
		if key == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "missing authentication token",
					"type":    "authentication_error",
					"code":    "invalid_api_key",
				},
			})
			c.Abort()
			return
		}

		// 2. Validate token
		token, user, err := model.ValidateAggToken(key)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": err.Error(),
					"type":    "authentication_error",
					"code":    "invalid_api_key",
				},
			})
			c.Abort()
			return
		}

		// 3. Check IP whitelist
		if !token.IsIPAllowed(c.ClientIP()) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"message": "IP not in allowed list",
					"type":    "permission_error",
					"code":    "ip_not_allowed",
				},
			})
			c.Abort()
			return
		}

		// 4. Set context
		c.Set("agg_token", token)
		c.Set("user", user)
		c.Set("user_id", user.Id)
		c.Next()
	}
}

func extractAggToken(c *gin.Context) string {
	// Standard: Authorization: Bearer ag-xxx
	auth := c.GetHeader("Authorization")
	if auth != "" {
		auth = strings.TrimPrefix(auth, "Bearer ")
		auth = strings.TrimPrefix(auth, "bearer ")
		auth = strings.TrimPrefix(auth, "ag-")
		return auth
	}

	// Anthropic compat: x-api-key
	apiKey := c.GetHeader("x-api-key")
	if apiKey != "" {
		return strings.TrimPrefix(apiKey, "ag-")
	}

	// Gemini compat: x-goog-api-key or query key
	googKey := c.GetHeader("x-goog-api-key")
	if googKey != "" {
		return strings.TrimPrefix(googKey, "ag-")
	}
	queryKey := c.Query("key")
	if queryKey != "" {
		return strings.TrimPrefix(queryKey, "ag-")
	}

	return ""
}
