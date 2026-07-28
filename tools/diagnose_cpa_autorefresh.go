// diagnose_cpa_autorefresh.go - Diagnose why CPA auto-refresh is not starting
package main

import (
	"encoding/json"
	"fmt"
	"os"

	cpaconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run diagnose_cpa_autorefresh.go <cpa-config-path>")
		fmt.Println("Example: go run diagnose_cpa_autorefresh.go cpa/config.yaml")
		os.Exit(1)
	}

	configPath := os.Args[1]
	fmt.Printf("=== CPA Auto-Refresh Diagnostic ===\n\n")
	fmt.Printf("Config file: %s\n\n", configPath)

	// Load config
	cfg, err := cpaconfig.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("❌ Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Config loaded successfully\n\n")

	// Check Home.Enabled
	fmt.Printf("--- Home Configuration ---\n")
	fmt.Printf("  Home.Enabled: %v\n", cfg.Home.Enabled)
	fmt.Printf("  Home.NodeID: %q\n", cfg.Home.NodeID)
	fmt.Printf("  Home.Host: %q\n", cfg.Home.Host)
	fmt.Printf("  Home.Port: %d\n\n", cfg.Home.Port)

	// Condition check
	homeEnabled := cfg.Home.Enabled
	fmt.Printf("--- Auto-Refresh Start Condition ---\n")
	fmt.Printf("  homeEnabled = %v\n", homeEnabled)
	fmt.Printf("  !homeEnabled = %v\n", !homeEnabled)
	fmt.Printf("  Will auto-refresh start? %v\n\n", !homeEnabled)

	if homeEnabled {
		fmt.Printf("❌ Auto-refresh is DISABLED because Home.Enabled = true\n")
		fmt.Printf("\n💡 Solution:\n")
		fmt.Printf("   Home mode is for CLIProxyAPI Home (cluster management).\n")
		fmt.Printf("   For embedded Gateway usage, Home should be disabled.\n")
		fmt.Printf("   However, Home.Enabled cannot be set via YAML (it has yaml:\"-\" tag).\n")
		fmt.Printf("   It's only set programmatically via --home-jwt flag.\n")
		fmt.Printf("\n   Since you're using embedded mode, Home.Enabled should be false.\n")
		fmt.Printf("   If it's true, there's a bug in the embedding code.\n")
	} else {
		fmt.Printf("✅ Auto-refresh SHOULD start (homeEnabled = false)\n")
		fmt.Printf("\n🔍 But it's not showing in logs. Possible reasons:\n")
		fmt.Printf("   1. coreManager is nil (auth manager not initialized)\n")
		fmt.Printf("   2. Service.Run() is not being called (using HTTP server only)\n")
		fmt.Printf("   3. Context is cancelled before reaching auto-refresh start\n")
		fmt.Printf("   4. There's an error before the auto-refresh line\n")
	}

	// Check auth dir
	fmt.Printf("\n--- Auth Directory ---\n")
	fmt.Printf("  AuthDir: %q\n", cfg.AuthDir)
	if _, err := os.Stat(cfg.AuthDir); err != nil {
		fmt.Printf("  ❌ Directory does not exist or not accessible: %v\n", err)
	} else {
		fmt.Printf("  ✅ Directory exists\n")

		// Count auth files
		entries, err := os.ReadDir(cfg.AuthDir)
		if err != nil {
			fmt.Printf("  ❌ Cannot read directory: %v\n", err)
		} else {
			authCount := 0
			expiredCount := 0
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if len(name) > 4 && name[len(name)-5:] == ".json" {
					authCount++

					// Try to read and check expiration
					data, err := os.ReadFile(cfg.AuthDir + "/" + name)
					if err == nil {
						var token map[string]interface{}
						if json.Unmarshal(data, &token) == nil {
							if expired, ok := token["expired"].(string); ok {
								// Simple check: if expired contains "2026-07" and it's before now
								if len(expired) >= 10 && expired[:7] < "2026-07" {
									expiredCount++
								}
							}
						}
					}
				}
			}
			fmt.Printf("  Auth files: %d\n", authCount)
			fmt.Printf("  Potentially expired: %d\n", expiredCount)
		}
	}

	// Check if service would start auto-refresh
	fmt.Printf("\n--- Expected Behavior ---\n")
	fmt.Printf("When CPA Service.Run() is called:\n")
	if !homeEnabled {
		fmt.Printf("  1. ✅ homeEnabled = false\n")
		fmt.Printf("  2. ✅ Should call coreManager.StartAutoRefresh(ctx, 15*time.Minute)\n")
		fmt.Printf("  3. ✅ Should log: \"core auth auto-refresh started (interval=15m)\"\n")
		fmt.Printf("\n❓ But the log message is missing. Next steps:\n")
		fmt.Printf("   - Check if Service.Run() is actually being called\n")
		fmt.Printf("   - Add debug logging in service/cpa/embed.go before service.Run()\n")
		fmt.Printf("   - Check if there's an early error in Service.Run()\n")
		fmt.Printf("   - Verify coreManager is not nil\n")
	} else {
		fmt.Printf("  1. ❌ homeEnabled = true\n")
		fmt.Printf("  2. ❌ Will NOT call StartAutoRefresh\n")
		fmt.Printf("  3. ❌ No auto-refresh log message\n")
	}

	fmt.Printf("\n=== Diagnostic Complete ===\n")
}
