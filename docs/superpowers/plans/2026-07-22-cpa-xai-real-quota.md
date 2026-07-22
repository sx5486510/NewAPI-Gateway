# CPA xAI Real Quota Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Return real Grok/xAI quota by supplying `x-userid` and refreshing expired OAuth tokens before billing calls.

**Architecture:** The frontend adapter reads the subject from credential JSON when the auth list omits it. The gateway management proxy recognizes only Grok billing calls, resolves the credential by `auth_index`, refreshes expired tokens through embedded CPA's own `api-call` transport, atomically persists the JSON, and injects the fresh token into the current request.

**Tech Stack:** React 18, Jest/react-scripts, Go 1.24, `net/http`, embedded CLIProxyAPI v7 management API.

---

## File Map

- Modify `web/src/components/cpaQuota.js` to resolve a missing xAI subject.
- Modify `web/src/components/cpaQuota.test.js` to cover production auth-list data.
- Create `service/cpa/xai_quota_auth.go` for xAI request preparation and refresh.
- Create `service/cpa/xai_quota_auth_test.go` for valid, expired, failure, traversal, and concurrency cases.
- Modify `service/cpa/management_proxy.go` to prepare xAI calls before forwarding.

### Task 1: Frontend xAI Subject Fallback

**Files:**
- Modify: `web/src/components/cpaQuota.js:780-828`
- Test: `web/src/components/cpaQuota.test.js:303-405`

- [ ] **Step 1: Write the failing production-shape test**

```javascript
test('Grok loads x-userid from the credential when the auth list omits it', async () => {
  const post = jest.fn((path, request) =>
    request.url.includes('format=credits')
      ? ok({ config: { creditUsagePercent: 25 } })
      : ok({ config: { monthlyLimit: { val: 2000 }, used: { val: 500 } } })
  );
  const downloadText = jest.fn(() =>
    JSON.stringify({ type: 'xai', sub: 'subject-from-auth-file' })
  );
  await fetchCPAQuota(
    { name: 'xai-production.json', provider: 'xai', auth_index: 'runtime-index' },
    { post, downloadText }
  );
  expect(downloadText).toHaveBeenCalledWith('xai-production.json');
  expect(post.mock.calls.map((call) => call[1].header['x-userid'])).toEqual([
    'subject-from-auth-file',
    'subject-from-auth-file',
  ]);
});
```

- [ ] **Step 2: Verify RED**

Run: `$env:CI='true'; npm test -- --watchAll=false --runInBand src/components/cpaQuota.test.js`

Expected: FAIL because `downloadText` is not called and `x-userid` is missing.

- [ ] **Step 3: Implement the minimal resolver**

```javascript
const resolveGrokUserId = async (file, downloadText) => {
  const direct = extractGrokUserId(file);
  if (direct || typeof downloadText !== 'function' || !file?.name) return direct;
  try {
    const credential = objectValue(
      JSON.parse(String(await downloadText(file.name)).trim())
    );
    return extractGrokUserId(credential);
  } catch {
    throw new Error('Grok credential file format is invalid');
  }
};
```

Change `fetchGrokQuota` to accept `downloadText`, await this resolver, and set `x-userid` before starting the two existing billing requests.

- [ ] **Step 4: Cover precedence and invalid JSON**

Add a test proving an existing `sub` skips credential download, and a test proving invalid downloaded JSON rejects before any billing request.

- [ ] **Step 5: Verify GREEN and commit**

Run: `$env:CI='true'; npm test -- --watchAll=false --runInBand src/components/cpaQuota.test.js`

Commit: `git add web/src/components/cpaQuota.js web/src/components/cpaQuota.test.js && git commit -m "fix(cpa): load xai subject for quota requests"`

### Task 2: Backend Valid-Token Preparation

**Files:**
- Create: `service/cpa/xai_quota_auth.go`
- Create: `service/cpa/xai_quota_auth_test.go`
- Modify: `service/cpa/management_proxy.go:262-327`

- [ ] **Step 1: Write a failing integration test**

Create `TestManagementProxyXAIQuotaUsesCredentialIdentity`. Use a temporary xAI JSON file with a future `expired`, `access_token: valid-access`, and `sub: subject-1`. A fake embedded CPA returns the file for `/v0/management/auth-files` and captures the forwarded `/v0/management/api-call` body.

```go
if got := forwarded.Header["Authorization"]; got != "Bearer valid-access" {
    t.Fatalf("Authorization = %q, want valid credential token", got)
}
if got := forwarded.Header["x-userid"]; got != "subject-1" {
    t.Fatalf("x-userid = %q, want subject-1", got)
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./service/cpa -run TestManagementProxyXAIQuotaUsesCredentialIdentity -count=1 -v`

Expected: FAIL because `$TOKEN$` is forwarded and `x-userid` is absent.

- [ ] **Step 3: Add xAI-only detection and types**

Define the public xAI CLI client ID `b1a00492-073a-47ea-816f-4c329264a828`, discovery URL `https://auth.x.ai/.well-known/openid-configuration`, and a five-minute refresh skew. Define typed views for auth-list entries and xAI token fields, while retaining the full credential as `map[string]any` for lossless persistence.

```go
func isXAIQuotaURL(raw string) bool {
    parsed, err := url.Parse(strings.TrimSpace(raw))
    return err == nil && parsed.Scheme == "https" &&
        strings.EqualFold(parsed.Hostname(), "cli-chat-proxy.grok.com") &&
        parsed.Path == "/v1/billing"
}
```

Parse API-call bodies through `map[string]json.RawMessage` so unknown fields survive, and recognize snake/camel/Pascal `auth_index` spellings.

- [ ] **Step 4: Resolve the credential safely**

Query embedded CPA's auth list, require provider `xai`, get `AuthDir` from the snapshot store, reject names whose `filepath.Base` differs or whose extension is not `.json`, and verify `filepath.Rel` remains inside the configured directory.

- [ ] **Step 5: Rewrite valid-token requests**

Replace `$TOKEN$` in the Authorization header with the file access token. Add `x-userid` from `sub` only when the request does not already have that header. Invoke the preparer in `handleAuthFileQuota`; return the original body unchanged for all other providers and URLs.

- [ ] **Step 6: Verify GREEN and commit**

Run: `go test ./service/cpa -run TestManagementProxyXAIQuotaUsesCredentialIdentity -count=1 -v`

Commit: `git add service/cpa/xai_quota_auth.go service/cpa/xai_quota_auth_test.go service/cpa/management_proxy.go && git commit -m "fix(cpa): prepare authenticated xai quota calls"`

### Task 3: Expired Token Refresh and Persistence

**Files:**
- Modify: `service/cpa/xai_quota_auth.go`
- Modify: `service/cpa/xai_quota_auth_test.go`
- Modify: `service/cpa/management_proxy.go`

- [ ] **Step 1: Write the failing expired-token test**

Create `TestManagementProxyXAIQuotaRefreshesExpiredCredential`. The fake embedded CPA returns nested status 200 and body fields `fresh-access`, `fresh-refresh`, `fresh-id`, `Bearer`, and `expires_in: 3600` for the internal refresh API-call. Assert the final billing call uses `Bearer fresh-access`, the file contains fresh token fields and a future expiry, and an unrelated `note` remains unchanged.

- [ ] **Step 2: Verify RED**

Run: `go test ./service/cpa -run TestManagementProxyXAIQuotaRefreshesExpiredCredential -count=1 -v`

Expected: FAIL because refresh is not implemented.

- [ ] **Step 3: Validate or discover the endpoint**

Require HTTPS and a hostname equal to `x.ai` or ending in `.x.ai`. If `token_endpoint` is absent, fetch OIDC discovery through embedded CPA's `api-call` so CPA's configured proxy is honored.

- [ ] **Step 4: Refresh through embedded CPA**

Post `grant_type=refresh_token`, the public xAI client ID, and the credential refresh token through embedded CPA's `/v0/management/api-call`, retaining `auth_index`. Reject a non-200 nested status, malformed response, or empty access token using the stable gateway error `auth_token_refresh_failed`; never include token response bodies in browser errors.

- [ ] **Step 5: Persist atomically and deduplicate**

Update only returned token fields plus `expired` and `last_refresh`. Marshal with indentation, write a same-directory temporary file at mode `0600`, `Sync`, close, and replace with `os.Rename`. Add `singleflight.Group` to `ManagementProxy`, keyed by `auth_index`, so concurrent weekly/monthly calls share one refresh.

- [ ] **Step 6: Cover failure, traversal, and concurrency**

Add these tests:

```go
func TestManagementProxyXAIQuotaRefreshFailureLeavesCredentialUntouched(t *testing.T)
func TestSecureAuthFilePathRejectsTraversal(t *testing.T)
func TestManagementProxyXAIQuotaDeduplicatesConcurrentRefresh(t *testing.T)
```

The failure test expects HTTP 502, code `auth_token_refresh_failed`, identical original file bytes, and no token values in the response.

- [ ] **Step 7: Verify GREEN and commit**

Run: `go test ./service/cpa -run 'TestManagementProxyXAIQuota|TestSecureAuthFilePath' -count=1 -v`

Commit: `git add service/cpa/xai_quota_auth.go service/cpa/xai_quota_auth_test.go service/cpa/management_proxy.go go.mod go.sum && git commit -m "fix(cpa): refresh expired xai quota credentials"`

### Task 4: Regression Verification

**Files:**
- Verify: `web/src/components/cpaQuota.test.js`
- Verify: `web/src/components/CPAAuthFiles.quota401.test.js`
- Verify: `service/cpa/management_proxy_test.go`
- Verify: `service/cpa/xai_quota_auth_test.go`

- [ ] **Step 1: Run focused frontend suites**

Run: `$env:CI='true'; npm test -- --watchAll=false --runInBand src/components/cpaQuota.test.js src/components/CPAAuthFiles.quota401.test.js`

Expected: zero failed suites.

- [ ] **Step 2: Format and run focused backend suites**

Run: `gofmt -w service/cpa/xai_quota_auth.go service/cpa/xai_quota_auth_test.go service/cpa/management_proxy.go`

Run: `go test ./service/cpa -run 'TestManagementProxy|TestSecureAuthFilePath' -count=1`

Expected: PASS.

- [ ] **Step 3: Run package verification**

Run: `go test ./service/cpa -count=1`

Run: `git diff --check`

Run: `git status --short`

Expected: diff check is clean. If the known Windows snapshot rollback test still fails, record its fresh output separately and do not claim the complete package passes. Preserve every unrelated untracked file.
