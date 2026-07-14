# Stable Model Route IDs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve unchanged model route IDs during provider synchronization and return errors when route updates reference deleted IDs.

**Architecture:** Replace provider route delete/reinsert with a transaction-scoped diff keyed by provider token and model name. Route update functions share a GORM helper that distinguishes missing IDs from database-specific zero-row results for idempotent updates.

**Tech Stack:** Go 1.18, GORM, SQLite in-memory tests, MySQL/PostgreSQL-compatible SQL behavior.

---

Implementation stays in the current worktree. Do not commit changes unless the user separately authorizes a commit.

### Task 1: Preserve Route IDs During Rebuild

**Files:**
- Modify: `model/model_route_test.go`
- Modify: `model/model_route.go:1372`

- [ ] **Step 1: Add the failing stable-ID regression test**

Append this test after `TestRebuildRoutesForProviderPreservesDisabledRoute`:

```go
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
```

- [ ] **Step 2: Add a characterization test for additions and removals**

Append this test to lock the existing rebuild semantics before refactoring:

```go
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
```

- [ ] **Step 3: Run the rebuild tests and verify the expected red state**

Run:

```powershell
go test ./model -run 'TestRebuildRoutesForProvider(PreservesExistingRouteIDAndSettings|AddsAndRemovesRoutes)$' -count=1
```

Expected: `TestRebuildRoutesForProviderPreservesExistingRouteIDAndSettings` fails because the rebuilt row receives a new ID. The additions/removals characterization test passes.

- [ ] **Step 4: Replace delete/reinsert with a transactional diff**

Keep the existing `routeInsert` type and existing-route query. Replace the preservation loop, provider-wide delete, and batch construction with this flow:

```go
	generatedKeys := make(map[string]struct{}, len(routes))
	newRoutes := make([]ModelRoute, 0, len(routes))
	for _, route := range routes {
		key := routeModelTokenKey(route.ModelName, route.ProviderTokenId)
		generatedKeys[key] = struct{}{}
		if previous, ok := existingMap[key]; ok {
			if err := tx.Model(&ModelRoute{}).Where("id = ?", previous.Id).Updates(map[string]interface{}{
				"priority": route.Priority,
				"weight":   route.Weight,
			}).Error; err != nil {
				tx.Rollback()
				return err
			}
			continue
		}
		newRoutes = append(newRoutes, route)
	}

```

Change the batch loop to iterate over `newRoutes` and build inserts from `newRoutes[i:end]`:

```go
	batchSize := 50
	for i := 0; i < len(newRoutes); i += batchSize {
		end := i + batchSize
		if end > len(newRoutes) {
			end = len(newRoutes)
		}
		batch := make([]routeInsert, 0, end-i)
		for _, route := range newRoutes[i:end] {
			enabled := route.Enabled
			batch = append(batch, routeInsert{
				ModelName:       route.ModelName,
				ProviderTokenId: route.ProviderTokenId,
				ProviderId:      route.ProviderId,
				Enabled:         &enabled,
				Priority:        route.Priority,
				Weight:          route.Weight,
				AllowCodex:      route.AllowCodex,
				AllowCC:         route.AllowCC,
				BlockClients:    route.BlockClients,
			})
		}
		if err := tx.Table("model_routes").Create(&batch).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	staleIDs := make([]int, 0)
	for key, route := range existingMap {
		if _, ok := generatedKeys[key]; !ok {
			staleIDs = append(staleIDs, route.Id)
		}
	}
	if len(staleIDs) > 0 {
		if err := tx.Where("id IN ?", staleIDs).Delete(&ModelRoute{}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
```

Matched rows only refresh generated routing fields, so their IDs and manual fields remain unchanged. Empty input still deletes every route belonging to the provider.

- [ ] **Step 5: Run the rebuild tests and verify green**

Run:

```powershell
go test ./model -run 'TestRebuildRoutesForProvider' -count=1
```

Expected: all rebuild tests pass.

### Task 2: Reject Missing Route IDs Atomically

**Files:**
- Modify: `model/model_route_test.go`
- Modify: `model/model_route.go:1077`

- [ ] **Step 1: Add failing single and batch update tests**

Append these tests:

```go
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
```

- [ ] **Step 2: Run the update tests and verify red**

Run:

```powershell
go test ./model -run 'Test(UpdateModelRouteFieldsRejectsMissingRoute|BatchUpdateModelRoutesRollsBackWhenRouteMissing)$' -count=1
```

Expected: both tests fail because zero-row updates currently return success and the batch commits.

- [ ] **Step 3: Add a shared update helper with missing-ID detection**

Add `gorm.io/gorm` to the imports in `model/model_route.go`, then add this helper above `UpdateModelRouteFields`:

```go
func updateModelRouteFieldsWithDB(db *gorm.DB, id int, updates map[string]interface{}) error {
	result := db.Model(&ModelRoute{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}

	var count int64
	if err := db.Model(&ModelRoute{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return errors.New("模型路由不存在")
	}
	return nil
}
```

Replace the final statement in `UpdateModelRouteFields` with:

```go
	return updateModelRouteFieldsWithDB(DB, id, updates)
```

Replace the update block in `BatchUpdateModelRoutes` with:

```go
		if err := updateModelRouteFieldsWithDB(tx, patch.Id, updates); err != nil {
			tx.Rollback()
			return err
		}
```

- [ ] **Step 4: Run the update tests and verify green**

Run:

```powershell
go test ./model -run 'Test(UpdateModelRouteFields|BatchUpdateModelRoutes)' -count=1
```

Expected: the new missing-ID tests and existing normalization/persistence tests pass.

### Task 3: Format and Verify the Complete Fix

**Files:**
- Modify: `model/model_route.go`
- Modify: `model/model_route_test.go`
- Verify: `docs/superpowers/specs/2026-07-14-model-route-stable-id-design.md`
- Verify: `docs/superpowers/plans/2026-07-14-model-route-stable-id.md`

- [ ] **Step 1: Format the changed Go files**

Run:

```powershell
gofmt -w model/model_route.go model/model_route_test.go
```

Expected: command completes without output.

- [ ] **Step 2: Run the focused model route tests**

Run:

```powershell
go test ./model -run 'Test(RebuildRoutesForProvider|UpdateModelRouteFields|BatchUpdateModelRoutes)' -count=1
```

Expected: pass.

- [ ] **Step 3: Run the full Go test suite**

Run:

```powershell
go test ./... -count=1
```

Expected: all packages pass.

- [ ] **Step 4: Inspect the final diff and worktree**

Run:

```powershell
git diff -- model/model_route.go model/model_route_test.go
git status --short
```

Expected: production changes are limited to model route rebuild/update behavior, tests cover the regression, the two design documents are untracked, and the user's existing `error.txt`, `req.txt`, and `resp.txt` remain untouched.
