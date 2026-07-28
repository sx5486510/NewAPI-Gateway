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

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run diagnose_auth_refresh.go <filename>")
		fmt.Println("Example: go run diagnose_auth_refresh.go xai-08av4ljy2n6l@me.23432453.xyz.json")
		os.Exit(1)
	}

	filename := os.Args[1]
	baseURL := "http://127.0.0.1:18317" // CPA 端口

	fmt.Printf("=== CPA Auth Refresh Diagnostics ===\n\n")
	fmt.Printf("Filename: %s\n", filename)
	fmt.Printf("CPA Base URL: %s\n\n", baseURL)

	// Step 1: Check CPA health
	fmt.Println("Step 1: Checking CPA health...")
	healthURL := baseURL + "/health"
	resp, err := http.Get(healthURL)
	if err != nil {
		fmt.Printf("❌ CPA health check failed: %v\n", err)
		fmt.Println("\n💡 Suggestion: Make sure Gateway is running with embedded CPA")
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Printf("✅ CPA is running (HTTP %d)\n\n", resp.StatusCode)
	} else {
		fmt.Printf("⚠️ CPA returned HTTP %d\n\n", resp.StatusCode)
	}

	// Step 2: Query auth file
	fmt.Println("Step 2: Querying auth file metadata...")
	queryURL := fmt.Sprintf("%s/v0/management/auth-files?filename=%s", baseURL, filename)

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		fmt.Printf("❌ Failed to create request: %v\n", err)
		os.Exit(1)
	}

	// Add management key (default from config.yaml)
	req.Header.Set("X-Management-Key", "cpa-default-key")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("HTTP Status: %d\n", resp.StatusCode)
	fmt.Printf("Response Body:\n%s\n\n", string(body))

	var queryResp struct {
		Success bool `json:"success"`
		Data    []struct {
			Filename string         `json:"filename"`
			Metadata map[string]any `json:"metadata"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &queryResp); err != nil {
		fmt.Printf("⚠️ Failed to parse response: %v\n", err)
		fmt.Println("Raw response above might contain clues")
		os.Exit(1)
	}

	if !queryResp.Success {
		fmt.Println("❌ Query returned success=false")
		os.Exit(1)
	}

	if len(queryResp.Data) == 0 {
		fmt.Println("❌ Auth file not found in CPA")
		fmt.Println("\n💡 Possible reasons:")
		fmt.Println("   1. File doesn't exist in auth-dir (C:\\Users\\shen\\.cli-proxy-api)")
		fmt.Println("   2. CPA hasn't indexed the file yet")
		fmt.Println("   3. Filename mismatch (case-sensitive)")
		os.Exit(1)
	}

	fmt.Printf("✅ Auth file found: %s\n", queryResp.Data[0].Filename)
	if exp, ok := queryResp.Data[0].Metadata["expired"].(string); ok {
		fmt.Printf("   Expired: %s\n", exp)
	}
	fmt.Println()

	// Step 3: Try to refresh
	fmt.Println("Step 3: Attempting token refresh...")
	refreshURL := baseURL + "/v0/management/auth-files/refresh"
	refreshPayload := map[string]string{"filename": filename}
	payloadBytes, _ := json.Marshal(refreshPayload)

	refreshReq, err := http.NewRequest("POST", refreshURL, bytes.NewReader(payloadBytes))
	if err != nil {
		fmt.Printf("❌ Failed to create refresh request: %v\n", err)
		os.Exit(1)
	}
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshReq.Header.Set("X-Management-Key", "cpa-default-key")

	resp, err = http.DefaultClient.Do(refreshReq)
	if err != nil {
		fmt.Printf("❌ Refresh request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read refresh response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("HTTP Status: %d\n", resp.StatusCode)
	fmt.Printf("Response Body:\n%s\n\n", string(body))

	var refreshResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Filename   string `json:"filename"`
			NewExpired string `json:"new_expired"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &refreshResp); err != nil {
		fmt.Printf("⚠️ Failed to parse refresh response: %v\n", err)
		os.Exit(1)
	}

	if !refreshResp.Success {
		fmt.Printf("❌ Refresh failed: %s\n", refreshResp.Message)
		os.Exit(1)
	}

	fmt.Println("✅ Token refresh successful!")
	fmt.Printf("   New Expired: %s\n", refreshResp.Data.NewExpired)
	fmt.Printf("   Refreshed At: %s\n", time.Now().Format(time.RFC3339))

	fmt.Println("\n=== Diagnostics Complete ===")
}
