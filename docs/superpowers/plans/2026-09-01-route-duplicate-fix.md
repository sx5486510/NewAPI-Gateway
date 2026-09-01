# Route Duplicate Entries Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent duplicate model-route/token records, make route rebuilds succeed on large SQLite datasets, and keep aliased model names distinguishable in the route UI.

**Architecture:** Route rebuild will deduplicate generated and persisted rows, delete stale IDs in bounded batches, and serialize rebuilds per provider. Runtime route planning will retain one candidate per provider token. Startup migrations will repair historical duplicates before creating unique indexes.

**Tech Stack:** Go, GORM, SQLite/MySQL/PostgreSQL, React, Jest.

---

### Task 1: Make route rebuild safe for large and duplicate datasets

**Files:**
- Modify: `model/model_route.go`
- Test: `model/model_route_test.go`

- [ ] Add failing tests covering duplicate existing rows and stale-ID deletion beyond SQLite variable limits.
- [ ] Run the focused tests and confirm the current implementation fails with duplicate rows or `too many SQL variables`.
- [ ] Add bounded ID deletion, deterministic duplicate retention, and generated-key deduplication inside `RebuildRoutesForProvider`.
- [ ] Run focused model tests and the full model package.

### Task 2: Prevent concurrent provider rebuilds and duplicate runtime candidates

**Files:**
- Modify: `service/sync.go`
- Modify: `model/model_route.go`
- Test: `model/model_route_test.go`
- Test: `service/sync_test.go`

- [ ] Add failing tests for concurrent rebuild serialization and one candidate per provider token.
- [ ] Run the tests to verify the failures.
- [ ] Add provider-scoped rebuild locking and candidate deduplication while preserving exact-model preference.
- [ ] Run model/service focused tests.

### Task 3: Repair historical token/route duplicates and add database constraints

**Files:**
- Modify: `model/main.go`
- Modify: `model/provider_token.go`
- Modify: `model/model_route.go`
- Test: `model/main_test.go`

- [ ] Add failing migration tests that seed duplicate token/route rows and assert one survivor plus unique indexes.
- [ ] Run the tests and confirm the migration does not currently repair them.
- [ ] Implement idempotent duplicate cleanup and unique-index creation with driver-aware migrations.
- [ ] Run migration tests and the full model package.

### Task 4: Show original model names in the route detail UI

**Files:**
- Modify: `web/src/components/ModelRoutesTable.js`
- Test: `web/src/components/ModelRoutesTable.test.js`

- [ ] Add a failing rendering test for aliased raw model names.
- [ ] Run the focused frontend test and confirm failure.
- [ ] Render the raw model name alongside the normalized display name without changing filtering behavior.
- [ ] Run focused frontend tests and production build.

### Task 5: Final verification

- [ ] Run `go test ./model ./service ./controller`.
- [ ] Run the relevant Jest tests and frontend build.
- [ ] Inspect git diff and report any remaining limitations before claiming completion.
