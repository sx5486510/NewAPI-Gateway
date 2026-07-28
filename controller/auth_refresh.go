package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"NewAPI-Gateway/service/cpa"

	"github.com/gin-gonic/gin"
)

// RefreshAuthToken manually refreshes an expired or expiring auth token via CPA management API.
// POST /api/auth/refresh
//
// Request body:
//
//	{
//	  "filename": "xai-08av4ljy2n6l@me.23432453.xyz.json"
//	}
//
// Response:
//
//	{
//	  "success": true,
//	  "message": "Token refreshed successfully",
//	  "data": {
//	    "filename": "xai-08av4ljy2n6l@me.23432453.xyz.json",
//	    "old_expired": "2026-07-20T14:30:00Z",
//	    "new_expired": "2026-07-27T20:30:00Z",
//	    "refreshed_at": "2026-07-27T14:35:22Z"
//	  }
//	}
func RefreshAuthToken(c *gin.Context) {
	runtime := cpa.DefaultRuntime()
	if runtime == nil || runtime.Proxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"code":    "runtime_unavailable",
			"message": "CPA runtime not initialized",
		})
		return
	}

	var req struct {
		Filename string `json:"filename" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    "invalid_request",
			"message": fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	refreshPayload := map[string]string{
		"filename": req.Filename,
	}

	payloadBytes, err := json.Marshal(refreshPayload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"code":    "marshal_error",
			"message": fmt.Sprintf("Failed to marshal refresh request: %v", err),
		})
		return
	}

	refreshReq, err := http.NewRequest("POST", "/v0/management/auth-files/refresh", bytes.NewReader(payloadBytes))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"code":    "request_failed",
			"message": fmt.Sprintf("Failed to create refresh request: %v", err),
		})
		return
	}

	refreshReq.Header.Set("Content-Type", "application/json")
	refreshReq = cpa.WithManagementAuditUser(refreshReq, c.GetString("username"))

	refreshRecorder := &responseRecorder{
		ResponseWriter: c.Writer,
		body:           &bytes.Buffer{},
	}

	runtime.Proxy.ServeHTTP(refreshRecorder, refreshReq)

	statusCode := refreshRecorder.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	contentType := refreshRecorder.Header().Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(statusCode, contentType, refreshRecorder.body.Bytes())
}

// responseRecorder captures HTTP response for internal processing.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	if r.body != nil {
		return r.body.Write(data)
	}
	return len(data), nil
}
