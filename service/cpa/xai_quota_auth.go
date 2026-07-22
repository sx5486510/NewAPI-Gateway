package cpa

import (
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

func isXAIQuotaURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" &&
		strings.EqualFold(parsed.Hostname(), "cli-chat-proxy.grok.com") &&
		parsed.Path == "/v1/billing"
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
	if !isXAIQuotaURL(rawJSONString(payload, "url", "URL")) {
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
	credentialBody, err := os.ReadFile(authPath)
	if err != nil {
		return nil, fmt.Errorf("read xAI credential: %w", err)
	}
	var credential xaiCredential
	if err := json.Unmarshal(credentialBody, &credential); err != nil {
		return nil, errors.New("parse xAI credential")
	}
	accessToken := strings.TrimSpace(credential.AccessToken)
	if accessToken == "" {
		return nil, errors.New("xAI credential access token is missing")
	}
	if expired, err := time.Parse(time.RFC3339, strings.TrimSpace(credential.Expired)); err == nil && !expired.After(time.Now().Add(xaiRefreshSkew)) {
		return nil, errors.New("xAI credential access token is expired")
	}

	setHeaderValue(headers, "Authorization", strings.ReplaceAll(headerValue(headers, "Authorization"), "$TOKEN$", accessToken))
	if headerValue(headers, "x-userid") == "" && strings.TrimSpace(credential.Subject) != "" {
		setHeaderValue(headers, "x-userid", strings.TrimSpace(credential.Subject))
	}
	payload["header"], err = json.Marshal(headers)
	if err != nil {
		return nil, err
	}
	return json.Marshal(payload)
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
