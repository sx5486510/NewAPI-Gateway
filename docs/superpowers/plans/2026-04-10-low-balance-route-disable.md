# Low Balance Route Disable Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Disable rebuilt provider routes when provider balance is below 1 USD, auto-restore them after balance recovery, and keep manually disabled routes disabled.

**Architecture:** Keep the balance threshold decision in `service.RebuildProviderRoutes(...)`, because that is already the rebuild-time business-rule owner. Add one internal persistence flag on `model_routes` so the model layer can distinguish an automatic low-balance disable from a human manual disable during later rebuilds and route edits.

**Tech Stack:** Go, GORM, SQLite-backed unit tests, existing `model` and `service` packages, Markdown docs

---

## File Map

- `model/model_route.go`
  Responsibility: add the internal auto-disable marker, expose a balance parser helper for other packages, preserve manual disables during rebuild persistence, and clear the automatic marker when admins patch route `enabled`.
- `model/model_route_rebuild_test.go`
  Responsibility: DB-backed unit tests for rebuild carry-forward rules and manual update semantics.
- `service/sync.go`
  Responsibility: read provider balance during route rebuild and stamp generated routes with `Enabled` plus `AutoDisabledByBalance`.
- `service/sync_rebuild_routes_test.go`
  Responsibility: DB-backed service tests for the `$1.00` threshold, auto recovery, and manual-disable persistence.
- `README.md`
  Responsibility: document the user-visible low-balance rebuild behavior for sync and manual rebuild operations.
- `docs/ARCHITECTURE.md`
  Responsibility: document the internal rebuild rule and the new automatic disable marker.

### Task 1: Persist Automatic vs Manual Route Disable State

**Files:**
- Modify: `model/model_route.go`
- Create: `model/model_route_rebuild_test.go`

- [ ] **Step 1: Write the failing model-layer tests**

Create `model/model_route_rebuild_test.go` with this content:

```go
package model

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupModelRouteTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "model-route-test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	oldDB := DB
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	if err := DB.AutoMigrate(&ModelRoute{}); err != nil {
		t.Fatalf("auto migrate model_routes: %v", err)
	}
}

func fetchModelRoute(t *testing.T, modelName string, tokenID int) ModelRoute {
	t.Helper()

	var route ModelRoute
	if err := DB.Where("model_name = ? AND provider_token_id = ?", modelName, tokenID).First(&route).Error; err != nil {
		t.Fatalf("fetch route %s/%d: %v", modelName, tokenID, err)
	}
	return route
}

func boolPtr(v bool) *bool {
	return &v
}

func TestRebuildRoutesForProvider_PreservesManualDisableAndAllowsAutoRecovery(t *testing.T) {
	setupModelRouteTestDB(t)

	existing := []ModelRoute{
		{
			ModelName:             "gpt-4o",
			ProviderTokenId:       101,
			ProviderId:            1,
			Enabled:               false,
			AutoDisabledByBalance: false,
			Priority:              7,
			Weight:                11,
		},
		{
			ModelName:             "gpt-4.1-mini",
			ProviderTokenId:       102,
			ProviderId:            1,
			Enabled:               false,
			AutoDisabledByBalance: true,
			Priority:              5,
			Weight:                9,
		},
	}
	if err := DB.Create(&existing).Error; err != nil {
		t.Fatalf("seed existing routes: %v", err)
	}

	rebuilt := []ModelRoute{
		{
			ModelName:             "gpt-4o",
			ProviderTokenId:       101,
			ProviderId:            1,
			Enabled:               true,
			AutoDisabledByBalance: false,
			Priority:              1,
			Weight:                1,
		},
		{
			ModelName:             "gpt-4.1-mini",
			ProviderTokenId:       102,
			ProviderId:            1,
			Enabled:               true,
			AutoDisabledByBalance: false,
			Priority:              1,
			Weight:                1,
		},
	}
	if err := RebuildRoutesForProvider(1, rebuilt); err != nil {
		t.Fatalf("rebuild routes: %v", err)
	}

	manual := fetchModelRoute(t, "gpt-4o", 101)
	if manual.Enabled {
		t.Fatalf("expected manual disable to persist")
	}
	if manual.AutoDisabledByBalance {
		t.Fatalf("expected manual disable to keep auto flag false")
	}
	if manual.Priority != 7 || manual.Weight != 11 {
		t.Fatalf("expected manual route to preserve priority/weight, got priority=%d weight=%d", manual.Priority, manual.Weight)
	}

	recovered := fetchModelRoute(t, "gpt-4.1-mini", 102)
	if !recovered.Enabled {
		t.Fatalf("expected auto-disabled route to recover when rebuilt as enabled")
	}
	if recovered.AutoDisabledByBalance {
		t.Fatalf("expected recovered route to clear auto flag")
	}
	if recovered.Priority != 5 || recovered.Weight != 9 {
		t.Fatalf("expected recovered route to preserve priority/weight, got priority=%d weight=%d", recovered.Priority, recovered.Weight)
	}
}

func TestUpdateModelRouteFields_ManualEditClearsAutoDisabledByBalance(t *testing.T) {
	setupModelRouteTestDB(t)

	route := ModelRoute{
		ModelName:             "gpt-4o",
		ProviderTokenId:       201,
		ProviderId:            1,
		Enabled:               false,
		AutoDisabledByBalance: true,
		Priority:              1,
		Weight:                1,
	}
	if err := DB.Create(&route).Error; err != nil {
		t.Fatalf("seed route: %v", err)
	}

	if err := UpdateModelRouteFields(route.Id, map[string]interface{}{"enabled": false}); err != nil {
		t.Fatalf("manual update route: %v", err)
	}

	updated := fetchModelRoute(t, "gpt-4o", 201)
	if updated.AutoDisabledByBalance {
		t.Fatalf("expected manual route update to clear auto flag")
	}
	if updated.Enabled {
		t.Fatalf("expected route to remain disabled after manual disable")
	}
}

func TestBatchUpdateModelRoutes_ManualEditClearsAutoDisabledByBalance(t *testing.T) {
	setupModelRouteTestDB(t)

	route := ModelRoute{
		ModelName:             "gpt-4.1",
		ProviderTokenId:       301,
		ProviderId:            1,
		Enabled:               false,
		AutoDisabledByBalance: true,
		Priority:              1,
		Weight:                1,
	}
	if err := DB.Create(&route).Error; err != nil {
		t.Fatalf("seed route: %v", err)
	}

	if err := BatchUpdateModelRoutes([]ModelRoutePatch{
		{Id: route.Id, Enabled: boolPtr(true)},
	}); err != nil {
		t.Fatalf("batch update route: %v", err)
	}

	updated := fetchModelRoute(t, "gpt-4.1", 301)
	if updated.AutoDisabledByBalance {
		t.Fatalf("expected batch manual update to clear auto flag")
	}
	if !updated.Enabled {
		t.Fatalf("expected batch update to enable route")
	}
}
```

- [ ] **Step 2: Run the model tests to verify they fail**

Run:

```bash
go test ./model -run 'Test(RebuildRoutesForProvider_PreservesManualDisableAndAllowsAutoRecovery|UpdateModelRouteFields_ManualEditClearsAutoDisabledByBalance|BatchUpdateModelRoutes_ManualEditClearsAutoDisabledByBalance)$'
```

Expected: FAIL with compile errors mentioning `AutoDisabledByBalance` being undefined on `ModelRoute`, plus the manual-update semantics not existing yet.

- [ ] **Step 3: Implement the model-layer persistence changes**

Update `model/model_route.go` with these changes.

Add the new field to `ModelRoute` and expose an exported balance parser wrapper:

```go
type ModelRoute struct {
	Id                    int    `json:"id"`
	ModelName             string `json:"model_name" gorm:"type:varchar(255);index;not null"`
	ProviderTokenId       int    `json:"provider_token_id" gorm:"index;not null"`
	ProviderId            int    `json:"provider_id" gorm:"index;not null"`
	Enabled               bool   `json:"enabled" gorm:"default:true"`
	AutoDisabledByBalance bool   `json:"-" gorm:"default:false"`
	Priority              int    `json:"priority" gorm:"default:0;index"`
	Weight                int    `json:"weight" gorm:"default:10"`
}

func ParseBalanceUSD(raw string) float64 {
	return parseBalanceUSD(raw)
}
```

Add a helper that turns any explicit `enabled` patch into a manual source of truth:

```go
func applyManualRouteEnabledOverride(updates map[string]interface{}) {
	if _, ok := updates["enabled"]; ok {
		updates["auto_disabled_by_balance"] = false
	}
}
```

Call that helper from both route-update entry points:

```go
func UpdateModelRouteFields(id int, updates map[string]interface{}) error {
	if id <= 0 {
		return errors.New("invalid route ID")
	}
	if len(updates) == 0 {
		return errors.New("no updates provided")
	}
	applyManualRouteEnabledOverride(updates)
	return DB.Model(&ModelRoute{}).Where("id = ?", id).Updates(updates).Error
}

func BatchUpdateModelRoutes(patches []ModelRoutePatch) error {
	if len(patches) == 0 {
		return errors.New("empty update list")
	}
	tx := DB.Begin()
	for _, patch := range patches {
		if patch.Id <= 0 {
			tx.Rollback()
			return errors.New("invalid route ID in batch")
		}
		updates := patch.ToUpdates()
		if len(updates) == 0 {
			continue
		}
		applyManualRouteEnabledOverride(updates)
		if err := tx.Model(&ModelRoute{}).Where("id = ?", patch.Id).Updates(updates).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}
```

Change the carry-forward rule in `RebuildRoutesForProvider(...)`:

```go
for i := range routes {
	key := routeModelTokenKey(routes[i].ModelName, routes[i].ProviderTokenId)
	if previous, ok := existingMap[key]; ok {
		if !previous.Enabled && !previous.AutoDisabledByBalance {
			routes[i].Enabled = false
			routes[i].AutoDisabledByBalance = false
		}
		routes[i].Priority = previous.Priority
		routes[i].Weight = previous.Weight
	}
}
```

- [ ] **Step 4: Run the model tests to verify they pass**

Run:

```bash
go test ./model -run 'Test(RebuildRoutesForProvider_PreservesManualDisableAndAllowsAutoRecovery|UpdateModelRouteFields_ManualEditClearsAutoDisabledByBalance|BatchUpdateModelRoutes_ManualEditClearsAutoDisabledByBalance)$'
```

Expected: PASS

- [ ] **Step 5: Commit the model-layer change**

Run:

```bash
git add model/model_route.go model/model_route_rebuild_test.go
git commit -m "feat: persist low balance route disable state"
```

### Task 2: Apply the Low-Balance Rule During Route Rebuild

**Files:**
- Modify: `service/sync.go`
- Create: `service/sync_rebuild_routes_test.go`
- Regression: `model/model_route.go`

- [ ] **Step 1: Write the failing service-level rebuild test**

Create `service/sync_rebuild_routes_test.go` with this content:

```go
package service

import (
	"NewAPI-Gateway/model"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupServiceSyncTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "service-sync-test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	if err := model.DB.AutoMigrate(&model.Provider{}, &model.ProviderToken{}, &model.ModelPricing{}, &model.ModelRoute{}); err != nil {
		t.Fatalf("auto migrate sync test tables: %v", err)
	}
}

func fetchServiceRoute(t *testing.T, providerID int, modelName string) model.ModelRoute {
	t.Helper()

	var route model.ModelRoute
	if err := model.DB.Where("provider_id = ? AND model_name = ?", providerID, modelName).First(&route).Error; err != nil {
		t.Fatalf("fetch rebuilt route: %v", err)
	}
	return route
}

func TestRebuildProviderRoutes_UsesBalanceThresholdAndPreservesManualDisable(t *testing.T) {
	setupServiceSyncTestDB(t)

	provider := model.Provider{
		Id:       1,
		Name:     "provider-a",
		BaseURL:  "https://example.com",
		Status:   1,
		Balance:  "$0.99",
		Priority: 2,
		Weight:   5,
	}
	if err := model.DB.Create(&provider).Error; err != nil {
		t.Fatalf("seed provider: %v", err)
	}

	token := model.ProviderToken{
		Id:         10,
		ProviderId: provider.Id,
		Name:       "token-a",
		GroupName:  "default",
		Status:     1,
		Priority:   3,
		Weight:     8,
	}
	if err := model.DB.Create(&token).Error; err != nil {
		t.Fatalf("seed token: %v", err)
	}

	pricing := model.ModelPricing{
		ProviderId:   provider.Id,
		ModelName:    "gpt-4o",
		EnableGroups: `["default"]`,
	}
	if err := model.DB.Create(&pricing).Error; err != nil {
		t.Fatalf("seed pricing: %v", err)
	}

	if err := RebuildProviderRoutes(provider.Id); err != nil {
		t.Fatalf("rebuild low-balance routes: %v", err)
	}

	route := fetchServiceRoute(t, provider.Id, "gpt-4o")
	if route.Enabled {
		t.Fatalf("expected $0.99 balance to disable rebuilt route")
	}
	if !route.AutoDisabledByBalance {
		t.Fatalf("expected low-balance rebuild to mark route auto-disabled")
	}

	if err := model.DB.Model(&model.Provider{}).Where("id = ?", provider.Id).Update("balance", "$1.00").Error; err != nil {
		t.Fatalf("update balance to recovery threshold: %v", err)
	}
	if err := RebuildProviderRoutes(provider.Id); err != nil {
		t.Fatalf("rebuild recovered routes: %v", err)
	}

	route = fetchServiceRoute(t, provider.Id, "gpt-4o")
	if !route.Enabled {
		t.Fatalf("expected $1.00 balance to re-enable rebuilt route")
	}
	if route.AutoDisabledByBalance {
		t.Fatalf("expected recovered route to clear auto-disabled marker")
	}

	if err := model.DB.Model(&model.Provider{}).Where("id = ?", provider.Id).Update("balance", "$0.99").Error; err != nil {
		t.Fatalf("drop balance below threshold again: %v", err)
	}
	if err := RebuildProviderRoutes(provider.Id); err != nil {
		t.Fatalf("rebuild after balance drops again: %v", err)
	}

	route = fetchServiceRoute(t, provider.Id, "gpt-4o")
	if route.Enabled || !route.AutoDisabledByBalance {
		t.Fatalf("expected repeated low-balance rebuild to disable route again")
	}

	if err := model.UpdateModelRouteFields(route.Id, map[string]interface{}{"enabled": false}); err != nil {
		t.Fatalf("manually disable route: %v", err)
	}

	route = fetchServiceRoute(t, provider.Id, "gpt-4o")
	if route.AutoDisabledByBalance {
		t.Fatalf("expected manual disable to clear auto-disabled marker")
	}

	if err := model.DB.Model(&model.Provider{}).Where("id = ?", provider.Id).Update("balance", "$1.00").Error; err != nil {
		t.Fatalf("recover balance after manual disable: %v", err)
	}
	if err := RebuildProviderRoutes(provider.Id); err != nil {
		t.Fatalf("rebuild after recovery with manual disable: %v", err)
	}

	route = fetchServiceRoute(t, provider.Id, "gpt-4o")
	if route.Enabled {
		t.Fatalf("expected manual disable to survive balance recovery")
	}
	if route.AutoDisabledByBalance {
		t.Fatalf("expected recovered manual disable to stay manual")
	}
}
```

- [ ] **Step 2: Run the service test to verify it fails**

Run:

```bash
go test ./service -run '^TestRebuildProviderRoutes_UsesBalanceThresholdAndPreservesManualDisable$'
```

Expected: FAIL because `RebuildProviderRoutes(...)` still generates enabled routes regardless of provider balance.

- [ ] **Step 3: Implement the rebuild-time low-balance rule**

Update `service/sync.go` so `RebuildProviderRoutes(...)` loads the provider, derives `providerBalanceLow`, and stamps generated routes with both `Enabled` and `AutoDisabledByBalance`.

Replace the top of `RebuildProviderRoutes(...)` with:

```go
func RebuildProviderRoutes(providerId int) error {
	provider, err := model.GetProviderById(providerId)
	if err != nil {
		return err
	}

	providerBalanceLow := model.ParseBalanceUSD(provider.Balance) < 1

	tokens, err := model.GetEnabledProviderTokensByProviderId(providerId)
	if err != nil {
		return err
	}

	pricing, err := model.GetModelPricingByProvider(providerId)
	if err != nil {
		return err
	}
```

When appending generated routes, stamp the new fields like this:

```go
routes = append(routes, model.ModelRoute{
	ModelName:             modelName,
	ProviderTokenId:       token.Id,
	ProviderId:            providerId,
	Enabled:               !providerBalanceLow,
	AutoDisabledByBalance: providerBalanceLow,
	Priority:              token.Priority,
	Weight:                token.Weight,
})
```

Use the already loaded `provider` for logging at the end of the function:

```go
common.SysLog(fmt.Sprintf("rebuilt %d routes for provider %s", len(routes), provider.Name))
```

- [ ] **Step 4: Run the targeted regression tests**

Run:

```bash
go test ./model -run 'Test(RebuildRoutesForProvider_PreservesManualDisableAndAllowsAutoRecovery|UpdateModelRouteFields_ManualEditClearsAutoDisabledByBalance|BatchUpdateModelRoutes_ManualEditClearsAutoDisabledByBalance)$'
go test ./service -run '^TestRebuildProviderRoutes_UsesBalanceThresholdAndPreservesManualDisable$'
```

Expected: PASS

- [ ] **Step 5: Commit the rebuild behavior change**

Run:

```bash
git add service/sync.go service/sync_rebuild_routes_test.go model/model_route.go
git commit -m "feat: disable rebuilt routes for low-balance providers"
```

### Task 3: Document the Rebuild Rule and Verify the Full Change

**Files:**
- Modify: `README.md`
- Modify: `docs/ARCHITECTURE.md`

- [ ] **Step 1: Update the user-facing and architecture docs**

In `README.md`, replace step 4 under `### 2. 同步数据` with:

```md
4. 自动重建模型路由表；当供应商余额低于 `$1.00` 时，该供应商本次重建出的路由会自动禁用。余额恢复并再次重建后会自动恢复；手动禁用的路由不会被自动恢复。
```

In `docs/ARCHITECTURE.md`, replace step 4 under `### 单供应商同步流程` with:

```md
4. 按 token 分组与模型可用组关系重建该供应商 `model_routes`；若 `providers.balance < 1`，则本次生成的路由写入 `enabled = false` 且 `auto_disabled_by_balance = true`。余额恢复后下一次重建自动恢复，但人工禁用的路由保持禁用。
```

- [ ] **Step 2: Verify the docs mention the new rule**

Run:

```bash
rg -n "低于 `\\$1\\.00`|providers\\.balance < 1|auto_disabled_by_balance" README.md docs/ARCHITECTURE.md
```

Expected: one hit in `README.md` and one hit in `docs/ARCHITECTURE.md`.

- [ ] **Step 3: Run the final regression suite**

Run:

```bash
go test ./model ./service
```

Expected: PASS

- [ ] **Step 4: Commit the docs and final verification state**

Run:

```bash
git add README.md docs/ARCHITECTURE.md
git commit -m "docs: describe low balance route rebuild behavior"
```

## Self-Review

### Spec coverage

- Balance sync behavior unchanged: covered by Task 2 because only `RebuildProviderRoutes(...)` changes.
- Threshold checked during rebuild only: covered by Task 2 service implementation and test.
- Balance `< 1` disables rebuilt routes: covered by Task 2 test at `$0.99`.
- Balance `>= 1` auto-restores routes: covered by Task 2 test at `$1.00`.
- Manual disables persist across recovery: covered by Task 1 persistence rule and Task 2 integration test.
- Manual route edits clear the automatic marker: covered by Task 1 tests and implementation.
- Docs updated: covered by Task 3.

No spec gaps found.

### Placeholder scan

- No placeholder markers remain in the document.
- Every task includes exact file paths, code snippets, commands, and expected outcomes.

### Type consistency

- The new field name is consistently `AutoDisabledByBalance` in Go and `auto_disabled_by_balance` in SQL update maps.
- The exported balance helper is consistently named `ParseBalanceUSD`.
- Rebuild logic consistently sets both `Enabled` and `AutoDisabledByBalance`.
