package middleware

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"net/url"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"net/http"
)

func authHelper(c *gin.Context, minRole int) {
	session := sessions.Default(c)
	username := session.Get("username")
	role := session.Get("role")
	id := session.Get("id")
	status := session.Get("status")
	authByToken := false
	if username == nil {
		// Check token
		token := c.Request.Header.Get("Authorization")
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "无权进行此操作，未登录或 token 无效",
			})
			c.Abort()
			return
		}
		user := model.ValidateUserToken(token)
		if user != nil && user.Username != "" {
			// Token is valid
			username = user.Username
			role = user.Role
			id = user.Id
			status = user.Status
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无权进行此操作，token 无效",
			})
			c.Abort()
			return
		}
		authByToken = true
	}
	if status.(int) == common.UserStatusDisabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户已被封禁",
		})
		c.Abort()
		return
	}
	if role.(int) < minRole {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权进行此操作，权限不足",
		})
		c.Abort()
		return
	}
	c.Set("username", username)
	c.Set("role", role)
	c.Set("id", id)
	c.Set("authByToken", authByToken)
	c.Next()
}

func UserAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleCommonUser)
	}
}

func AdminAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleAdminUser)
	}
}

func RootAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleRootUser)
	}
}

// CPAManagementAuth allows either Root session OR X-Management-Key/Bearer token.
// For Root sessions: enforces NoTokenAuth and SameOrigin.
// For X-Management-Key: bypasses session checks, lets CPA validate the key.
func CPAManagementAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if request has X-Management-Key or Authorization Bearer
		hasManagementKey := c.GetHeader("X-Management-Key") != ""
		if !hasManagementKey {
			if auth := c.GetHeader("Authorization"); auth != "" {
				// Only treat as management key if it's a Bearer token
				// (session-based requests also use Authorization but go through cookie flow)
				parts := strings.SplitN(auth, " ", 2)
				if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
					hasManagementKey = true
				}
			}
		}

		if hasManagementKey {
			// External API mode: allow the request, let ManagementProxy + CPA validate the key
			c.Next()
			return
		}

		// Internal browser mode: enforce Root session + NoTokenAuth + SameOrigin
		authHelper(c, common.RoleRootUser)
		if c.IsAborted() {
			return
		}

		// NoTokenAuth check
		authByToken := c.GetBool("authByToken")
		if authByToken {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "本接口不支持使用 token 进行验证",
			})
			c.Abort()
			return
		}

		// SameOrigin check for mutations
		method := c.Request.Method
		if method == http.MethodPost || method == http.MethodPut ||
			method == http.MethodPatch || method == http.MethodDelete {

			requestOrigin := c.GetHeader("Origin")
			if requestOrigin == "" {
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

			gatewayScheme := "http"
			if c.Request.TLS != nil {
				gatewayScheme = "https"
			} else {
				forwarded := c.GetHeader("X-Forwarded-Proto")
				if forwarded != "" && !strings.Contains(forwarded, ",") {
					gatewayScheme = strings.TrimSpace(forwarded)
				}
			}

			gatewayHost := strings.ToLower(c.Request.Host)
			originHost := strings.ToLower(originURL.Host)
			originScheme := strings.ToLower(originURL.Scheme)

			gatewayPort := effectivePortFromSameOrigin(gatewayHost, gatewayScheme)
			originPort := effectivePortFromSameOrigin(originHost, originScheme)

			gatewayHostOnly := stripPortFromSameOrigin(gatewayHost)
			originHostOnly := stripPortFromSameOrigin(originHost)

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
		}

		c.Next()
	}
}

func stripPortFromSameOrigin(host string) string {
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		return host[:idx]
	}
	return host
}

func effectivePortFromSameOrigin(host, scheme string) string {
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		return host[idx+1:]
	}

	if scheme == "https" {
		return "443"
	}
	return "80"
}

// NoTokenAuth You should always use this after normal auth middlewares.
func NoTokenAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authByToken := c.GetBool("authByToken")
		if authByToken {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "本接口不支持使用 token 进行验证",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// TokenOnlyAuth You should always use this after normal auth middlewares.
func TokenOnlyAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authByToken := c.GetBool("authByToken")
		if !authByToken {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "本接口仅支持使用 token 进行验证",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
