# Route System Prompt Injection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add administrator-managed, model-specific system prompt presets that a model route can inject as the first message of OpenAI chat-completions requests.

**Architecture:** Store presets in a dedicated `system_prompts` table and a nullable preset reference on `model_routes`. Resolve the selected preset for each concrete retry attempt, rebuild the outgoing JSON from the cached original body, and inject only for `POST /v1/chat/completions`. Expose admin CRUD APIs and a dedicated React page, then extend the existing route-table batch editor with a model-filtered preset selector.

**Tech Stack:** Go 1.22, Gin, GORM, SQLite/MySQL-compatible migrations, React 18, Axios, Jest/React Testing Library.

---

## File Map

- Create `model/system_prompt.go`: preset model, validation, CRUD, reference counting, transactional deletion.
- Create `model/system_prompt_test.go`: model-level persistence and deletion tests.
- Modify `model/model_route.go`: nullable binding, patch support, overview projection, rebuild preservation.
- Modify `model/main.go`: auto-migrate the preset model and add the route column.
- Create `controller/system_prompt.go`: administrator CRUD HTTP handlers.
- Create `controller/system_prompt_test.go`: API validation and deletion behavior.
- Modify `router/api-router.go`: register protected preset endpoints.
- Create `service/system_prompt_injection.go`: pure OpenAI chat request transformation.
- Create `service/system_prompt_injection_test.go`: transformation unit tests.
- Modify `controller/relay.go`: pass the selected route into each proxy attempt.
- Modify `service/proxy.go`: resolve the attempt preset and transform only the attempt body.
- Modify `service/proxy_test.go`: retry/body integration coverage.
- Create `web/src/pages/SystemPrompt/index.js`: prompt management page.
- Create `web/src/pages/SystemPrompt/index.test.js`: page behavior tests.
- Modify `web/src/App.js`: lazy route registration.
- Modify `web/src/components/Layout.js`: administrator navigation item.
- Modify `web/src/components/ModelRoutesTable.js`: preset loading, selector, and batch patch.
- Modify `web/src/components/ModelRoutesTable.test.js`: selector filtering and persistence tests.
- Modify `docs/API_REFERENCE.md`: document admin endpoints and route binding field.

### Task 1: Persist Prompt Presets and Route Bindings

**Files:**
- Create: `model/system_prompt.go`
- Create: `model/system_prompt_test.go`
- Modify: `model/model_route.go`
- Modify: `model/main.go`

- [ ] **Step 1: Write failing model tests**

Add table-driven tests that initialize the existing test DB, create two presets with the same name for different models, reject duplicates for one model, count route references, reject referenced deletion without unbinding, and atomically null bindings with unbinding enabled. Use concrete records:

```go
p := &SystemPrompt{Name: "Writer", ModelName: "gpt-4.1", Content: "Write precisely."}
if err := CreateSystemPrompt(p); err != nil { t.Fatal(err) }
route := &ModelRoute{ModelName: "gpt-4.1", ProviderId: 1, ProviderTokenId: 1, SystemPromptId: &p.Id}
if err := DB.Create(route).Error; err != nil { t.Fatal(err) }
if _, err := DeleteSystemPrompt(p.Id, false); !errors.Is(err, ErrSystemPromptInUse) { t.Fatalf("got %v", err) }
if count, err := DeleteSystemPrompt(p.Id, true); err != nil || count != 1 { t.Fatalf("count=%d err=%v", count, err) }
```

- [ ] **Step 2: Run the focused test and verify failure**

Run: `go test ./model -run SystemPrompt -count=1`

Expected: FAIL because `SystemPrompt`, CRUD functions, and `SystemPromptId` do not exist.

- [ ] **Step 3: Implement the preset model and migration**

Define focused APIs in `model/system_prompt.go`:

```go
type SystemPrompt struct {
    Id int `json:"id"`
    Name string `json:"name" gorm:"type:varchar(255);not null;uniqueIndex:idx_system_prompt_model_name"`
    ModelName string `json:"model_name" gorm:"type:varchar(255);not null;uniqueIndex:idx_system_prompt_model_name;index"`
    Content string `json:"content" gorm:"type:text;not null"`
    CreatedAt int64 `json:"created_at"`
    UpdatedAt int64 `json:"updated_at"`
    RouteCount int64 `json:"route_count" gorm:"-"`
}

var ErrSystemPromptInUse = errors.New("system prompt is in use")

func CreateSystemPrompt(prompt *SystemPrompt) error
func UpdateSystemPrompt(prompt *SystemPrompt) error
func ListSystemPrompts(modelName, keyword string) ([]*SystemPrompt, error)
func GetSystemPromptByID(id int) (*SystemPrompt, error)
func DeleteSystemPrompt(id int, unbind bool) (int64, error)
```

Normalize with `strings.TrimSpace`, reject empty name/model/content, and execute unbind plus delete in one `DB.Transaction`. Add `SystemPromptId *int` to `ModelRoute`, `ModelRoutePatch`, `ToUpdates`, and overview query/result mapping. Add `AutoMigrate(&SystemPrompt{})` and ensure `ModelRoute` migration adds the nullable column.

- [ ] **Step 4: Preserve bindings during route rebuild**

When `RebuildRoutesForProvider` matches an existing route by its current stable identity key, copy `existing.SystemPromptId` into the rebuilt row. New or unmatched routes remain nil.

- [ ] **Step 5: Run model tests**

Run: `go test ./model -count=1`

Expected: PASS.

- [ ] **Step 6: Commit persistence changes**

```bash
git add model/system_prompt.go model/system_prompt_test.go model/model_route.go model/main.go
git commit -m "feat: persist route system prompt presets"
```

### Task 2: Add Administrator Prompt APIs

**Files:**
- Create: `controller/system_prompt.go`
- Create: `controller/system_prompt_test.go`
- Modify: `router/api-router.go`

- [ ] **Step 1: Write failing handler tests**

Build a Gin test router with the handlers and assert create/list/update, exact `model` filtering, duplicate-name errors, referenced deletion returning a conflict-style response, and `?unbind=true` clearing the route. Assert the response envelope follows the project convention:

```go
if body["success"] != true { t.Fatalf("response=%v", body) }
```

- [ ] **Step 2: Verify handler tests fail**

Run: `go test ./controller -run SystemPrompt -count=1`

Expected: FAIL because handlers are undefined.

- [ ] **Step 3: Implement CRUD handlers**

Create handlers:

```go
func GetSystemPrompts(c *gin.Context)
func CreateSystemPrompt(c *gin.Context)
func UpdateSystemPrompt(c *gin.Context)
func DeleteSystemPrompt(c *gin.Context)
```

Parse positive integer IDs, use `model` and `keyword` list parameters, require an explicit case-insensitive `unbind=true`, and return `success`, `message`, and `data`. On an in-use delete, return `success:false` with `data.route_count` so the UI can offer automatic unbinding.

- [ ] **Step 4: Register protected routes**

Under `/api/system-prompt`, apply `middleware.AdminAuth()` and `middleware.NoTokenAuth()` and register GET, POST, PUT `/:id`, and DELETE `/:id`.

- [ ] **Step 5: Run controller tests**

Run: `go test ./controller -run SystemPrompt -count=1`

Expected: PASS.

- [ ] **Step 6: Commit API changes**

```bash
git add controller/system_prompt.go controller/system_prompt_test.go router/api-router.go
git commit -m "feat: add system prompt admin api"
```

### Task 3: Validate Route Prompt Updates

**Files:**
- Modify: `model/model_route.go`
- Modify: `model/model_route_test.go`
- Modify: `controller/route_test.go`

- [ ] **Step 1: Write failing binding tests**

Test matching binding, cross-model rejection, nonexistent ID rejection, clearing with JSON null, and batch rollback when one patch is invalid. Use `SystemPromptId` pointers for selection and an explicit nullable patch representation so omitted and null are distinguishable.

- [ ] **Step 2: Verify tests fail**

Run: `go test ./model ./controller -run 'Route.*SystemPrompt|SystemPrompt.*Route' -count=1`

Expected: FAIL because patch validation and nullable clearing are absent.

- [ ] **Step 3: Implement validated nullable patching**

Introduce a small nullable JSON field type or custom unmarshal structure that distinguishes:

```json
{}
{"system_prompt_id": null}
{"system_prompt_id": 12}
```

Before single or batch updates, load the target route and selected preset and require exact `ModelName` equality. Perform the complete batch validation before updating, then use the existing transaction.

- [ ] **Step 4: Run route tests**

Run: `go test ./model ./controller -run 'Route|SystemPrompt' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit binding validation**

```bash
git add model/model_route.go model/model_route_test.go controller/route_test.go
git commit -m "feat: validate route prompt bindings"
```

### Task 4: Inject the Prompt Per Proxy Attempt

**Files:**
- Create: `service/system_prompt_injection.go`
- Create: `service/system_prompt_injection_test.go`
- Modify: `controller/relay.go`
- Modify: `service/proxy.go`
- Modify: `service/proxy_test.go`

- [ ] **Step 1: Write failing pure transformation tests**

Cover no prompt, empty messages, existing system message ordering, Unicode/multiline content, missing messages, non-array messages, and non-chat paths. Target this interface:

```go
func injectRouteSystemPrompt(method, path string, body []byte, content string) ([]byte, error)
```

Assert the decoded first message equals `map[string]any{"role":"system", "content":"第一行\n\"quoted\""}` and the original messages follow unchanged.

- [ ] **Step 2: Verify transformation tests fail**

Run: `go test ./service -run RouteSystemPrompt -count=1`

Expected: FAIL because the transformer does not exist.

- [ ] **Step 3: Implement the pure transformer**

Return the input bytes immediately unless method is POST, path is exactly `/v1/chat/completions`, and content is non-empty. Decode into a structure retaining other top-level fields via `map[string]json.RawMessage`, require `messages` as `[]json.RawMessage`, prepend a JSON-marshaled system message, then encode the body.

- [ ] **Step 4: Pass route identity into the proxy attempt**

Change the call boundary consistently:

```go
func ProxyToUpstream(c *gin.Context, route model.ModelRoute, token *model.ProviderToken, provider *model.Provider) *ProxyAttemptError
```

Call it from `controller.Relay` with `attempt.Route`. After `getRequestBodyBytes` and `rewriteRequestModel`, resolve `route.SystemPromptId`; on a missing/mismatched preset return a retryable route-attempt error and log the route ID. Transform the local `bodyBytes` only, leaving the cached original untouched.

- [ ] **Step 5: Add retry integration tests**

Use two upstream test servers/routes with distinct presets. Make the first return a retryable failure and the second capture its body. Assert each upstream receives exactly one injected message and the second receives only its own preset. Add a trace assertion that captured request content includes the actual injected body when tracing is enabled.

- [ ] **Step 6: Run proxy and relay tests**

Run: `go test ./service ./controller -run 'Proxy|Relay|RouteSystemPrompt' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit proxy injection**

```bash
git add service/system_prompt_injection.go service/system_prompt_injection_test.go service/proxy.go service/proxy_test.go controller/relay.go
git commit -m "feat: inject route system prompts"
```

### Task 5: Build the Prompt Management Page

**Files:**
- Create: `web/src/pages/SystemPrompt/index.js`
- Create: `web/src/pages/SystemPrompt/index.test.js`
- Modify: `web/src/App.js`
- Modify: `web/src/components/Layout.js`

- [ ] **Step 1: Write failing page tests**

Mock `API.get/post/put/delete` and test loading, model filtering, create/edit validation, and referenced deletion. The referenced-delete test must assert the first DELETE omits `unbind`, the dialog displays the returned route count, and confirmation sends `?unbind=true`.

- [ ] **Step 2: Verify frontend tests fail**

Run: `cd web; npm test -- --runInBand --watchAll=false SystemPrompt`

Expected: FAIL because the page is missing.

- [ ] **Step 3: Implement the page using existing UI components**

Use `Card`, `Table`, `Input`, `Button`, and `Modal`; keep API access through `API` and feedback through `showError/showSuccess`. Render name, model, a bounded content summary, route count, update time, and edit/delete controls. Require trimmed non-empty fields before submission.

- [ ] **Step 4: Register navigation and route**

Lazy-load the page in `web/src/App.js` at `/system-prompts`. Add an administrator-only `System Prompts` navigation item in `Layout.js` using an existing Lucide text/document icon.

- [ ] **Step 5: Run page tests**

Run: `cd web; npm test -- --runInBand --watchAll=false SystemPrompt`

Expected: PASS.

- [ ] **Step 6: Commit the management page**

```bash
git add web/src/pages/SystemPrompt/index.js web/src/pages/SystemPrompt/index.test.js web/src/App.js web/src/components/Layout.js
git commit -m "feat: add system prompt management page"
```

### Task 6: Add Route-Table Prompt Selectors

**Files:**
- Modify: `web/src/components/ModelRoutesTable.js`
- Modify: `web/src/components/ModelRoutesTable.test.js`

- [ ] **Step 1: Write failing selector tests**

Mock the prompt list endpoint with presets for two models. Assert each route row shows `No system prompt` plus only exact-model presets, initial selection reflects `system_prompt_id`, changing selection adds the nullable field to the existing batch payload, and selecting the empty option sends null.

- [ ] **Step 2: Verify selector tests fail**

Run: `cd web; npm test -- --runInBand --watchAll=false ModelRoutesTable`

Expected: FAIL because no selector or prompt fetch exists.

- [ ] **Step 3: Implement prompt loading and selection state**

Fetch presets once for the models visible in the route data, index them by exact `model_name`, and render a `System Prompt` column. Store the selected ID in the same editable-row state used by priority, weight, enabled, and client restrictions. Use an empty option value for null and numeric IDs for selections.

- [ ] **Step 4: Extend the batch patch**

Include `system_prompt_id` only for rows whose selection changed. Send JSON null when cleared. Preserve the current dirty-state, save success, refresh, sorting, and pagination behavior.

- [ ] **Step 5: Run route-table tests**

Run: `cd web; npm test -- --runInBand --watchAll=false ModelRoutesTable`

Expected: PASS.

- [ ] **Step 6: Commit route selectors**

```bash
git add web/src/components/ModelRoutesTable.js web/src/components/ModelRoutesTable.test.js
git commit -m "feat: select prompts per model route"
```

### Task 7: Documentation and Full Verification

**Files:**
- Modify: `docs/API_REFERENCE.md`

- [ ] **Step 1: Document the feature contract**

Add the four administrator endpoints, request/response examples, `unbind=true` behavior, nullable `system_prompt_id` in route updates, exact-model validation, and the statement that injection applies only to `POST /v1/chat/completions`.

- [ ] **Step 2: Run Go formatting and static tests**

Run: `gofmt -w model/system_prompt.go model/system_prompt_test.go model/model_route.go model/main.go controller/system_prompt.go controller/system_prompt_test.go controller/route_test.go controller/relay.go router/api-router.go service/system_prompt_injection.go service/system_prompt_injection_test.go service/proxy.go service/proxy_test.go`

Run: `go test ./... -count=1`

Expected: all Go packages PASS.

- [ ] **Step 3: Run the complete frontend suite**

Run: `cd web; npm test -- --runInBand --watchAll=false`

Expected: all frontend tests PASS.

- [ ] **Step 4: Build both applications**

Run: `go build ./...`

Expected: exit code 0.

Run: `cd web; npm run build`

Expected: production build completes successfully.

- [ ] **Step 5: Inspect migration compatibility manually**

Start against a copy of an existing database, verify old routes show null prompt bindings, create a preset, bind one route, send a chat-completions request to a capture upstream, and confirm the injected message is first. Rebuild that provider's routes and verify the stable route retains the binding.

- [ ] **Step 6: Commit documentation and final adjustments**

```bash
git add docs/API_REFERENCE.md
git commit -m "docs: document route system prompt injection"
```

- [ ] **Step 7: Verify the final diff**

Run: `git status --short`

Expected: no feature-related uncommitted files; pre-existing unrelated untracked files may remain.

Run: `git log --oneline -8`

Expected: the feature commits appear in task order.
