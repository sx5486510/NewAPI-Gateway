package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// UpstreamClient wraps HTTP calls to an upstream NewAPI instance
type UpstreamClient struct {
	BaseURL     string
	AccessToken string
	UserID      int
	HTTPClient  *http.Client
}

func NewUpstreamClient(baseURL string, accessToken string, userID int) *UpstreamClient {
	return &UpstreamClient{
		BaseURL:     baseURL,
		AccessToken: accessToken,
		UserID:      userID,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// UpstreamResponse is the standard NewAPI response wrapper
type UpstreamResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// UpstreamPricing mirrors the upstream Pricing structure
type UpstreamPricing struct {
	ModelName       string   `json:"model_name"`
	QuotaType       int      `json:"quota_type"`
	ModelRatio      float64  `json:"model_ratio"`
	ModelPrice      float64  `json:"model_price"`
	CompletionRatio float64  `json:"completion_ratio"`
	EnableGroups    []string `json:"enable_groups"`
}

// UpstreamToken mirrors the upstream Token structure
type UpstreamToken struct {
	Id                 int    `json:"id"`
	Key                string `json:"key"`
	Name               string `json:"name"`
	Status             int    `json:"status"`
	Group              string `json:"group"`
	RemainQuota        int64  `json:"remain_quota"`
	UnlimitedQuota     bool   `json:"unlimited_quota"`
	UsedQuota          int64  `json:"used_quota"`
	ModelLimitsEnabled bool   `json:"model_limits_enabled"`
	ModelLimits        string `json:"model_limits"`
}

// UpstreamUserSelf mirrors partial user/self response
type UpstreamUserSelf struct {
	Id      int   `json:"id"`
	Balance int64 `json:"quota"`
	Status  int   `json:"status"`
}

// CheckinResponse for the checkin endpoint
type CheckinResponse struct {
	QuotaAwarded int64 `json:"quota_awarded"`
}

func (c *UpstreamClient) doRequest(method string, path string) ([]byte, error) {
	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("New-Api-User", strconv.Itoa(c.UserID))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// GetPricing fetches /api/pricing from the upstream
func (c *UpstreamClient) GetPricing() ([]UpstreamPricing, error) {
	body, err := c.doRequest("GET", "/api/pricing")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []UpstreamPricing `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetTokens fetches /api/token/ from the upstream
func (c *UpstreamClient) GetTokens(page int, pageSize int) ([]UpstreamToken, error) {
	path := fmt.Sprintf("/api/token/?p=%d&page_size=%d", page, pageSize)
	body, err := c.doRequest("GET", path)
	if err != nil {
		return nil, err
	}
	var resp UpstreamResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	var tokens []UpstreamToken
	if err := json.Unmarshal(resp.Data, &tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

// GetUserSelf fetches /api/user/self from the upstream
func (c *UpstreamClient) GetUserSelf() (*UpstreamUserSelf, error) {
	body, err := c.doRequest("GET", "/api/user/self")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Success bool             `json:"success"`
		Data    UpstreamUserSelf `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Checkin performs /api/user/checkin on the upstream
func (c *UpstreamClient) Checkin() (*CheckinResponse, error) {
	url := c.BaseURL + "/api/user/checkin"
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("New-Api-User", strconv.Itoa(c.UserID))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Data    CheckinResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, fmt.Errorf("checkin failed: %s", result.Message)
	}
	return &result.Data, nil
}
