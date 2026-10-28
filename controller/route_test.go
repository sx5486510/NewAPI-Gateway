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
	"time"

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

	updateBinding := func(body string) {
		t.Helper()
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

	updateBinding(`{"system_prompt_id":` + strconv.Itoa(prompt.Id) + `}`)
	var stored model.ModelRoute
	if err := model.DB.First(&stored, route.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SystemPromptId == nil || *stored.SystemPromptId != prompt.Id {
		t.Fatalf("expected binding to prompt %d, got %+v", prompt.Id, stored.SystemPromptId)
	}

	updateBinding(`{"system_prompt_id":null}`)
	stored = model.ModelRoute{}
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

func TestResetRouteHealthClearsCooldownAndHealthStats(t *testing.T) {
	setupRouteControllerTestDB(t)

	common.GlobalRouteCooldown = common.NewRouteCooldownManager(func() common.RouteCooldownConfig {
		return common.RouteCooldownConfig{
			Enabled:                true,
			BaseSeconds:            60,
			Multiplier:             2,
			MaxSeconds:             600,
			MinConsecutiveFailures: 1,
			UnsupportedModelHours:  24,
			TokenBaseSeconds:       600,
			TokenMaxSeconds:        7200,
		}
	})

	provider := model.Provider{Name: "provider", BaseURL: "https://example.test", Status: common.UserStatusEnabled}
	if err := provider.Insert(); err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	token := model.ProviderToken{ProviderId: provider.Id, Name: "token", GroupName: "default", Status: common.UserStatusEnabled}
	if err := token.Insert(); err != nil {
		t.Fatalf("insert token: %v", err)
	}
	route := model.ModelRoute{ModelName: "gpt-reset", ProviderId: provider.Id, ProviderTokenId: token.Id, Enabled: true}
	if err := model.DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}

	// 本小时内的失败日志 + 路由/令牌冷却
	hourStart := time.Now().Truncate(time.Hour).Unix()
	if err := model.DB.Create(&model.UsageLog{
		ProviderId:      provider.Id,
		ProviderTokenId: token.Id,
		ModelName:       route.ModelName,
		Status:          0,
		ErrorMessage:    "upstream 500",
		CreatedAt:       hourStart,
	}).Error; err != nil {
		t.Fatalf("create usage log: %v", err)
	}
	common.GlobalRouteCooldown.RecordRouteFailure(token.Id, route.ModelName)
	common.GlobalRouteCooldown.RecordTokenFailure(token.Id)

	router := gin.New()
	router.POST("/api/route/:id/reset", ResetRouteHealth)
	router.GET("/api/route/overview", GetModelRouteOverview)

	loadOverview := func() model.ModelRouteOverviewItem {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/route/overview", nil)
		router.ServeHTTP(recorder, req)
		var payload struct {
			Success bool                           `json:"success"`
			Data    []model.ModelRouteOverviewItem `json:"data"`
			Message string                         `json:"message"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode overview: %v", err)
		}
		if !payload.Success || len(payload.Data) != 1 {
			t.Fatalf("unexpected overview payload: %+v", payload)
		}
		return payload.Data[0]
	}

	before := loadOverview()
	if before.HealthErrorCount != 1 || before.HealthValue != -1 {
		t.Fatalf("expected one failure before reset, got errors=%d health=%d", before.HealthErrorCount, before.HealthValue)
	}
	if !before.CooldownInCooldown {
		t.Fatalf("expected route in cooldown before reset")
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/route/"+strconv.Itoa(route.Id)+"/reset", nil)
	router.ServeHTTP(recorder, req)
	var resetPayload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			HealthResetAt        int64 `json:"health_reset_at"`
			TokenCooldownCleared bool  `json:"token_cooldown_cleared"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resetPayload); err != nil {
		t.Fatalf("decode reset: %v", err)
	}
	if !resetPayload.Success || resetPayload.Data.HealthResetAt <= 0 || !resetPayload.Data.TokenCooldownCleared {
		t.Fatalf("unexpected reset payload: %+v", resetPayload)
	}

	after := loadOverview()
	if after.HealthErrorCount != 0 || after.HealthSuccessCount != 0 || after.HealthValue != 0 {
		t.Fatalf("expected cleared health stats after reset, got errors=%d success=%d health=%d", after.HealthErrorCount, after.HealthSuccessCount, after.HealthValue)
	}
	if after.CooldownInCooldown || after.CooldownHalfOpen || after.CooldownReason != "" {
		t.Fatalf("expected cooldown cleared after reset, got %+v", after)
	}

	// 重置之后的失败仍然要重新计入健康值
	if err := model.DB.Create(&model.UsageLog{
		ProviderId:      provider.Id,
		ProviderTokenId: token.Id,
		ModelName:       route.ModelName,
		Status:          0,
		ErrorMessage:    "upstream 500 again",
		CreatedAt:       time.Now().Add(time.Hour).Unix(),
	}).Error; err != nil {
		t.Fatalf("create post-reset usage log: %v", err)
	}
	post := loadOverview()
	if post.HealthErrorCount != 1 || post.HealthValue != -1 {
		t.Fatalf("expected post-reset failures to count again, got errors=%d health=%d", post.HealthErrorCount, post.HealthValue)
	}
}
