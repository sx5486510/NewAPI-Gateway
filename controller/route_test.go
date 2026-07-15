package controller

import (
	"NewAPI-Gateway/common"
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

func setupRouteControllerTestDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	model.DB = db
	if err := model.DB.AutoMigrate(&model.Provider{}, &model.ProviderToken{}, &model.ModelRoute{}, &model.ModelPricing{}, &model.UsageLog{}, &model.Option{}, &model.SystemPrompt{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{
		"RoutingUsageWindowHours":        "24",
		"RoutingBaseWeightFactor":        "1",
		"RoutingValueScoreFactor":        "0",
		"RoutingHealthAdjustmentEnabled": "false",
	}
	common.OptionMapRWMutex.Unlock()
	common.GlobalRouteCooldown = common.NewRouteCooldownManager(func() common.RouteCooldownConfig {
		return common.RouteCooldownConfig{Enabled: false}
	})
}

func TestRouteSystemPromptNumericSelectionAndNullClear(t *testing.T) {
	setupRouteControllerTestDB(t)
	prompt := model.SystemPrompt{Name: "preset", ModelName: "gpt-4", Content: "content"}
	if err := model.DB.Create(&prompt).Error; err != nil {
		t.Fatal(err)
	}
	route := model.ModelRoute{ModelName: "gpt-4", ProviderId: 1, ProviderTokenId: 1, Enabled: true}
	if err := model.DB.Create(&route).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/api/route/:id", UpdateRoute)

	for _, body := range []string{
		`{"system_prompt_id":` + strconv.Itoa(prompt.Id) + `}`,
		`{"system_prompt_id":null}`,
	} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/route/"+strconv.Itoa(route.Id), bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		var payload struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if !payload.Success {
			t.Fatalf("body %s failed: %s", body, payload.Message)
		}
	}
	var stored model.ModelRoute
	if err := model.DB.First(&stored, route.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SystemPromptId != nil {
		t.Fatalf("expected cleared binding, got %v", *stored.SystemPromptId)
	}
}

func TestRouteSystemPromptRejectsCrossModelSelection(t *testing.T) {
	setupRouteControllerTestDB(t)
	prompt := model.SystemPrompt{Name: "preset", ModelName: "gpt-other", Content: "content"}
	if err := model.DB.Create(&prompt).Error; err != nil {
		t.Fatal(err)
	}
	route := model.ModelRoute{ModelName: "gpt-4", ProviderId: 1, ProviderTokenId: 1, Enabled: true}
	if err := model.DB.Create(&route).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/api/route/:id", UpdateRoute)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/route/"+strconv.Itoa(route.Id), bytes.NewBufferString(`{"system_prompt_id":`+strconv.Itoa(prompt.Id)+`}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Success || payload.Message != "system prompt model does not match route model" {
		t.Fatalf("expected stable mismatch error, got %#v", payload)
	}
}

func TestBatchUpdateRoutesSavesDisabledRouteAndOverviewReturnsIt(t *testing.T) {
	setupRouteControllerTestDB(t)

	provider := model.Provider{Name: "provider", BaseURL: "https://example.test", Status: common.UserStatusEnabled}
	if err := provider.Insert(); err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	token := model.ProviderToken{
		ProviderId: provider.Id,
		Name:       "token",
		GroupName:  "default",
		Status:     common.UserStatusEnabled,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("insert token: %v", err)
	}
	route := model.ModelRoute{
		ModelName:       "gpt-test",
		ProviderId:      provider.Id,
		ProviderTokenId: token.Id,
		Enabled:         true,
	}
	if err := model.DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}

	router := gin.New()
	router.POST("/api/route/batch-update", BatchUpdateRoutes)
	router.GET("/api/route/overview", GetModelRouteOverview)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/route/batch-update",
		bytes.NewBufferString(`{"items":[{"id":`+strconv.Itoa(route.Id)+`,"enabled":false}]}`),
	)
	router.ServeHTTP(recorder, req)

	var updatePayload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &updatePayload); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if !updatePayload.Success {
		t.Fatalf("expected update success, got message %q", updatePayload.Message)
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/route/overview", nil)
	router.ServeHTTP(recorder, req)

	var overviewPayload struct {
		Success bool                           `json:"success"`
		Data    []model.ModelRouteOverviewItem `json:"data"`
		Message string                         `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &overviewPayload); err != nil {
		t.Fatalf("decode overview response: %v", err)
	}
	if !overviewPayload.Success {
		t.Fatalf("expected overview success, got message %q", overviewPayload.Message)
	}
	if len(overviewPayload.Data) != 1 {
		t.Fatalf("expected one overview route, got %d", len(overviewPayload.Data))
	}
	if overviewPayload.Data[0].Enabled {
		t.Fatal("expected overview to return enabled=false")
	}
}
