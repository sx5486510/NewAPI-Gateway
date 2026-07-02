package controller

import (
	"NewAPI-Gateway/model"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupProviderControllerTestDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	model.DB = db
	if err := model.DB.AutoMigrate(&model.Provider{}, &model.ProviderToken{}, &model.ModelPricing{}, &model.ModelRoute{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
}

func TestUpdateProviderTokenPartialClientRestrictionPreservesTokenFields(t *testing.T) {
	setupProviderControllerTestDB(t)

	provider := model.Provider{Name: "provider", BaseURL: "https://example.test", Status: 1}
	if err := provider.Insert(); err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	token := model.ProviderToken{
		ProviderId:      provider.Id,
		UpstreamTokenId: 99,
		SkKey:           "sk-test",
		Name:            "primary",
		GroupName:       "default",
		Status:          1,
		Priority:        7,
		Weight:          11,
		RemainQuota:     123,
		UnlimitedQuota:  true,
		ModelLimits:     "gpt-4o",
		AllowCodex:      true,
		AllowCC:         true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("insert token: %v", err)
	}

	router := gin.New()
	router.PUT("/api/provider/token/:token_id", UpdateProviderToken)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/provider/token/"+strconv.Itoa(token.Id),
		bytes.NewBufferString(`{"allow_codex":false,"allow_cc":false,"block_clients":true}`),
	)
	router.ServeHTTP(recorder, req)

	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success {
		t.Fatalf("expected update success, got message %q", payload.Message)
	}

	var stored model.ProviderToken
	if err := model.DB.First(&stored, token.Id).Error; err != nil {
		t.Fatalf("load token: %v", err)
	}
	if stored.Name != token.Name || stored.GroupName != token.GroupName || stored.Weight != token.Weight || stored.RemainQuota != token.RemainQuota || !stored.UnlimitedQuota {
		t.Fatalf("partial update overwrote token fields: %+v", stored)
	}
	if stored.AllowCodex || stored.AllowCC || !stored.BlockClients {
		t.Fatalf("client restrictions were not updated correctly: %+v", stored)
	}
}

func TestUpdateKeyOnlyProviderUpdatesForwardingTokenKey(t *testing.T) {
	setupProviderControllerTestDB(t)

	provider := model.Provider{
		Name:         "key-only",
		BaseURL:      "https://example.test",
		ApiKey:       "old-key",
		ProviderType: model.ProviderTypeKeyOnly,
		Status:       1,
		Priority:     3,
		Weight:       5,
	}
	if err := provider.Insert(); err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	token := model.ProviderToken{
		ProviderId:      provider.Id,
		UpstreamTokenId: 0,
		SkKey:           "old-key",
		Name:            provider.Name,
		GroupName:       "default",
		Status:          1,
		Priority:        provider.Priority,
		Weight:          provider.Weight,
		UnlimitedQuota:  true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("insert token: %v", err)
	}

	router := gin.New()
	router.PUT("/api/provider/", UpdateProvider)

	body, err := json.Marshal(model.Provider{
		Id:           provider.Id,
		Name:         provider.Name,
		BaseURL:      provider.BaseURL,
		ApiKey:       "new-key",
		ProviderType: model.ProviderTypeKeyOnly,
		Status:       provider.Status,
		Priority:     provider.Priority,
		Weight:       provider.Weight,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/provider/", bytes.NewBuffer(body))
	router.ServeHTTP(recorder, req)

	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success {
		t.Fatalf("expected update success, got message %q", payload.Message)
	}

	var storedProvider model.Provider
	if err := model.DB.First(&storedProvider, provider.Id).Error; err != nil {
		t.Fatalf("load provider: %v", err)
	}
	if storedProvider.ApiKey != "new-key" {
		t.Fatalf("provider api key was not updated, got %q", storedProvider.ApiKey)
	}

	var storedToken model.ProviderToken
	if err := model.DB.First(&storedToken, token.Id).Error; err != nil {
		t.Fatalf("load token: %v", err)
	}
	if storedToken.SkKey != "new-key" {
		t.Fatalf("forwarding token key was not updated, got %q", storedToken.SkKey)
	}
}
