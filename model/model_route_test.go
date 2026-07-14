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

func TestBuildRouteAttemptsFiltersRoutesAboveDefaultPriceGuardLimit(t *testing.T) {
	setupModelRouteTestDB(t)

	insertRouteCandidate(t, 1, 101, 0, 10)
	insertRouteCandidate(t, 2, 102, 0, 10)
	if err := DB.Create(&ModelPricing{ProviderId: 1, ModelName: "gpt-test", ModelRatio: 10, CompletionRatio: 1}).Error; err != nil {
		t.Fatalf("create affordable pricing: %v", err)
	}
	if err := DB.Create(&ModelPricing{ProviderId: 2, ModelName: "gpt-test", ModelRatio: 40, CompletionRatio: 1}).Error; err != nil {
		t.Fatalf("create expensive pricing: %v", err)
	}

	plan, err := BuildRouteAttemptsByPriority("gpt-test", "")
	if err != nil {
		t.Fatalf("build route attempts: %v", err)
	}
	if len(plan) != 1 || len(plan[0]) != 1 {
		t.Fatalf("expected only the affordable route to participate, got %#v", plan)
	}
	if plan[0][0].Route.ProviderId != 1 {
		t.Fatalf("expected provider 1 to remain, got provider %d", plan[0][0].Route.ProviderId)
	}
}

func TestBuildRouteAttemptsKeepsExpensiveRoutesWhenPriceGuardDisabled(t *testing.T) {
	setupModelRouteTestDB(t)
	common.OptionMapRWMutex.Lock()
	common.OptionMap[routingPriceGuardEnabledOptionKey] = "false"
	common.OptionMapRWMutex.Unlock()

	insertRouteCandidate(t, 1, 101, 0, 10)
	if err := DB.Create(&ModelPricing{ProviderId: 1, ModelName: "gpt-test", ModelRatio: 40, CompletionRatio: 1}).Error; err != nil {
		t.Fatalf("create expensive pricing: %v", err)
	}

	plan, err := BuildRouteAttemptsByPriority("gpt-test", "")
	if err != nil {
		t.Fatalf("build route attempts: %v", err)
	}
	if len(plan) != 1 || len(plan[0]) != 1 {
		t.Fatalf("expected expensive route to participate when price guard is disabled, got %#v", plan)
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

func TestRebuildRoutesForProviderPreservesExistingRouteIDAndSettings(t *testing.T) {
	setupModelRouteTestDB(t)

	route := ModelRoute{
		ModelName:       "gpt-test",
		ProviderId:      1,
		ProviderTokenId: 101,
		Enabled:         false,
		Priority:        1,
		Weight:          2,
		AllowCodex:      true,
		AllowCC:         true,
	}
	if err := DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	otherRoute := ModelRoute{
		ModelName:       "other-model",
		ProviderId:      2,
		ProviderTokenId: 201,
		Enabled:         true,
	}
	if err := DB.Create(&otherRoute).Error; err != nil {
		t.Fatalf("create other provider route: %v", err)
	}
	if err := UpdateModelRouteFields(route.Id, map[string]interface{}{"enabled": false}); err != nil {
		t.Fatalf("disable route: %v", err)
	}

	if err := RebuildRoutesForProvider(1, []ModelRoute{{
		ModelName:       "gpt-test",
		ProviderId:      1,
		ProviderTokenId: 101,
		Enabled:         true,
		Priority:        9,
		Weight:          10,
	}}); err != nil {
		t.Fatalf("rebuild routes: %v", err)
	}

	var stored ModelRoute
	if err := DB.First(&stored, "provider_id = ? AND provider_token_id = ? AND model_name = ?", 1, 101, "gpt-test").Error; err != nil {
		t.Fatalf("load rebuilt route: %v", err)
	}
	if stored.Id != route.Id {
		t.Fatalf("expected route ID %d to be preserved, got %d", route.Id, stored.Id)
	}
	if stored.Enabled {
		t.Fatal("expected enabled setting to be preserved")
	}
	if !stored.AllowCodex || !stored.AllowCC || stored.BlockClients {
		t.Fatalf("expected client restrictions to be preserved, got codex=%v cc=%v block=%v", stored.AllowCodex, stored.AllowCC, stored.BlockClients)
	}
	if stored.Priority != 9 || stored.Weight != 10 {
		t.Fatalf("expected generated priority and weight to refresh, got priority=%d weight=%d", stored.Priority, stored.Weight)
	}
}

func TestRebuildRoutesForProviderAddsAndRemovesRoutes(t *testing.T) {
	setupModelRouteTestDB(t)

	existing := []ModelRoute{
		{ModelName: "keep", ProviderId: 1, ProviderTokenId: 101, Enabled: true},
		{ModelName: "remove", ProviderId: 1, ProviderTokenId: 102, Enabled: true},
	}
	if err := DB.Create(&existing).Error; err != nil {
		t.Fatalf("create existing routes: %v", err)
	}

	if err := RebuildRoutesForProvider(1, []ModelRoute{
		{ModelName: "keep", ProviderId: 1, ProviderTokenId: 101, Enabled: true},
		{ModelName: "add", ProviderId: 1, ProviderTokenId: 103, Enabled: true},
	}); err != nil {
		t.Fatalf("rebuild routes: %v", err)
	}

	var routes []ModelRoute
	if err := DB.Where("provider_id = ?", 1).Order("model_name").Find(&routes).Error; err != nil {
		t.Fatalf("load routes: %v", err)
	}
	if len(routes) != 2 || routes[0].ModelName != "add" || routes[1].ModelName != "keep" {
		t.Fatalf("expected add and keep routes, got %#v", routes)
	}
}

func TestUpdateModelRouteFieldsRejectsMissingRoute(t *testing.T) {
	setupModelRouteTestDB(t)

	if err := UpdateModelRouteFields(999, map[string]interface{}{"enabled": false}); err == nil {
		t.Fatal("expected missing route update to fail")
	}
}

func TestBatchUpdateModelRoutesRollsBackWhenRouteMissing(t *testing.T) {
	setupModelRouteTestDB(t)

	route := ModelRoute{
		ModelName:       "gpt-test",
		ProviderId:      1,
		ProviderTokenId: 101,
		Enabled:         true,
	}
	if err := DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}

	disabled := false
	err := BatchUpdateModelRoutes([]ModelRoutePatch{
		{Id: route.Id, Enabled: &disabled},
		{Id: 999, Enabled: &disabled},
	})
	if err == nil {
		t.Fatal("expected batch with missing route to fail")
	}

	var stored ModelRoute
	if err := DB.First(&stored, route.Id).Error; err != nil {
		t.Fatalf("load route: %v", err)
	}
	if !stored.Enabled {
		t.Fatal("expected valid route update to be rolled back")
	}
}
