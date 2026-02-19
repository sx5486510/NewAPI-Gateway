package controller

import (
	"encoding/json"
	"gin-template/model"
	"gin-template/service"
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

	// 3. Route to provider with retry
	maxRetry := 3
	for retry := 0; retry < maxRetry; retry++ {
		providerToken, provider, err := model.SelectProviderToken(modelName, retry)
		if err != nil {
			if retry == maxRetry-1 {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error": gin.H{
						"message": "no available provider for model: " + modelName,
						"type":    "server_error",
						"code":    "service_unavailable",
					},
				})
				return
			}
			continue
		}

		// 4. Proxy to upstream
		service.ProxyToUpstream(c, providerToken, provider)
		return
	}
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
