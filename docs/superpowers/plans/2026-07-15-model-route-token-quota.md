# Model Route Token Quota Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add each model route's synchronized provider-token quota to the route overview API and display it in the model-route detail table.

**Architecture:** Extend the existing `model_routes` to `provider_tokens` left join; do not add a database column or upstream request. Return nullable quota fields so orphaned tokens remain distinguishable from a real zero quota, then format finite raw quota units as USD in React.

**Tech Stack:** Go, GORM, SQLite-backed Go tests, React 18, Jest/react-scripts.

---

### Task 0: Repair the pre-existing stream outcome tests

**Files:**
- Modify: `service/proxy_cooldown_classify_test.go`

- [ ] Add `streamCompleted=false` to the four legacy `streamRouteOutcome` calls.
- [ ] Add a regression assertion that a completed stream remains successful after client cancellation.
- [ ] Run `go test ./service -run '^TestStreamRouteOutcome' -count=1` and commit separately.

### Task 1: Expose token quota through the route overview API

**Files:**
- Modify: `model/model_route_test.go`
- Modify: `model/model_route.go:203-270`
- Modify: `model/model_route.go:1132-1235`

- [ ] **Step 1: Write the failing overview test**

Add `TestGetModelRouteOverviewIncludesTokenQuota`. Create finite, unlimited, and orphaned-token routes, then assert the exact nullable API values:

```go
finite := byModel["finite-model"]
if finite.TokenUnlimitedQuota == nil || *finite.TokenUnlimitedQuota ||
	finite.TokenRemainQuota == nil || *finite.TokenRemainQuota != 250000 ||
	finite.TokenUsedQuota == nil || *finite.TokenUsedQuota != 750000 {
	t.Fatalf("unexpected finite token quota: %+v", finite)
}
unlimited := byModel["unlimited-model"]
if unlimited.TokenUnlimitedQuota == nil || !*unlimited.TokenUnlimitedQuota {
	t.Fatalf("expected unlimited token quota: %+v", unlimited)
}
orphan := byModel["orphan-model"]
if orphan.TokenUnlimitedQuota != nil || orphan.TokenRemainQuota != nil || orphan.TokenUsedQuota != nil {
	t.Fatalf("expected missing token quota to remain nil: %+v", orphan)
}
```

- [ ] **Step 2: Run the focused test and verify it fails**

```powershell
go test ./model -run '^TestGetModelRouteOverviewIncludesTokenQuota$' -count=1
```

Expected: compilation fails because `ModelRouteOverviewItem` has no token quota fields.

- [ ] **Step 3: Add nullable API and query fields**

Add these fields to `ModelRouteOverviewItem`:

```go
TokenUnlimitedQuota *bool  `json:"token_unlimited_quota"`
TokenRemainQuota    *int64 `json:"token_remain_quota"`
TokenUsedQuota      *int64 `json:"token_used_quota"`
```

Add equivalent pointer fields with `gorm:"column:..."` tags to `modelRouteOverviewRow`. Extend the existing select without `COALESCE` so a missing token remains SQL NULL:

```go
"pt.unlimited_quota AS token_unlimited_quota",
"pt.remain_quota AS token_remain_quota",
"pt.used_quota AS token_used_quota",
```

Map the row pointers directly into `ModelRouteOverviewItem`:

```go
TokenUnlimitedQuota: row.TokenUnlimitedQuota,
TokenRemainQuota:    row.TokenRemainQuota,
TokenUsedQuota:      row.TokenUsedQuota,
```

- [ ] **Step 4: Format and run the focused test**

```powershell
gofmt -w model/model_route.go model/model_route_test.go
go test ./model -run '^TestGetModelRouteOverviewIncludesTokenQuota$' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the backend slice**

```powershell
git add model/model_route.go model/model_route_test.go
git commit -m "feat: expose token quota in model route overview"
```

### Task 2: Display token quota in every route row

**Files:**
- Modify: `web/src/components/ModelRoutesTable.test.js`
- Modify: `web/src/components/ModelRoutesTable.js:150-175`
- Modify: `web/src/components/ModelRoutesTable.js:883-1010`

- [ ] **Step 1: Write the failing component test**

Add finite quota fields to the shared route fixture:

```js
token_unlimited_quota: false,
token_remain_quota: 250000,
token_used_quota: 750000,
```

Add a test response containing that route plus an unlimited route and a route whose three quota values are `null`. Assert the new header and values:

```js
expect(document.querySelector('.routes-detail-scroller thead').textContent)
  .toContain('\u4ee4\u724c\u989d\u5ea6');
const quotaValues = [...document.querySelectorAll('.routes-token-quota')]
  .map((node) => node.textContent.trim());
expect(quotaValues).toEqual(expect.arrayContaining(['$0.50', '\u65e0\u9650', '\u2014']));
```

- [ ] **Step 2: Run the focused component test and verify it fails**

Run from `web/`:

```powershell
$env:CI='true'; npm test -- --runInBand ModelRoutesTable.test.js
```

Expected: FAIL because the quota header and cells do not exist.

- [ ] **Step 3: Add quota formatting and the table column**

Add this helper near the price formatters:

```js
const formatTokenQuota = (route) => {
    if (route.token_unlimited_quota == null) return { type: 'missing', label: '\u2014' };
    if (route.token_unlimited_quota === true) return { type: 'unlimited', label: '\u65e0\u9650' };
    if (route.token_remain_quota == null) return { type: 'missing', label: '\u2014' };
    const rawQuota = Number(route.token_remain_quota);
    if (!Number.isFinite(rawQuota)) return { type: 'missing', label: '\u2014' };
    return { type: 'finite', label: `$${(rawQuota / 500000).toFixed(2)}` };
};
```

Update the table to nine columns, insert `令牌额度` after `供应商 / 令牌`, and change empty/group separator `colSpan` values from 8 to 9. Use widths `16, 10, 11, 7, 7, 14, 13, 9, 13`, compute `const tokenQuota = formatTokenQuota(route);`, and render:

```jsx
<Td style={cellMiddleStyle}>
    <div className="routes-token-quota">
        {tokenQuota.type === 'unlimited'
            ? <Badge color="green">{tokenQuota.label}</Badge>
            : <span>{tokenQuota.label}</span>}
    </div>
</Td>
```

- [ ] **Step 4: Run the focused component test**

```powershell
$env:CI='true'; npm test -- --runInBand ModelRoutesTable.test.js
```

Expected: all `ModelRoutesTable` tests PASS.

- [ ] **Step 5: Commit the frontend slice**

```powershell
git add web/src/components/ModelRoutesTable.js web/src/components/ModelRoutesTable.test.js
git commit -m "feat: show token quota on model routes"
```

### Task 3: Verify the complete feature

**Files:**
- Verify: `docs/superpowers/specs/2026-07-15-model-route-token-quota-design.md`
- Verify: `model/model_route.go`
- Verify: `web/src/components/ModelRoutesTable.js`

- [ ] **Step 1: Run focused backend regression tests**

```powershell
go test ./model -run 'Test(GetModelRouteOverviewIncludesTokenQuota|BatchUpdateModelRoutesSavesDisabledRouteAndOverviewReadsIt)' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run the complete Go test suite**

```powershell
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run the focused React suite and production build**

Run from `web/`:

```powershell
$env:CI='true'; npm test -- --runInBand ModelRoutesTable.test.js
npm run build
```

Expected: tests PASS and build reports `Compiled successfully`.

- [ ] **Step 4: Inspect the final diff and workspace state**

```powershell
git diff --check HEAD~2..HEAD
git status --short
git log -4 --oneline
```

Expected: no whitespace errors; only the user's pre-existing untracked `error.txt`, `req.txt`, and `resp.txt` remain.
