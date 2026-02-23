package controller

import (
	"NewAPI-Gateway/model"
	"NewAPI-Gateway/service"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Relay is the main proxy handler for all OpenAI-compatible API calls
func Relay(c *gin.Context) {
	aggToken := c.MustGet("agg_token").(*model.AggregatedToken)

	// 1. Extract model from request body
	modelName := extractModelFromBody(c)
	if modelName == "" {
		modelName = "unknown"
	}
	c.Set("request_model_original", modelName)
	c.Set("request_model", modelName)

	// 2. Check model whitelist
	if !aggToken.IsModelAllowed(modelName) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": "model not allowed: " + modelName,
				"type":    "permission_error",
				"code":    "model_not_allowed",
			},
		})
		return
	}

	// 3. Build retry plan: retry all routes within one priority first, then downgrade.
	plan, err := model.BuildRouteAttemptsByPriority(modelName)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": gin.H{
				"message": "no available provider for model: " + modelName,
				"type":    "server_error",
				"code":    "service_unavailable",
			},
		})
		return
	}

	var lastErr *service.ProxyAttemptError
	for _, priorityGroup := range plan {
		for _, attempt := range priorityGroup {
			if attempt.Route.ModelName != "" {
				c.Set("request_model_resolved", attempt.Route.ModelName)
				c.Set("request_model", attempt.Route.ModelName)
			}

			if proxyErr := service.ProxyToUpstream(c, attempt.Token, attempt.Provider); proxyErr == nil {
				return
			} else {
				lastErr = proxyErr
				if !proxyErr.Retryable {
					statusCode := proxyErr.StatusCode
					if statusCode <= 0 {
						statusCode = http.StatusBadGateway
					}
					c.JSON(statusCode, gin.H{
						"error": gin.H{
							"message": proxyErr.Message,
							"type":    "server_error",
							"code":    "upstream_request_failed",
						},
					})
					return
				}
			}
		}
	}

	message := "no available provider for model: " + modelName
	if lastErr != nil && lastErr.Message != "" {
		message = "all providers failed for model: " + modelName
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "server_error",
			"code":    "service_unavailable",
		},
	})
}

// ListModels returns all available models across all providers
func ListModels(c *gin.Context) {
	models, err := model.GetDistinctModels()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data":   []gin.H{},
		})
		return
	}

	var modelList []gin.H
	for _, m := range models {
		modelList = append(modelList, gin.H{
			"id":       m,
			"object":   "model",
			"owned_by": "aggregated-gateway",
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   modelList,
	})
}

func GetModel(c *gin.Context) {
	modelName := c.Param("model")
	models, _ := model.GetDistinctModels()
	for _, m := range models {
		if m == modelName {
			c.JSON(http.StatusOK, gin.H{
				"id":       m,
				"object":   "model",
				"owned_by": "aggregated-gateway",
			})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": gin.H{
			"message": "model not found: " + modelName,
			"type":    "invalid_request_error",
		},
	})
}

// BillingSubscription returns a fake billing subscription for compatibility
func BillingSubscription(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"object":                "billing_subscription",
		"has_payment_method":    true,
		"hard_limit_usd":        999999,
		"soft_limit_usd":        999999,
		"system_hard_limit_usd": 999999,
		"access_until":          4102444800, // 2100-01-01
	})
}

// BillingUsage returns usage data for compatibility
func BillingUsage(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"object":      "list",
		"total_usage": 0,
	})
}

func extractModelFromBody(c *gin.Context) string {
	// Read body and restore it
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	// Restore body for later use by proxy
	c.Request.Body = io.NopCloser(io.NopCloser(
		&readCloserWrapper{data: bodyBytes, pos: 0},
	))

	var body struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return ""
	}
	return body.Model
}

type readCloserWrapper struct {
	data []byte
	pos  int
}

func (r *readCloserWrapper) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *readCloserWrapper) Close() error {
	return nil
}
