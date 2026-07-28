package service

import (
	"fmt"
	"time"

	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
)

// RefreshExpiringTokens checks for tokens that will expire soon and refreshes them
func RefreshExpiringTokens() {
	// Check tokens expiring within 10 minutes
	const checkWindowSeconds = 600
	tokens, err := model.GetExpiringSoonTokens(checkWindowSeconds)
	if err != nil {
		common.SysLog(fmt.Sprintf("get expiring tokens failed: %v", err))
		return
	}

	if len(tokens) == 0 {
		return
	}

	refreshedCount := 0
	failedCount := 0

	for _, token := range tokens {
		if err := refreshProviderToken(token); err != nil {
			common.SysLog(fmt.Sprintf("refresh token failed for token %d (provider_id=%d, upstream_token_id=%d): %v",
				token.Id, token.ProviderId, token.UpstreamTokenId, err))
			failedCount++
		} else {
			refreshedCount++
		}
	}

	if refreshedCount > 0 || failedCount > 0 {
		common.SysLog(fmt.Sprintf("token refresh completed: %d refreshed, %d failed", refreshedCount, failedCount))
	}
}

func refreshProviderToken(token *model.ProviderToken) error {
	if token.RefreshToken == "" {
		return fmt.Errorf("refresh token is empty")
	}

	// Get provider info
	provider, err := model.GetProviderById(token.ProviderId)
	if err != nil {
		return fmt.Errorf("get provider failed: %w", err)
	}
	if provider == nil {
		return fmt.Errorf("provider not found")
	}

	// Skip key-only providers
	if provider.IsKeyOnly() {
		return fmt.Errorf("key-only provider does not support token refresh")
	}

	// Create upstream client
	client := NewUpstreamClient(provider.BaseURL, provider.AccessToken, provider.UserID)

	// Call refresh API
	refreshed, err := client.RefreshUpstreamToken(token.UpstreamTokenId, token.RefreshToken)
	if err != nil {
		return fmt.Errorf("upstream refresh failed: %w", err)
	}

	// Update local token
	token.SkKey = refreshed.AccessToken
	if refreshed.RefreshToken != "" {
		token.RefreshToken = refreshed.RefreshToken
	}
	if refreshed.ExpiresAt > 0 {
		token.ExpiresAt = refreshed.ExpiresAt
	}
	token.LastSynced = time.Now().Unix()

	if err := token.Update(); err != nil {
		return fmt.Errorf("update token failed: %w", err)
	}

	expiresAtStr := "unknown"
	if token.ExpiresAt > 0 {
		expiresAtStr = time.Unix(token.ExpiresAt, 0).Format("2006-01-02 15:04:05")
	}
	common.SysLog(fmt.Sprintf("token refreshed successfully: id=%d, provider=%s, expires_at=%s",
		token.Id, provider.Name, expiresAtStr))

	return nil
}
