package cpa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const xaiRefreshSkew = 5 * time.Minute

const (
	xaiOAuthClientID = "b1a00492-073a-47ea-816f-4c329264a828"
	xaiDiscoveryURL  = "https://auth.x.ai/.well-known/openid-configuration"
)

type xaiQuotaConfigStore interface {
	Basic() (*CPAConfig, error)
}

type xaiAuthListEntry struct {
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	AuthIndex string `json:"auth_index"`
}

type xaiCredential struct {
	Type          string `json:"type"`
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token"`
	IDToken       string `json:"id_token"`
	TokenType     string `json:"token_type"`
	ExpiresIn     int    `json:"expires_in"`
	Expired       string `json:"expired"`
	LastRefresh   string `json:"last_refresh"`
	Subject       string `json:"sub"`
	TokenEndpoint string `json:"token_endpoint"`
}

type xaiQuotaCredential struct {
	AccessToken string
	Subject     string
}

type xaiTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// isXAIManagedURL reports whether an api-call target is an xAI endpoint whose
// request should receive a freshly refreshed access token. This covers both the
// billing/quota endpoint and the Grok CLI chat endpoint used by the connectivity
// test, all served from cli-chat-proxy.grok.com. Without this, the api-call would
// forward a stale (possibly expired) $TOKEN$ and the request would fail spuriously.
func isXAIManagedURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" ||
		!strings.EqualFold(parsed.Hostname(), "cli-chat-proxy.grok.com") {
		return false
	}
	switch parsed.Path {
	case "/v1/billing", "/v1/responses", "/v1/responses/compact", "/v1/chat/completions":
		return true
	default:
		return false
	}
}

func secureAuthFilePath(authDir, name string) (string, error) {
	if name == "" || filepath.Base(name) != name || !strings.EqualFold(filepath.Ext(name), ".json") {
		return "", errors.New("invalid auth file name")
	}
	root, err := filepath.Abs(expandHome(authDir))
	if err != nil {
		return "", err
	}
	candidate, err := filepath.Abs(filepath.Join(root, name))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("auth file is outside configured directory")
	}
	return candidate, nil
}

func rawJSONString(payload map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var value string
		if json.Unmarshal(raw, &value) == nil && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func headerValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func setHeaderValue(headers map[string]string, name, value string) {
	for key := range headers {
		if strings.EqualFold(key, name) {
			headers[key] = value
			return
		}
	}
	headers[name] = value
}

func (p *ManagementProxy) prepareXAIQuotaAPICall(ctx context.Context, body []byte, lease *ManagementLease) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, nil
	}
	if !isXAIManagedURL(rawJSONString(payload, "url", "URL")) {
		return body, nil
	}
	authIndex := rawJSONString(payload, "auth_index", "authIndex", "AuthIndex")
	if authIndex == "" {
		return body, nil
	}
	var headers map[string]string
	if err := json.Unmarshal(payload["header"], &headers); err != nil || headers == nil {
		return body, nil
	}
	if !strings.Contains(headerValue(headers, "Authorization"), "$TOKEN$") {
		return body, nil
	}

	entry, err := p.xaiAuthEntry(ctx, lease, authIndex)
	if err != nil {
		return nil, err
	}
	if entry == nil || !strings.EqualFold(strings.TrimSpace(entry.Provider), "xai") {
		return body, nil
	}
	configStore, ok := p.store.(xaiQuotaConfigStore)
	if !ok {
		return nil, errors.New("CPA auth directory is unavailable")
	}
	basic, err := configStore.Basic()
	if err != nil || basic == nil {
		return nil, fmt.Errorf("load CPA auth directory: %w", err)
	}
	authPath, err := secureAuthFilePath(basic.AuthDir, entry.Name)
	if err != nil {
		return nil, err
	}
	prepared, err, _ := p.xaiRefresh.Do(authIndex, func() (interface{}, error) {
		return p.loadXAIQuotaCredential(context.WithoutCancel(ctx), lease, authIndex, authPath)
	})
	if err != nil {
		return nil, err
	}
	credential, ok := prepared.(*xaiQuotaCredential)
	if !ok || credential == nil {
		return nil, errors.New("invalid xAI credential preparation result")
	}

	setHeaderValue(headers, "Authorization", strings.ReplaceAll(headerValue(headers, "Authorization"), "$TOKEN$", credential.AccessToken))
	if headerValue(headers, "x-userid") == "" && credential.Subject != "" {
		setHeaderValue(headers, "x-userid", credential.Subject)
	}
	payload["header"], err = json.Marshal(headers)
	if err != nil {
		return nil, err
	}
	return json.Marshal(payload)
}

func xaiCredentialPreparationMessage(err error) string {
	const fallback = "Failed to prepare xAI credentials"
	if err == nil {
		return fallback
	}
	message := strings.TrimSpace(err.Error())
	switch {
	case message == "CPA auth directory is unavailable",
		message == "invalid xAI credential preparation result",
		message == "parse xAI credential",
		message == "xAI credential access token and refresh token are missing",
		message == "xAI credential refresh token is missing",
		message == "xAI token endpoint must use https",
		message == "xAI token endpoint must be on x.ai",
		message == "xAI OpenID discovery failed",
		message == "xAI OpenID discovery returned an invalid response",
		message == "xAI token refresh failed",
		message == "xAI token refresh returned an invalid response",
		message == "parse xAI credential for persistence",
		message == "xAI auth index was not found",
		message == "xAI refresh token expired or invalid",
		message == "xAI refresh token unauthorized":
		return message
	case strings.HasPrefix(message, "xAI token refresh failed (HTTP "):
		return message
	default:
		return fallback
	}
}

func (p *ManagementProxy) loadXAIQuotaCredential(ctx context.Context, lease *ManagementLease, authIndex, authPath string) (*xaiQuotaCredential, error) {
	body, err := os.ReadFile(authPath)
	if err != nil {
		return nil, fmt.Errorf("read xAI credential: %w", err)
	}
	var credential xaiCredential
	if err := json.Unmarshal(body, &credential); err != nil {
		return nil, errors.New("parse xAI credential")
	}
	accessToken := strings.TrimSpace(credential.AccessToken)
	expired, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(credential.Expired))
	if accessToken != "" && (parseErr != nil || expired.After(time.Now().Add(xaiRefreshSkew))) {
		return &xaiQuotaCredential{AccessToken: accessToken, Subject: strings.TrimSpace(credential.Subject)}, nil
	}
	if strings.TrimSpace(credential.RefreshToken) == "" {
		if accessToken == "" {
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
	token, err := p.refreshXAIToken(ctx, lease, authIndex, endpoint, credential.RefreshToken)
	if err != nil {
		return nil, err
	}
	if err := persistXAIToken(authPath, body, token, time.Now()); err != nil {
		return nil, err
	}
	return &xaiQuotaCredential{AccessToken: strings.TrimSpace(token.AccessToken), Subject: strings.TrimSpace(credential.Subject)}, nil
}

func validateXAITokenEndpoint(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" {
		return "", errors.New("xAI token endpoint must use https")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host != "x.ai" && !strings.HasSuffix(host, ".x.ai") {
		return "", errors.New("xAI token endpoint must be on x.ai")
	}
	return parsed.String(), nil
}

func (p *ManagementProxy) discoverXAITokenEndpoint(ctx context.Context, lease *ManagementLease, authIndex string) (string, error) {
	requestBody, err := json.Marshal(map[string]interface{}{
		"authIndex": authIndex,
		"method":    http.MethodGet,
		"url":       xaiDiscoveryURL,
		"header": map[string]string{
			"Accept": "application/json",
		},
	})
	if err != nil {
		return "", err
	}
	result, err := p.callEmbeddedAPICall(ctx, lease, requestBody)
	if err != nil || result.StatusCode != http.StatusOK {
		return "", errors.New("xAI OpenID discovery failed")
	}
	var discovery struct {
		TokenEndpoint string `json:"token_endpoint"`
	}
	if err := decodeNestedBody(result.Body, &discovery); err != nil {
		return "", errors.New("xAI OpenID discovery returned an invalid response")
	}
	return validateXAITokenEndpoint(discovery.TokenEndpoint)
}

func (p *ManagementProxy) refreshXAIToken(ctx context.Context, lease *ManagementLease, authIndex, endpoint, refreshToken string) (*xaiTokenResponse, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {xaiOAuthClientID},
		"refresh_token": {strings.TrimSpace(refreshToken)},
	}
	requestBody, err := json.Marshal(map[string]interface{}{
		"authIndex": authIndex,
		"method":    http.MethodPost,
		"url":       endpoint,
		"header": map[string]string{
			"Accept":       "application/json",
			"Content-Type": "application/x-www-form-urlencoded",
		},
		"data": form.Encode(),
	})
	if err != nil {
		return nil, err
	}
	result, err := p.callEmbeddedAPICall(ctx, lease, requestBody)
	if err != nil {
		return nil, errors.New("xAI token refresh failed")
	}
	if result.StatusCode != http.StatusOK {
		switch result.StatusCode {
		case http.StatusBadRequest:
			return nil, errors.New("xAI refresh token expired or invalid")
		case http.StatusUnauthorized:
			return nil, errors.New("xAI refresh token unauthorized")
		default:
			return nil, fmt.Errorf("xAI token refresh failed (HTTP %d)", result.StatusCode)
		}
	}
	var token xaiTokenResponse
	if err := decodeNestedBody(result.Body, &token); err != nil || strings.TrimSpace(token.AccessToken) == "" {
		return nil, errors.New("xAI token refresh returned an invalid response")
	}
	return &token, nil
}

type embeddedAPICallResult struct {
	StatusCode int             `json:"status_code"`
	Body       json.RawMessage `json:"body"`
}

func (p *ManagementProxy) callEmbeddedAPICall(ctx context.Context, lease *ManagementLease, body []byte) (*embeddedAPICallResult, error) {
	target := *lease.Target
	target.Path = "/v0/management/api-call"
	target.RawQuery = ""
	target.Fragment = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+lease.Password)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if !isSuccess(resp.StatusCode) {
		return nil, errors.New("embedded CPA API call failed")
	}
	var result embeddedAPICallResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func decodeNestedBody(raw json.RawMessage, target interface{}) error {
	if len(raw) == 0 {
		return errors.New("empty nested body")
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		return json.Unmarshal([]byte(text), target)
	}
	return json.Unmarshal(raw, target)
}

func persistXAIToken(path string, original []byte, token *xaiTokenResponse, now time.Time) error {
	var credential map[string]interface{}
	if err := json.Unmarshal(original, &credential); err != nil {
		return errors.New("parse xAI credential for persistence")
	}
	credential["access_token"] = strings.TrimSpace(token.AccessToken)
	if strings.TrimSpace(token.RefreshToken) != "" {
		credential["refresh_token"] = strings.TrimSpace(token.RefreshToken)
	}
	if strings.TrimSpace(token.IDToken) != "" {
		credential["id_token"] = strings.TrimSpace(token.IDToken)
	}
	if strings.TrimSpace(token.TokenType) != "" {
		credential["token_type"] = strings.TrimSpace(token.TokenType)
	}
	credential["expires_in"] = token.ExpiresIn
	now = now.UTC()
	credential["last_refresh"] = now.Format(time.RFC3339)
	if token.ExpiresIn > 0 {
		credential["expired"] = now.Add(time.Duration(token.ExpiresIn) * time.Second).Format(time.RFC3339)
	} else {
		delete(credential, "expired")
	}
	body, err := json.MarshalIndent(credential, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeFileAtomic(path, body, 0o600)
}

// atomicRename is os.Rename by default; tests may override to inject failures.
var atomicRename = os.Rename

func writeFileAtomic(path string, body []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	temporary, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err = temporary.Write(body); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}

	// Windows cannot rename over an existing file. Move the original aside
	// first, replace, and restore the backup if the replace fails.
	backupPath := path + ".bak"
	haveBackup := false
	if _, statErr := os.Stat(path); statErr == nil {
		_ = os.Remove(backupPath)
		if err = atomicRename(path, backupPath); err != nil {
			return fmt.Errorf("back up old file before replace: %w", err)
		}
		haveBackup = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat old file before replace: %w", statErr)
	}

	if err = atomicRename(temporaryPath, path); err != nil {
		if haveBackup {
			if restoreErr := atomicRename(backupPath, path); restoreErr != nil {
				return fmt.Errorf("replace file: %w (and restoring original failed: %v)", err, restoreErr)
			}
		}
		return fmt.Errorf("replace file: %w", err)
	}

	if haveBackup {
		_ = os.Remove(backupPath)
	}
	return nil
}

func (p *ManagementProxy) xaiAuthEntry(ctx context.Context, lease *ManagementLease, authIndex string) (*xaiAuthListEntry, error) {
	target := *lease.Target
	target.Path = "/v0/management/auth-files"
	target.RawQuery = ""
	target.Fragment = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+lease.Password)
	resp, err := p.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if !isSuccess(resp.StatusCode) {
		return nil, fmt.Errorf("auth files list returned %d", resp.StatusCode)
	}
	var result struct {
		Files []xaiAuthListEntry `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, err
	}
	for index := range result.Files {
		if result.Files[index].AuthIndex == authIndex {
			return &result.Files[index], nil
		}
	}
	return nil, errors.New("xAI auth index was not found")
}
