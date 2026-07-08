package model

import (
	"NewAPI-Gateway/common"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupModelRouteTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}

	DB = db
	if err := DB.AutoMigrate(&Provider{}, &ProviderToken{}, &ModelRoute{}, &ModelPricing{}, &UsageLog{}, &Option{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{
		routingUsageWindowHoursOptionKey: "24",
		routingBaseWeightFactorOptionKey: "1",
		routingValueScoreFactorOptionKey: "0",
		routingHealthEnabledOptionKey:    "false",
	}
	common.OptionMapRWMutex.Unlock()
	common.GlobalRouteCooldown = common.NewRouteCooldownManager(func() common.RouteCooldownConfig {
		return common.RouteCooldownConfig{Enabled: false}
	})
}

func insertRouteCandidate(t *testing.T, providerID int, tokenID int, priority int, weight int) {
	t.Helper()

	provider := Provider{
		Id:       providerID,
		Name:     "provider",
		BaseURL:  "https://example.com",
		Status:   common.UserStatusEnabled,
		Priority: priority,
		Weight:   weight,
	}
	if err := DB.Create(&provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}

	token := ProviderToken{
		Id:         tokenID,
		ProviderId: providerID,
		Name:       "token",
		GroupName:  "default",
		Status:     common.UserStatusEnabled,
		Priority:   priority,
		Weight:     weight,
	}
	if err := DB.Create(&token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	route := ModelRoute{
		ModelName:       "gpt-test",
		ProviderId:      providerID,
		ProviderTokenId: tokenID,
		Enabled:         true,
		Priority:        priority,
		Weight:          weight,
	}
	if err := DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
}

func TestBuildRouteAttemptsIgnoresPriorityTiers(t *testing.T) {
	setupModelRouteTestDB(t)

	insertRouteCandidate(t, 1, 101, 100, 10)
	insertRouteCandidate(t, 2, 102, 0, 10)

	plan, err := BuildRouteAttemptsByPriority("gpt-test", "")
	if err != nil {
		t.Fatalf("build route attempts: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected a single retry group when priority is ignored, got %d", len(plan))
	}
	if len(plan[0]) != 2 {
		t.Fatalf("expected both routes in the same retry group, got %d", len(plan[0]))
	}
}

func TestComputeRouteContributionIgnoresManualWeight(t *testing.T) {
	lowWeight := computeRouteContribution(-10, 1, 1, 1, 0)
	highWeight := computeRouteContribution(100, 1, 1, 1, 0)

	if lowWeight != highWeight {
		t.Fatalf("expected manual weight to be ignored, got low=%f high=%f", lowWeight, highWeight)
	}
	if lowWeight <= 0 {
		t.Fatalf("expected positive baseline contribution, got %f", lowWeight)
	}
}

func TestUpdateModelRouteFieldsNormalizesClientRestrictions(t *testing.T) {
	setupModelRouteTestDB(t)

	route := ModelRoute{
		ModelName:       "gpt-test",
		ProviderId:      1,
		ProviderTokenId: 101,
		Enabled:         true,
		AllowCodex:      true,
	}
	if err := DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}

	if err := UpdateModelRouteFields(route.Id, map[string]interface{}{
		"allow_codex":   true,
		"allow_cc":      true,
		"block_clients": true,
	}); err != nil {
		t.Fatalf("update route: %v", err)
	}

	var stored ModelRoute
	if err := DB.First(&stored, route.Id).Error; err != nil {
		t.Fatalf("load route: %v", err)
	}
	if stored.AllowCodex {
		t.Fatal("expected allow_codex to be cleared when block_clients is enabled")
	}
	if stored.AllowCC {
		t.Fatal("expected allow_cc to be cleared when block_clients is enabled")
	}
	if !stored.BlockClients {
		t.Fatal("expected block_clients to be saved")
	}
}

func TestBatchUpdateModelRoutesSavesDisabledRouteAndOverviewReadsIt(t *testing.T) {
	setupModelRouteTestDB(t)

	provider := Provider{
		Id:      1,
		Name:    "provider",
		BaseURL: "https://example.com",
		Status:  common.UserStatusEnabled,
	}
	if err := DB.Create(&provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}

	token := ProviderToken{
		Id:         101,
		ProviderId: 1,
		Name:       "token",
		GroupName:  "default",
		Status:     common.UserStatusEnabled,
	}
	if err := DB.Create(&token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	route := ModelRoute{
		ModelName:       "gpt-test",
		ProviderId:      1,
		ProviderTokenId: 101,
		Enabled:         true,
	}
	if err := DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}

	enabled := false
	if err := BatchUpdateModelRoutes([]ModelRoutePatch{{
		Id:      route.Id,
		Enabled: &enabled,
	}}); err != nil {
		t.Fatalf("batch update route: %v", err)
	}

	var stored ModelRoute
	if err := DB.First(&stored, route.Id).Error; err != nil {
		t.Fatalf("load route: %v", err)
	}
	if stored.Enabled {
		t.Fatal("expected batch update to persist enabled=false")
	}

	overview, err := GetModelRouteOverview("", 0, false)
	if err != nil {
		t.Fatalf("get route overview: %v", err)
	}
	if len(overview) != 1 {
		t.Fatalf("expected one overview route, got %d", len(overview))
	}
	if overview[0].Enabled {
		t.Fatal("expected overview to read enabled=false")
	}
}

func TestRebuildRoutesForProviderPreservesDisabledRoute(t *testing.T) {
	setupModelRouteTestDB(t)

	route := ModelRoute{
		ModelName:       "gpt-test",
		ProviderId:      1,
		ProviderTokenId: 101,
		Enabled:         false,
	}
	if err := DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	if err := UpdateModelRouteFields(route.Id, map[string]interface{}{"enabled": false}); err != nil {
		t.Fatalf("disable route: %v", err)
	}

	if err := RebuildRoutesForProvider(1, []ModelRoute{
		{
			ModelName:       "gpt-test",
			ProviderId:      1,
			ProviderTokenId: 101,
			Enabled:         true,
		},
	}); err != nil {
		t.Fatalf("rebuild routes: %v", err)
	}

	var stored ModelRoute
	if err := DB.First(&stored, "provider_id = ? AND provider_token_id = ? AND model_name = ?", 1, 101, "gpt-test").Error; err != nil {
		t.Fatalf("load rebuilt route: %v", err)
	}
	if stored.Enabled {
		t.Fatal("expected disabled route to remain disabled after rebuild")
	}
}
