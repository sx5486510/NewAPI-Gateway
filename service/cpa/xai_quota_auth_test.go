package cpa

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type xaiQuotaTestStore struct {
	authDir string
}

func (s *xaiQuotaTestStore) PersistRuntime() error { return nil }

func (s *xaiQuotaTestStore) Basic() (*CPAConfig, error) {
	return &CPAConfig{AuthDir: s.authDir}, nil
}

type capturedAPICall struct {
	AuthIndex string            `json:"authIndex"`
	URL       string            `json:"url"`
	Header    map[string]string `json:"header"`
}

func TestManagementProxyXAIQuotaUsesCredentialIdentity(t *testing.T) {
	authDir := t.TempDir()
	const authName = "xai-valid.json"
	credential := map[string]interface{}{
		"type":          "xai",
		"access_token":  "valid-access",
		"refresh_token": "valid-refresh",
		"sub":           "subject-1",
		"expired":       time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	credentialBody, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, authName), credentialBody, 0o600); err != nil {
		t.Fatal(err)
	}

	var forwarded capturedAPICall
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"files": []map[string]interface{}{{
					"name": authName, "provider": "xai", "auth_index": "runtime-index",
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/management/api-call":
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err := json.Unmarshal(body, &forwarded); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status_code": http.StatusOK,
				"body":        map[string]interface{}{"config": map[string]interface{}{"creditUsagePercent": 25}},
			})
		default:
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.String())
		}
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := NewManagementProxy(
		&fakeLeaseProvider{target: target, password: "runtime-secret"},
		&xaiQuotaTestStore{authDir: authDir},
		nil,
	)

	payload := `{"authIndex":"runtime-index","method":"GET","url":"https://cli-chat-proxy.grok.com/v1/billing?format=credits","header":{"Authorization":"Bearer $TOKEN$","x-xai-token-auth":"xai-grok-cli"}}`
	req := httptest.NewRequest(http.MethodPost, "/v0/management/api-call", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := forwarded.Header["Authorization"]; got != "Bearer valid-access" {
		t.Fatalf("Authorization = %q, want valid credential token", got)
	}
	if got := forwarded.Header["x-userid"]; got != "subject-1" {
		t.Fatalf("x-userid = %q, want subject-1", got)
	}
}
