// refresh_token_via_api.go - Refresh token via Gateway API (command-line tool)
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	defaultBaseURL = "http://localhost:3000"
	apiEndpoint    = "/api/auth/refresh"
)

type RefreshRequest struct {
	Filename string `json:"filename"`
}

type RefreshResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Filename    string `json:"filename"`
		OldExpired  string `json:"old_expired"`
		NewExpired  string `json:"new_expired"`
		RefreshedAt string `json:"refreshed_at"`
	} `json:"data"`
	Code string `json:"code,omitempty"`
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run refresh_token_via_api.go <admin-token> <auth-filename>")
		fmt.Println("")
		fmt.Println("Arguments:")
		fmt.Println("  admin-token    Your Gateway admin Bearer token")
		fmt.Println("  auth-filename  The authentication file name (e.g., xai-xxx.json)")
		fmt.Println("")
		fmt.Println("Example:")
		fmt.Println("  go run refresh_token_via_api.go sk-xxxxx xai-08av4ljy2n6l@me.23432453.xyz.json")
		fmt.Println("")
		fmt.Println("Environment Variables:")
		fmt.Println("  GATEWAY_BASE_URL  Gateway base URL (default: http://localhost:3000)")
		os.Exit(1)
	}

	adminToken := os.Args[1]
	authFilename := os.Args[2]

	baseURL := os.Getenv("GATEWAY_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	fmt.Printf("🔄 Refreshing token via Gateway API...\n")
	fmt.Printf("   Gateway: %s\n", baseURL)
	fmt.Printf("   File: %s\n\n", authFilename)

	// Prepare request
	reqBody := RefreshRequest{
		Filename: authFilename,
	}
	reqData, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Printf("❌ Failed to marshal request: %v\n", err)
		os.Exit(1)
	}

	// Create HTTP request
	url := baseURL + apiEndpoint
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqData))
	if err != nil {
		fmt.Printf("❌ Failed to create request: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	// Send request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ Failed to send request: %v\n", err)
		fmt.Println("\nPossible reasons:")
		fmt.Println("  1. Gateway is not running")
		fmt.Println("  2. Wrong base URL (try setting GATEWAY_BASE_URL)")
		fmt.Println("  3. Network connectivity issues")
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Read response
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read response: %v\n", err)
		os.Exit(1)
	}

	var result RefreshResponse
	if err := json.Unmarshal(respData, &result); err != nil {
		fmt.Printf("❌ Failed to parse response: %v\n", err)
		fmt.Printf("Raw response: %s\n", string(respData))
		os.Exit(1)
	}

	// Check result
	if !result.Success {
		fmt.Printf("❌ Token refresh failed: %s\n", result.Message)
		if result.Code != "" {
			fmt.Printf("   Error code: %s\n", result.Code)
		}
		fmt.Println("\nCommon issues:")
		fmt.Println("  - Refresh token is missing or expired")
		fmt.Println("  - CPA runtime not initialized")
		fmt.Println("  - Authentication file not found")
		fmt.Println("  - Invalid admin token")
		os.Exit(1)
	}

	// Success
	fmt.Printf("✅ Token refreshed successfully!\n\n")
	fmt.Printf("📋 Details:\n")
	fmt.Printf("   File: %s\n", result.Data.Filename)
	fmt.Printf("   Old Expired: %s\n", result.Data.OldExpired)
	fmt.Printf("   New Expired: %s\n", result.Data.NewExpired)
	fmt.Printf("   Refreshed At: %s\n", result.Data.RefreshedAt)
	fmt.Println("")
	fmt.Println("💡 The token has been automatically updated in the Gateway.")
	fmt.Println("   No need to restart the service.")
}
