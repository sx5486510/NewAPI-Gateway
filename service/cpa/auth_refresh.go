package cpa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func (p *ManagementProxy) handleAuthFileRefresh(w http.ResponseWriter, r *http.Request, lease *ManagementLease) bool {
	if r.Method != http.MethodPost || normalizePath(r.URL.Path) != "/v0/management/auth-files/refresh" {
		return false
	}

	var req struct {
		Filename string `json:"filename"`
		Name     string `json:"name"`
	}
	body := http.MaxBytesReader(w, r.Body, maxAPICallRequestBody)
	defer r.Body.Close()
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Invalid refresh request body")
		return true
	}
	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		filename = strings.TrimSpace(req.Name)
	}
	if filename == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "filename is required")
		return true
	}

	entry, err := p.xaiAuthEntryByName(r.Context(), lease, filename)
	if err != nil {
		// Expose the real cause to aid diagnosis (timeout, upstream status, decode error, etc.)
		writeJSONError(w, http.StatusBadGateway, "auth_file_list_failed", fmt.Sprintf("Failed to load auth files: %v", err))
		return true
	}
	if entry == nil {
		writeJSONError(w, http.StatusNotFound, "not_found", fmt.Sprintf("Auth file not found: %s", filename))
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(entry.Provider), "xai") {
		writeJSONError(w, http.StatusNotImplemented, "unsupported_provider", "Manual refresh is currently supported only for xAI auth files")
		return true
	}

	configStore, ok := p.store.(xaiQuotaConfigStore)
	if !ok {
		writeJSONError(w, http.StatusBadGateway, "auth_directory_unavailable", "CPA auth directory is unavailable")
		return true
	}
	basic, err := configStore.Basic()
	if err != nil || basic == nil {
		writeJSONError(w, http.StatusBadGateway, "auth_directory_unavailable", "CPA auth directory is unavailable")
		return true
	}
	authPath, err := secureAuthFilePath(basic.AuthDir, entry.Name)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_auth_file", err.Error())
		return true
	}

	result, err, _ := p.xaiRefresh.Do("manual-refresh:"+entry.AuthIndex, func() (interface{}, error) {
		return p.refreshXAIAuthFile(context.WithoutCancel(r.Context()), lease, entry.AuthIndex, authPath)
	})
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "refresh_failed", xaiCredentialPreparationMessage(err))
		return true
	}
	refreshed, ok := result.(*xaiAuthRefreshResult)
	if !ok || refreshed == nil {
		writeJSONError(w, http.StatusBadGateway, "refresh_failed", "invalid xAI credential preparation result")
		return true
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Token refreshed successfully",
		"data": map[string]interface{}{
			"filename":     entry.Name,
			"old_expired":  refreshed.OldExpired,
			"new_expired":  refreshed.NewExpired,
			"refreshed_at": refreshed.RefreshedAt,
		},
	})
	return true
}

type xaiAuthRefreshResult struct {
	OldExpired  string
	NewExpired  string
	RefreshedAt string
}

func (p *ManagementProxy) refreshXAIAuthFile(ctx context.Context, lease *ManagementLease, authIndex, authPath string) (*xaiAuthRefreshResult, error) {
	body, err := os.ReadFile(authPath)
	if err != nil {
		return nil, fmt.Errorf("read xAI credential: %w", err)
	}
	var credential xaiCredential
	if err := json.Unmarshal(body, &credential); err != nil {
		return nil, errors.New("parse xAI credential")
	}
	if strings.TrimSpace(credential.RefreshToken) == "" {
		if strings.TrimSpace(credential.AccessToken) == "" {
			return nil, errors.New("xAI credential access token and refresh token are missing")
		}
		return nil, errors.New("xAI credential refresh token is missing")
	}

	endpoint := strings.TrimSpace(credential.TokenEndpoint)
	if endpoint == "" {
		endpoint, err = p.discoverXAITokenEndpoint(ctx, lease, authIndex)
	} else {
		endpoint, err = validateXAITokenEndpoint(endpoint)
	}
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	token, err := p.refreshXAIToken(ctx, lease, authIndex, endpoint, credential.RefreshToken)
	if err != nil {
		return nil, err
	}
	if err := persistXAIToken(authPath, body, token, now); err != nil {
		return nil, err
	}

	newExpired := ""
	if token.ExpiresIn > 0 {
		newExpired = now.Add(time.Duration(token.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	return &xaiAuthRefreshResult{
		OldExpired:  strings.TrimSpace(credential.Expired),
		NewExpired:  newExpired,
		RefreshedAt: now.Format(time.RFC3339),
	}, nil
}

func (p *ManagementProxy) xaiAuthEntryByName(ctx context.Context, lease *ManagementLease, name string) (*xaiAuthListEntry, error) {
	entries, err := p.xaiAuthEntries(ctx, lease)
	if err != nil {
		return nil, err
	}
	for index := range entries {
		if entries[index].Name == name {
			return &entries[index], nil
		}
	}
	return nil, nil
}

func (p *ManagementProxy) xaiAuthEntries(ctx context.Context, lease *ManagementLease) ([]xaiAuthListEntry, error) {
	target := *lease.Target
	target.Path = "/v0/management/auth-files"
	target.RawQuery = ""
	target.Fragment = ""

	// Use independent timeout to avoid inheriting an already-expired parent context
	listCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(listCtx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth files list request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+lease.Password)

	resp, err := p.transport.RoundTrip(req)
	if err != nil {
		// Return precise error cause
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("auth files list request canceled: %w", err)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("auth files list request timeout (30s): %w", err)
		}
		return nil, fmt.Errorf("auth files list request failed: %w", err)
	}
	defer resp.Body.Close()

	if !isSuccess(resp.StatusCode) {
		// Read error body for better diagnostics
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("auth files list returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Stream decode to handle arbitrarily large responses (no size limit)
	var result struct {
		Files []xaiAuthListEntry `json:"files"`
	}
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode auth files list response (status %d): %w", resp.StatusCode, err)
	}
	return result.Files, nil
}
