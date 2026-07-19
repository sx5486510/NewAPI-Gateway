# CPA Quota Actions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add separate CPA-native real-quota and cooldown-reset actions while bounding bulk traffic and hardening the management proxy.

**Architecture:** Keep the official CPA routes public through the existing Root-only management proxy. Treat `/api-call` and `/reset-quota` as runtime-only operations, keep provider request templates in `cpaQuota.js`, and add small frontend helpers for management-response validation and bounded concurrency.

**Tech Stack:** Go 1.x `net/http`, React 18, Axios 0.27, Jest/React DOM tests

---

### Task 1: Preserve CPA runtime-only semantics and proxy boundaries

**Files:**
- Modify: `service/cpa/management_proxy.go`
- Test: `service/cpa/management_proxy_test.go`

- [ ] **Step 1: Write failing proxy tests**

Add tests which POST `/v0/management/reset-quota` and assert the upstream body is unchanged while persistence and sync counters remain zero; POST an `api-call` body of `maxAPICallRequestBody + 1` bytes and assert 413 without an upstream request; assert the default transport response-header timeout is 65 seconds; and return `Connection`/named hop-by-hop response headers from upstream and assert they are removed.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./service/cpa -run 'TestManagementProxy(ResetQuota|APICall)' -count=1`

Expected: reset-quota persistence, oversized-body, timeout, and response-header assertions fail for the current implementation.

- [ ] **Step 3: Implement runtime-only and bounded proxy behavior**

Introduce:

```go
const maxAPICallRequestBody int64 = 1 << 20

func isRuntimeOnlyManagementRequest(r *http.Request) bool {
	if r == nil || r.Method != http.MethodPost {
		return false
	}
	switch normalizePath(r.URL.Path) {
	case "/v0/management/api-call", "/v0/management/reset-quota":
		return true
	default:
		return false
	}
}
```

Exclude runtime-only requests from mutation persistence. Read `maxAPICallRequestBody + 1`, return `http.StatusRequestEntityTooLarge` with code `request_body_too_large` when exceeded, set `ResponseHeaderTimeout` to 65 seconds, map special-proxy transport failures to a stable message, and copy response headers only after removing standard and `Connection`-named hop-by-hop headers.

- [ ] **Step 4: Run tests and verify GREEN**

Run: `go test ./service/cpa -count=1`

Expected: PASS.

### Task 2: Make CPA frontend failures explicit and reject empty quota results

**Files:**
- Create: `web/src/helpers/cpa-management.js`
- Create: `web/src/helpers/cpa-management.test.js`
- Modify: `web/src/helpers/index.js`
- Modify: `web/src/components/cpaQuota.js`
- Test: `web/src/components/cpaQuota.test.js`

- [ ] **Step 1: Write failing response-validation tests**

Define tests for the desired helper:

```js
expect(() => requireCPASuccess({ data: { success: false, message: 'CPA offline' } }))
  .toThrow('CPA offline');
expect(requireCPASuccess({ status: 200, data: { status: 'ok' } }).data)
  .toEqual({ status: 'ok' });
```

Add a quota adapter test where the provider returns HTTP 200 with an empty quota payload and assert `fetchCPAQuota` rejects with an “额度响应为空” error.

- [ ] **Step 2: Run tests and verify RED**

Run: `$env:CI='true'; npm test -- --runInBand --watchAll=false --testMatch '**/src/**/*.[t]est.js'`

Expected: the new helper is missing and empty quota currently resolves.

- [ ] **Step 3: Implement response validation and final quota validation**

Create:

```js
export const requireCPASuccess = (response) => {
  if (response?.data?.success === false) {
    throw new Error(response.data.message || 'CPA 管理请求失败');
  }
  return response;
};
```

Export it from `helpers/index.js`. After awaiting a provider adapter in `fetchCPAQuota`, require at least one normalized quota item across all groups; otherwise throw `${provider} 额度响应为空`.

- [ ] **Step 4: Run tests and verify GREEN**

Run the same frontend Jest command.

Expected: PASS.

### Task 3: Add the cooldown-reset button and bound bulk quota concurrency

**Files:**
- Create: `web/src/helpers/async-pool.js`
- Create: `web/src/helpers/async-pool.test.js`
- Modify: `web/src/helpers/index.js`
- Modify: `web/src/components/CPAAuthFiles.js`
- Test: `web/src/components/CPAAuthFiles.test.js`

- [ ] **Step 1: Write failing helper and component tests**

Add an async-pool test with deferred workers, track peak active work, and assert `mapWithConcurrency(items, 4, worker)` never exceeds four while preserving result order.

Add component tests which click “重置冷却”, assert:

```js
expect(helpers.API.post).toHaveBeenCalledWith('/v0/management/reset-quota', {
  auth_index: '3',
});
```

Assert the list reloads after success, duplicate reset clicks produce one request, fake `{success:false,message}` responses render the supplied error, and bulk quota acquisition never starts more than four authentication workers concurrently.

- [ ] **Step 2: Run tests and verify RED**

Run: `$env:CI='true'; npm test -- --runInBand --watchAll=false --testMatch '**/src/**/*.[t]est.js'`

Expected: helper import/button queries fail and current `Promise.allSettled` exceeds four workers.

- [ ] **Step 3: Implement UI actions and bounded execution**

Create:

```js
export const mapWithConcurrency = async (items, limit, worker) => {
  const results = new Array(items.length);
  let nextIndex = 0;
  const runners = Array.from(
    { length: Math.min(Math.max(1, limit), items.length) },
    async () => {
      while (nextIndex < items.length) {
        const index = nextIndex++;
        try {
          results[index] = { status: 'fulfilled', value: await worker(items[index], index) };
        } catch (reason) {
          results[index] = { status: 'rejected', reason };
        }
      }
    }
  );
  await Promise.all(runners);
  return results;
};
```

In `CPAAuthFiles`, wrap CPA calls with `requireCPASuccess`, add per-auth reset in-flight state, POST the official reset route with string `auth_index`, reload the list after success, render text-labelled “获取真实额度” and “重置冷却” buttons, and replace unbounded `Promise.allSettled` with `mapWithConcurrency(files, 4, handleRefreshQuota)`.

- [ ] **Step 4: Run tests and verify GREEN**

Run the frontend Jest command.

Expected: PASS.

### Task 4: Full verification

**Files:**
- Verify all modified files

- [ ] **Step 1: Format changed sources**

Run: `gofmt -w service/cpa/management_proxy.go service/cpa/management_proxy_test.go`

Run: `npx prettier --write src/components/CPAAuthFiles.js src/components/CPAAuthFiles.test.js src/components/cpaQuota.js src/components/cpaQuota.test.js src/helpers/cpa-management.js src/helpers/cpa-management.test.js src/helpers/async-pool.js src/helpers/async-pool.test.js src/helpers/index.js`

- [ ] **Step 2: Run backend tests**

Run: `go test ./service/cpa -count=1`

Expected: PASS.

- [ ] **Step 3: Run frontend tests**

Run: `$env:CI='true'; npm test -- --runInBand --watchAll=false --testMatch '**/src/**/*.[t]est.js'`

Expected: all suites PASS; existing React Router future-flag warnings may remain.

- [ ] **Step 4: Build the frontend**

Run: `npm run build`

Expected: production build completes successfully; existing non-blocking warnings are reported separately.

- [ ] **Step 5: Inspect the final diff**

Run: `git status --short` and `git diff --check`.

Expected: only planned source, test, spec, and plan files are changed; no whitespace errors.
