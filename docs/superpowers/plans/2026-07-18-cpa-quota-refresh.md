# CPA Quota Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the broken three-provider quota button with a tested Gateway implementation that follows the embedded CPA frontend for Antigravity, Claude, Codex, Kimi, and Grok.

**Architecture:** Keep the dedicated Go proxy because `api-call` is not a CPA configuration mutation, but cover and correct its transparent response contract. Move provider detection, CPA request orchestration, fallback rules, and response normalization into a pure JavaScript module; keep `CPAAuthFiles` responsible only for per-file async state and rendering.

**Tech Stack:** Go `net/http`/`httptest`, React 18, Axios, Jest through `react-scripts`, existing Gateway UI components.

---

## File Structure

- Modify `service/cpa/management_proxy_test.go`: add the API-call proxy contract regression test.
- Modify `service/cpa/management_proxy.go`: preserve the upstream response content type and remove unused quota-list code introduced by the broken change.
- Create `web/src/components/cpaQuota.js`: pure provider registry, field extraction, request orchestration, fallback, and normalized quota parsing.
- Create `web/src/components/cpaQuota.test.js`: unit tests for all five provider adapters and shared error behavior.
- Modify `web/src/components/CPAAuthFiles.js`: replace hard-coded requests with per-file quota state, normalized rendering, and fetch-all behavior.
- Modify `web/src/components/CPAAuthFiles.test.js`: cover single refresh, fetch-all, partial failure, and duplicate-click behavior.

### Normalized frontend contract

`fetchCPAQuota(file, deps)` returns:

```js
{
  provider: 'claude',
  plan: 'Max',
  groups: [
    {
      id: 'limits',
      label: 'Claude 额度',
      items: [
        {
          id: 'five-hour',
          label: '5 小时限额',
          remainingPercent: 74,
          resetAt: '2026-07-18T12:00:00Z',
          detail: '',
        },
      ],
    },
  ],
  meta: [{ label: '额外用量', value: '$3.40 / $20.00' }],
  warnings: [],
}
```

All providers use the same outer shape. Provider-specific information belongs in `groups`, `meta`, or `warnings`; `CPAAuthFiles` must not inspect raw provider payloads.

---

### Task 1: Lock Down the Management API-Call Proxy

**Files:**
- Modify: `service/cpa/management_proxy_test.go`
- Modify: `service/cpa/management_proxy.go:249-346`

- [ ] **Step 1: Write the failing proxy contract test**

Add a test that captures the upstream method, path, body, and Authorization; returns a non-default content type and status; and counts persistence/sync calls:

```go
func TestManagementProxyAPICallForwardsWithoutPersisting(t *testing.T) {
	var capturedBody string
	var capturedAuthorization string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v0/management/api-call" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.String())
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		capturedBody = string(body)
		capturedAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/problem+json")
		w.Header().Set("X-CPA-Request", "quota")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `{"status_code":403,"body":{"error":"denied"}}`)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	provider := &fakeLeaseProvider{target: upstreamURL, password: "runtime-secret"}
	persistCalls := &atomic.Int32{}
	syncCalls := &atomic.Int32{}
	proxy := NewManagementProxy(provider, &mockSnapshotStore{persistFunc: func() error {
		persistCalls.Add(1)
		return nil
	}}, func() { syncCalls.Add(1) })

	payload := `{"authIndex":"7","method":"GET","url":"https://example.test/usage"}`
	req := httptest.NewRequest(http.MethodPost, "/v0/management/api-call", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer browser-placeholder")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if capturedBody != payload {
		t.Fatalf("body = %q, want %q", capturedBody, payload)
	}
	if capturedAuthorization != "Bearer runtime-secret" {
		t.Fatalf("authorization = %q", capturedAuthorization)
	}
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMultiStatus)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content type = %q", got)
	}
	if rec.Header().Get("X-CPA-Request") != "quota" {
		t.Fatal("missing upstream response header")
	}
	if persistCalls.Load() != 0 || syncCalls.Load() != 0 {
		t.Fatalf("persist=%d sync=%d, want zero", persistCalls.Load(), syncCalls.Load())
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run:

```powershell
$env:GOCACHE='E:\NewAPI-Gateway\.gocache'
go test ./service/cpa -run TestManagementProxyAPICallForwardsWithoutPersisting -count=1
```

Expected: FAIL because `handleAuthFileQuota` forces `application/json` before copying the upstream `Content-Type`.

- [ ] **Step 3: Make the minimal proxy fix**

Replace the response-header block with upstream-first copying and a fallback only when CPA did not send a content type:

```go
for key, values := range resp.Header {
	if key == "Content-Length" || key == "Transfer-Encoding" {
		continue
	}
	for _, value := range values {
		w.Header().Add(key, value)
	}
}
if w.Header().Get("Content-Type") == "" {
	w.Header().Set("Content-Type", "application/json")
}
w.WriteHeader(resp.StatusCode)
_, _ = io.Copy(w, resp.Body)
```

Remove the unused `authFileNameOnly` and `listAuthFiles` declarations. They are not part of quota refresh and duplicate the existing auth-file listing path.

- [ ] **Step 4: Verify GREEN and the CPA package**

Run:

```powershell
go test ./service/cpa -run TestManagementProxyAPICallForwardsWithoutPersisting -count=1
go test ./service/cpa -count=1
```

Expected: both PASS.

- [ ] **Step 5: Commit**

```powershell
git add service/cpa/management_proxy.go service/cpa/management_proxy_test.go
git commit -m "fix(cpa): preserve api-call proxy responses"
```

---

### Task 2: Build and Test the Shared Quota Contract

**Files:**
- Create: `web/src/components/cpaQuota.js`
- Create: `web/src/components/cpaQuota.test.js`

- [ ] **Step 1: Write failing tests for provider and credential normalization**

Create tests for exact CPA matching and coercion:

```js
import {
  getQuotaProvider,
  getAuthIndex,
  isAuthFileDisabled,
  parseApiCallPayload,
} from './cpaQuota';

test.each([
  [{ provider: 'antigravity' }, 'antigravity'],
  [{ type: 'claude' }, 'claude'],
  [{ provider: 'codex' }, 'codex'],
  [{ type: 'kimi' }, 'kimi'],
  [{ provider: 'grok' }, 'xai'],
  [{ provider: 'x-ai' }, 'xai'],
  [{ provider: 'unknown' }, null],
])('normalizes CPA provider %p', (file, expected) => {
  expect(getQuotaProvider(file)).toBe(expected);
});

test('normalizes auth_index and disabled values', () => {
  expect(getAuthIndex({ auth_index: 17 })).toBe('17');
  expect(getAuthIndex({ authIndex: ' 18 ' })).toBe('18');
  expect(isAuthFileDisabled({ disabled: 'true' })).toBe(true);
  expect(isAuthFileDisabled({ disabled: 0 })).toBe(false);
});

test('parses the CPA inner response and rejects a provider error', () => {
  expect(parseApiCallPayload({ status_code: 200, body: '{"ok":true}' }).body).toEqual({ ok: true });
  expect(() => parseApiCallPayload({ status_code: 403, body: '{"error":"denied"}' }))
    .toThrow('403 denied');
});
```

- [ ] **Step 2: Run the test and verify RED**

Run from `web`:

```powershell
$env:CI='true'
npm test -- --runInBand --watchAll=false src/components/cpaQuota.test.js
```

Expected: FAIL because `cpaQuota.js` does not exist.

- [ ] **Step 3: Implement the shared contract**

Add these public functions and keep all unsafe parsing behind them:

```js
const supportedProviders = new Set(['antigravity', 'claude', 'codex', 'kimi', 'xai']);

const stringValue = (value) => {
  if (typeof value === 'string') return value.trim() || null;
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return null;
};

export const getQuotaProvider = (file) => {
  const raw = stringValue(file?.provider ?? file?.type)?.toLowerCase().replace(/_/g, '-');
  const provider = raw === 'grok' || raw === 'x-ai' ? 'xai' : raw;
  return supportedProviders.has(provider) ? provider : null;
};

export const getAuthIndex = (file) => stringValue(file?.auth_index ?? file?.authIndex);

export const isAuthFileDisabled = (file) => {
  const value = file?.disabled;
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  return typeof value === 'string' && value.trim().toLowerCase() === 'true';
};

const parseBody = (body) => {
  if (body == null || typeof body === 'object') return body;
  const text = String(body).trim();
  if (!text) return null;
  try { return JSON.parse(text); } catch { return text; }
};

const responseMessage = (statusCode, body) => {
  const message = body?.error?.message ?? body?.error ?? body?.message ??
    (typeof body === 'string' ? body : 'Request failed');
  return `${statusCode || ''} ${message}`.trim();
};

export const parseApiCallPayload = (payload) => {
  const statusCode = Number(payload?.status_code ?? 0);
  const body = parseBody(payload?.body);
  if (statusCode < 200 || statusCode >= 300) {
    const error = new Error(responseMessage(statusCode, body));
    error.status = statusCode;
    throw error;
  }
  return { statusCode, header: payload?.header ?? {}, body };
};
```

- [ ] **Step 4: Verify GREEN**

Run the same Jest command. Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add web/src/components/cpaQuota.js web/src/components/cpaQuota.test.js
git commit -m "test(cpa): define quota provider contract"
```

---

### Task 3: Add the Five CPA Provider Adapters

**Files:**
- Modify: `web/src/components/cpaQuota.js`
- Modify: `web/src/components/cpaQuota.test.js`

- [ ] **Step 1: Add failing request-contract tests**

Use an injected `post` function so tests assert real payloads without coupling the module to Axios:

```js
const ok = (body) => Promise.resolve({ data: { status_code: 200, header: {}, body } });

test('Claude uses OAuth usage and profile endpoints', async () => {
  const post = jest.fn()
    .mockImplementationOnce(() => ok({ five_hour: { utilization: 0.25, resets_at: '2026-07-18T12:00:00Z' } }))
    .mockImplementationOnce(() => ok({ organization: { organization_type: 'claude_max' } }));

  const quota = await fetchCPAQuota({ type: 'claude', auth_index: 3 }, { post });

  expect(post.mock.calls.map((call) => call[1].url)).toEqual([
    'https://api.anthropic.com/api/oauth/usage',
    'https://api.anthropic.com/api/oauth/profile',
  ]);
  expect(post.mock.calls[0][1]).toMatchObject({
    authIndex: '3',
    method: 'GET',
    header: {
      Authorization: 'Bearer $TOKEN$',
      'Content-Type': 'application/json',
      'anthropic-beta': 'oauth-2025-04-20',
    },
  });
  expect(quota.groups[0].items[0].remainingPercent).toBe(75);
});

test('Codex sends Chatgpt-Account-Id and tolerates reset-credit failure', async () => {
  const post = jest.fn()
    .mockImplementationOnce(() => ok({ plan_type: 'plus', rate_limit: { primary_window: { used_percent: 10 } } }))
    .mockImplementationOnce(() => Promise.reject(new Error('reset endpoint unavailable')));
  const idToken = { 'https://api.openai.com/auth': { chatgpt_account_id: 'acct-7' } };

  const quota = await fetchCPAQuota({ type: 'codex', auth_index: '4', id_token: idToken }, { post });

  expect(post.mock.calls[0][1].header['Chatgpt-Account-Id']).toBe('acct-7');
  expect(post.mock.calls[0][1].url).toBe('https://chatgpt.com/backend-api/wham/usage');
  expect(post.mock.calls[1][1].url).toBe('https://chatgpt.com/backend-api/wham/rate-limit-reset-credits');
  expect(quota.warnings).toEqual(['reset endpoint unavailable']);
});

test('Kimi calls coding usages', async () => {
  const post = jest.fn(() => ok({ usages: [{ limit: 100, used: 25, reset_at: 1784376000 }] }));
  await fetchCPAQuota({ provider: 'kimi', auth_index: 5 }, { post });
  expect(post.mock.calls[0][1]).toMatchObject({
    authIndex: '5', method: 'GET', url: 'https://api.kimi.com/coding/v1/usages',
  });
});

test('Grok merges credits and billing and sends x-userid', async () => {
  const post = jest.fn()
    .mockImplementationOnce(() => ok({ config: { monthlyLimitCents: 2000 } }))
    .mockImplementationOnce(() => ok({ config: { includedUsedCents: 500 } }));
  const quota = await fetchCPAQuota({ type: 'grok', auth_index: 6, user: { id: 'user-6' } }, { post });
  expect(post.mock.calls.map((call) => call[1].url)).toEqual([
    'https://cli-chat-proxy.grok.com/v1/billing?format=credits',
    'https://cli-chat-proxy.grok.com/v1/billing',
  ]);
  expect(post.mock.calls[0][1].header['x-userid']).toBe('user-6');
  expect(quota.meta).toContainEqual({ label: '月度积分', value: '$15.00 / $20.00' });
});

test('Antigravity downloads a missing project id and falls back endpoints', async () => {
  let quotaCalls = 0;
  const post = jest.fn((path, request) => {
    if (request.url.includes('loadCodeAssist')) {
      return ok({ currentTier: { id: 'g1-pro-tier', name: 'Pro' } });
    }
    quotaCalls += 1;
    if (quotaCalls === 1) {
      return Promise.resolve({ data: { status_code: 404, body: '{"error":"missing"}' } });
    }
    return ok({ groups: [{ displayName: 'Gemini Models', buckets: [
      { bucketId: 'weekly', displayName: 'Weekly', remainingFraction: 0.6 },
    ] }] });
  });
  const downloadText = jest.fn(() => Promise.resolve('{"installed":{"project_id":"project-9"}}'));

  const quota = await fetchCPAQuota(
    { provider: 'antigravity', auth_index: 9, name: 'ag.json' },
    { post, downloadText },
  );

  expect(downloadText).toHaveBeenCalledWith('ag.json');
  const quotaRequests = post.mock.calls.map((call) => call[1])
    .filter((request) => request.url.includes('retrieveUserQuotaSummary'));
  expect(quotaRequests[0].data).toBe('{"project":"project-9"}');
  expect(quotaRequests).toHaveLength(2);
  expect(quota.groups[0].items[0].remainingPercent).toBe(60);
});
```

- [ ] **Step 2: Run the adapter tests and verify RED**

Run the focused Jest command. Expected: FAIL because `fetchCPAQuota` and adapters do not exist.

- [ ] **Step 3: Add the exact CPA endpoint and header constants**

Add constants matching the embedded CPA frontend:

```js
const API_CALL_PATH = '/v0/management/api-call';
const CLAUDE_USAGE_URL = 'https://api.anthropic.com/api/oauth/usage';
const CLAUDE_PROFILE_URL = 'https://api.anthropic.com/api/oauth/profile';
const CODEX_USAGE_URL = 'https://chatgpt.com/backend-api/wham/usage';
const CODEX_RESET_CREDITS_URL = 'https://chatgpt.com/backend-api/wham/rate-limit-reset-credits';
const KIMI_USAGE_URL = 'https://api.kimi.com/coding/v1/usages';
const GROK_CREDITS_URL = 'https://cli-chat-proxy.grok.com/v1/billing?format=credits';
const GROK_BILLING_URL = 'https://cli-chat-proxy.grok.com/v1/billing';
const ANTIGRAVITY_QUOTA_URLS = [
  'https://daily-cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary',
  'https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:retrieveUserQuotaSummary',
  'https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary',
];
const ANTIGRAVITY_TIER_URL = 'https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist';
```

- [ ] **Step 4: Implement one adapter per provider and a registry dispatcher**

Use this dispatcher and dependency boundary:

```js
const callCPA = async (post, request, options) => {
  const response = await post(API_CALL_PATH, request, options);
  return parseApiCallPayload(response?.data);
};

const providerAdapters = {
  antigravity: fetchAntigravityQuota,
  claude: fetchClaudeQuota,
  codex: fetchCodexQuota,
  kimi: fetchKimiQuota,
  xai: fetchGrokQuota,
};

export const fetchCPAQuota = async (file, { post, downloadText }) => {
  const provider = getQuotaProvider(file);
  if (!provider) throw new Error('不支持的供应商类型');
  const authIndex = getAuthIndex(file);
  if (!authIndex) throw new Error('认证文件缺少 auth_index');
  return providerAdapters[provider]({ file, authIndex, post, downloadText });
};
```

Implement adapter behavior exactly as follows:

- Claude: `Promise.allSettled` usage/profile; usage is mandatory; profile is optional; map `utilization` to `100 - utilization` and `resets_at` to `resetAt`.
- Codex: extract account ID from top-level/metadata/attributes ID tokens, including JWT strings and decoded objects; query usage and reset credits concurrently; map every primary/secondary/code-review/additional window exposed by CPA; reset-credit failure goes to `warnings`.
- Kimi: accept `usages`, `limits`, or array-shaped usage payloads; calculate `remainingPercent = (limit - used) / limit * 100`; preserve reset hints.
- Grok: extract user ID from top-level/metadata/attributes OAuth and user objects; run both billing calls with `Promise.allSettled`; fail only if both fail; merge configs before normalizing weekly limits, monthly cents, product usage, plan, and pay-as-you-go values.
- Antigravity: inspect top-level/metadata/attributes project fields, then `downloadText`; query tier without making it mandatory; try quota endpoints sequentially and continue only on failed call/non-2xx; map groups/buckets and `remainingFraction * 100`.

Use small named helpers (`objectValue`, `numberValue`, `percentValue`, `decodeJWTPayload`, `extractCodexAccountId`, `extractAntigravityProjectId`, `extractGrokUserId`, `quotaItem`) so each extraction rule is directly unit-testable. Clamp percentages to `0..100` and preserve unknown values as `null` rather than inventing zero.

- [ ] **Step 5: Add parser fixtures for every provider**

Extend tests with at least one success fixture and one malformed/empty fixture per provider. Assert the normalized `plan`, `groups`, `items`, `meta`, and `warnings`, not raw implementation details.

- [ ] **Step 6: Verify GREEN**

Run:

```powershell
$env:CI='true'
npm test -- --runInBand --watchAll=false src/components/cpaQuota.test.js
```

Expected: PASS with no console warnings.

- [ ] **Step 7: Commit**

```powershell
git add web/src/components/cpaQuota.js web/src/components/cpaQuota.test.js
git commit -m "feat(cpa): follow native quota providers"
```

---

### Task 4: Integrate Per-File Quota State Into CPAAuthFiles

**Files:**
- Modify: `web/src/components/CPAAuthFiles.js`
- Modify: `web/src/components/CPAAuthFiles.test.js`

- [ ] **Step 1: Replace quota test fixtures with supported files carrying auth indexes**

Extend `mockAuthFiles.files` to include Antigravity, Claude, Codex, Kimi, Grok, disabled, and unsupported entries. Every supported enabled fixture must have a stable `auth_index`.

- [ ] **Step 2: Write failing single-refresh component test**

Mock the Claude usage/profile calls and click the Claude file's quota button by accessible label:

```js
test('refreshes one quota and renders the returned limits without reloading files', async () => {
  helpers.API.get.mockResolvedValueOnce({ data: mockAuthFiles });
  helpers.API.post
    .mockResolvedValueOnce({ data: { status_code: 200, body: { five_hour: { utilization: 20 } } } })
    .mockResolvedValueOnce({ data: { status_code: 200, body: { organization: { organization_type: 'claude_max' } } } });

  await renderLoadedAuthFiles();
  const button = container.querySelector('[aria-label="刷新 claude@example.com.json 的额度"]');

  await act(async () => {
    button.click();
    await flushPromises();
  });

  expect(container.textContent).toContain('5 小时限额');
  expect(container.textContent).toContain('80%');
  expect(helpers.API.get).toHaveBeenCalledTimes(1);
});
```

Expected RED: the old handler uses one wrong request and reloads auth files.

- [ ] **Step 3: Write failing fetch-all and duplicate-click tests**

Assert that:

- “获取全部” calls quota only for supported enabled files.
- a rejected provider call renders an error next to that file while another file renders success.
- clicking a file twice while its request is unresolved starts only one provider workflow.
- disabled and unsupported files have no quota refresh button.

- [ ] **Step 4: Add per-file state and injected download helper**

At component scope add:

```js
const [quotaStates, setQuotaStates] = useState({});
const quotaInFlightRef = useRef(new Set());

const downloadAuthFileText = useCallback(async (name) => {
  const response = await API.get('/v0/management/auth-files/download', {
    params: { name },
    responseType: 'text',
  });
  return typeof response.data === 'string' ? response.data : JSON.stringify(response.data ?? {});
}, []);
```

- [ ] **Step 5: Replace `handleRefreshQuota` with provider-module orchestration**

Use one state update per transition and always clear the in-flight guard:

```js
const handleRefreshQuota = useCallback(async (file) => {
  const key = file.name || String(file.auth_index ?? file.authIndex ?? '');
  if (!key || quotaInFlightRef.current.has(key)) return;
  quotaInFlightRef.current.add(key);
  setQuotaStates((current) => ({ ...current, [key]: { status: 'loading' } }));
  try {
    const quota = await fetchCPAQuota(file, {
      post: API.post.bind(API),
      downloadText: downloadAuthFileText,
    });
    setQuotaStates((current) => ({ ...current, [key]: { status: 'success', quota } }));
  } catch (error) {
    setQuotaStates((current) => ({
      ...current,
      [key]: { status: 'error', error: error instanceof Error ? error.message : '未知错误' },
    }));
  } finally {
    quotaInFlightRef.current.delete(key);
  }
}, [downloadAuthFileText]);
```

Do not call `fetchAuthFiles` from quota refresh.

- [ ] **Step 6: Add fetch-all behavior**

Filter with `getQuotaProvider(file)` and `!isAuthFileDisabled(file)`, then use `Promise.allSettled(files.map(handleRefreshQuota))`. Disable the fetch-all button only while the batch is active; per-file guards remain authoritative.

- [ ] **Step 7: Render normalized quota data**

Replace `renderQuotaInfo(file)` with a renderer that receives `quotaStates[key]`:

- `idle`/missing: “点击刷新额度”。
- `loading`: spinner plus “正在加载额度…”。
- `error`: `AlertCircle` plus the exact stored message.
- `success`: plan/meta first, then every normalized group/item; show remaining percentage, a small progress bar, optional detail/reset time, and warning messages.

Add `aria-label={`刷新 ${file.name} 的额度`}` to each per-file quota button. Preserve existing auth-file actions and layout.

- [ ] **Step 8: Verify GREEN and run both component suites**

Run:

```powershell
$env:CI='true'
npm test -- --runInBand --watchAll=false src/components/cpaQuota.test.js src/components/CPAAuthFiles.test.js
```

Expected: PASS.

- [ ] **Step 9: Commit**

```powershell
git add web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
git commit -m "feat(cpa): display native quota refresh results"
```

---

### Task 5: Full Verification and Handoff

**Files:**
- Verify only; modify a task-owned file only if verification exposes a regression.

- [ ] **Step 1: Run focused frontend tests**

```powershell
Set-Location E:\NewAPI-Gateway\web
$env:CI='true'
npm test -- --runInBand --watchAll=false src/components/cpaQuota.test.js src/components/CPAAuthFiles.test.js
```

Expected: all tests PASS.

- [ ] **Step 2: Run all CPA backend tests**

```powershell
Set-Location E:\NewAPI-Gateway
$env:GOCACHE='E:\NewAPI-Gateway\.gocache'
go test ./service/cpa -count=1
```

Expected: PASS.

- [ ] **Step 3: Run the frontend production build**

```powershell
Set-Location E:\NewAPI-Gateway\web
npm run build
```

Expected: exit code 0. Existing repository warnings may be reported, but no new errors or warnings may originate from the changed files.

- [ ] **Step 4: Inspect the final diff and working tree**

```powershell
Set-Location E:\NewAPI-Gateway
git diff --check
git status --short
git diff HEAD~3 -- service/cpa/management_proxy.go service/cpa/management_proxy_test.go web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js web/src/components/cpaQuota.js web/src/components/cpaQuota.test.js
```

Confirm that unrelated untracked user files remain untouched.

- [ ] **Step 5: Final verification commit if needed**

Only if verification required a task-scoped correction:

```powershell
git add service/cpa/management_proxy.go service/cpa/management_proxy_test.go web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js web/src/components/cpaQuota.js web/src/components/cpaQuota.test.js
git commit -m "test(cpa): verify quota refresh integration"
```
