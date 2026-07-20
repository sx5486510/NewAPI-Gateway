# CPA Auth Credential Status Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Gateway 的 CPA 认证文件页面安全展示最近刷新、Access Token 到期状态和 Refresh Token 三态诊断信息。

**Architecture:** 新建纯函数模块，把官方认证 JSON 立即压缩为不含 Token 的派生元数据；`CPAAuthFiles` 继续复用 CPA 官方列表和下载接口，以四并发异步加载详情并用请求代次隔离迟到响应。Refresh Token 状态在渲染时组合下载元数据、CPA 列表状态和当前会话真实额度错误，因此 401 可以立即把对应文件标为“疑似失效”，而不执行 OAuth refresh。

**Tech Stack:** React 18 hooks、Axios 封装 `API`、Jest/jsdom、Create React App、现有 `mapWithConcurrency`。

---

## 文件结构

- Create: `web/src/components/cpaAuthStatus.js`：解析 CPA 官方认证 JSON、判断到期状态、识别 401/unauthorized 证据并生成 Refresh Token 状态；只返回非敏感数据。
- Create: `web/src/components/cpaAuthStatus.test.js`：纯函数边界、安全性和状态优先级测试。
- Modify: `web/src/components/CPAAuthFiles.js`：并发读取认证详情、隔离旧请求、渲染认证状态并组合额度 401 证据。
- Modify: `web/src/components/CPAAuthFiles.test.js`：为列表/下载接口建立可区分的 mock，覆盖渲染、错误隔离、并发、旧请求和 DOM 不泄密。

### Task 1: 凭据状态纯函数

**Files:**
- Create: `web/src/components/cpaAuthStatus.js`
- Create: `web/src/components/cpaAuthStatus.test.js`

- [ ] **Step 1: 写失败的解析和状态测试**

创建 `web/src/components/cpaAuthStatus.test.js`：

```js
import {
  getRefreshTokenStatus,
  parseAuthCredentialMetadata,
} from './cpaAuthStatus';

describe('CPA auth credential status', () => {
  const now = Date.parse('2026-07-20T08:00:00Z');

  test.each([
    ['2026-07-20T08:00:01Z', 'valid'],
    ['2026-07-20T08:00:00Z', 'expired'],
    ['2026-07-19T08:00:00Z', 'expired'],
    ['', 'unknown'],
    ['not-a-date', 'unknown'],
  ])('classifies official expired value %p', (expired, expected) => {
    expect(
      parseAuthCredentialMetadata(
        JSON.stringify({ expired, refresh_token: 'refresh-secret' }),
        now
      ).accessStatus
    ).toBe(expected);
  });

  test.each([
    [{}, false],
    [{ refresh_token: '' }, false],
    [{ refresh_token: '   ' }, false],
    [{ refresh_token: 'refresh-secret' }, true],
  ])('detects refresh token presence without returning it', (input, expected) => {
    const result = parseAuthCredentialMetadata(JSON.stringify(input), now);
    expect(result.hasRefreshToken).toBe(expected);
    expect(JSON.stringify(result)).not.toContain('refresh-secret');
  });

  test('normalizes an expiry without retaining credential secrets', () => {
    const result = parseAuthCredentialMetadata(
      JSON.stringify({
        expired: '2026-07-20T09:00:00+01:00',
        access_token: 'access-secret',
        refresh_token: 'refresh-secret',
        id_token: 'id-secret',
      }),
      now
    );

    expect(result).toEqual({
      accessStatus: 'expired',
      expiresAt: '2026-07-20T08:00:00.000Z',
      hasRefreshToken: true,
    });
    expect(JSON.stringify(result)).not.toMatch(/access-secret|refresh-secret|id-secret/);
  });

  test.each(['401 denied', 'HTTP 401', 'Unauthorized', 'unauthorised', '未授权']) (
    'marks explicit unauthorized evidence as suspected invalid: %s',
    (evidence) => {
      expect(
        getRefreshTokenStatus(
          { hasRefreshToken: true },
          { file: { status_message: evidence } }
        )
      ).toBe('suspected_invalid');
    }
  );

  test('uses only quota errors as unauthorized evidence', () => {
    expect(
      getRefreshTokenStatus(
        { hasRefreshToken: true },
        { quotaState: { status: 'error', error: '401 denied' } }
      )
    ).toBe('suspected_invalid');
    expect(
      getRefreshTokenStatus(
        { hasRefreshToken: true },
        { quotaState: { status: 'success', error: '401 historical text' } }
      )
    ).toBe('unverified');
    expect(
      getRefreshTokenStatus(
        { hasRefreshToken: true },
        { quotaState: { status: 'error', error: '502 request failed' } }
      )
    ).toBe('unverified');
  });

  test('missing refresh token takes priority over unauthorized evidence', () => {
    expect(
      getRefreshTokenStatus(
        { hasRefreshToken: false },
        { file: { status: 'unauthorized' } }
      )
    ).toBe('missing');
  });

  test.each(['not json', '[]', 'null'])('rejects invalid auth JSON: %s', (text) => {
    expect(() => parseAuthCredentialMetadata(text, now)).toThrow(
      '认证文件格式无效'
    );
  });
});
```

- [ ] **Step 2: 运行测试并确认因模块缺失而失败**

Run:

```powershell
cd web
npm test -- --watchAll=false --runInBand src/components/cpaAuthStatus.test.js
```

Expected: FAIL，Jest 报告无法解析 `./cpaAuthStatus`。

- [ ] **Step 3: 实现最小的纯函数模块**

创建 `web/src/components/cpaAuthStatus.js`：

```js
const UNAUTHORIZED_PATTERN = /(^|\D)401(\D|$)|unauthori[sz]ed|未授权/i;

const isRecord = (value) =>
  value !== null && typeof value === 'object' && !Array.isArray(value);

export const parseAuthCredentialMetadata = (text, now = Date.now()) => {
  let auth;
  try {
    auth = typeof text === 'string' ? JSON.parse(text) : text;
  } catch {
    throw new Error('认证文件格式无效');
  }
  if (!isRecord(auth)) {
    throw new Error('认证文件格式无效');
  }

  const rawExpiry =
    typeof auth.expired === 'string' ? auth.expired.trim() : '';
  const expiryTime = rawExpiry ? Date.parse(rawExpiry) : Number.NaN;
  const expiresAt = Number.isNaN(expiryTime)
    ? null
    : new Date(expiryTime).toISOString();
  const accessStatus = Number.isNaN(expiryTime)
    ? 'unknown'
    : expiryTime <= now
      ? 'expired'
      : 'valid';

  return {
    accessStatus,
    expiresAt,
    hasRefreshToken:
      typeof auth.refresh_token === 'string' &&
      auth.refresh_token.trim().length > 0,
  };
};

const hasUnauthorizedEvidence = (...values) =>
  values.some(
    (value) =>
      typeof value === 'string' && UNAUTHORIZED_PATTERN.test(value.trim())
  );

export const getRefreshTokenStatus = (
  metadata,
  { file = {}, quotaState } = {}
) => {
  if (!metadata?.hasRefreshToken) return 'missing';
  const quotaError =
    quotaState?.status === 'error' ? quotaState.error : undefined;
  return hasUnauthorizedEvidence(
    file.status,
    file.status_message,
    quotaError
  )
    ? 'suspected_invalid'
    : 'unverified';
};
```

- [ ] **Step 4: 运行纯函数测试并确认通过**

Run:

```powershell
cd web
npm test -- --watchAll=false --runInBand src/components/cpaAuthStatus.test.js
```

Expected: PASS，所有 `CPA auth credential status` 测试通过。

- [ ] **Step 5: 格式化并提交纯函数单元**

Run:

```powershell
cd web
npx prettier --write src/components/cpaAuthStatus.js src/components/cpaAuthStatus.test.js
cd ..
git add web/src/components/cpaAuthStatus.js web/src/components/cpaAuthStatus.test.js
git commit -m "feat(cpa): derive safe credential status"
```

Expected: 只提交新增模块及其测试。

### Task 2: 并发加载和展示认证详情

**Files:**
- Modify: `web/src/components/CPAAuthFiles.js`
- Modify: `web/src/components/CPAAuthFiles.test.js`

- [ ] **Step 1: 先让现有测试 mock 区分列表和下载请求**

在 `mockAuthFiles` 后加入以下 helper，并把普通成功用例中的 `helpers.API.get.mockResolvedValue({ data: ... })` 改为 `mockCPAAuthGet({ listData: ... })`。加载中和列表失败测试继续显式覆盖列表请求。所有 GET 次数断言改为只统计对应路径：

```js
const defaultCredential = JSON.stringify({
  expired: '2099-07-20T08:00:00Z',
  refresh_token: 'default-refresh-secret',
});

const mockCPAAuthGet = ({
  listData = mockAuthFiles,
  downloads = {},
  downloadHandler,
} = {}) => {
  helpers.API.get.mockImplementation((path, config = {}) => {
    if (path === '/v0/management/auth-files') {
      return Promise.resolve({ data: listData });
    }
    if (path === '/v0/management/auth-files/download') {
      const name = config.params?.name;
      if (downloadHandler) return downloadHandler(name);
      const result = downloads[name];
      if (result instanceof Error) return Promise.reject(result);
      return Promise.resolve({ data: result ?? defaultCredential });
    }
    return Promise.reject(new Error(`unexpected GET ${path}`));
  });
};

const getCallsFor = (path) =>
  helpers.API.get.mock.calls.filter(([requestedPath]) => requestedPath === path);
```

例如把列表次数断言改为：

```js
expect(getCallsFor('/v0/management/auth-files')).toHaveLength(1);
```

或刷新列表后：

```js
expect(getCallsFor('/v0/management/auth-files')).toHaveLength(2);
```

- [ ] **Step 2: 为状态渲染和官方接口参数写失败测试**

给 fixture 的 Claude 文件增加 `last_refresh: '2026-07-20T07:00:00Z'`，新增测试：

```js
test('loads and displays safe credential status from official CPA endpoints', async () => {
  mockCPAAuthGet({
    listData: { files: [mockAuthFiles.files[0]] },
    downloads: {
      'claude@example.com.json': JSON.stringify({
        expired: '2020-01-01T00:00:00Z',
        access_token: 'access-secret-value',
        refresh_token: 'refresh-secret-value',
      }),
    },
  });

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });

  const row = container.querySelector(
    '[data-auth-file="claude@example.com.json"]'
  );
  expect(row.textContent).toContain('最近刷新');
  expect(row.textContent).toContain('2026');
  expect(row.textContent).toContain('Access Token');
  expect(row.textContent).toContain('已过期');
  expect(row.textContent).toContain('Refresh Token');
  expect(row.textContent).toContain('存在但未验证');
  expect(row.textContent).not.toMatch(/access-secret-value|refresh-secret-value/);
  expect(
    getCallsFor('/v0/management/auth-files/download')[0][1]
  ).toMatchObject({
    params: { name: 'claude@example.com.json' },
    responseType: 'text',
  });
});
```

- [ ] **Step 3: 运行组件测试并确认缺少认证状态 UI**

Run:

```powershell
cd web
npm test -- --watchAll=false --runInBand src/components/CPAAuthFiles.test.js
```

Expected: 新测试 FAIL，行内没有“最近刷新”和 Token 状态。

- [ ] **Step 4: 在组件中增加代次隔离的四并发详情加载**

在 imports 中加入：

```js
import {
  getRefreshTokenStatus,
  parseAuthCredentialMetadata,
} from './cpaAuthStatus';
```

在组件 state/refs 中加入：

```js
const [credentialStates, setCredentialStates] = useState({});
const credentialLoadGenerationRef = useRef(0);
```

把现有首次 `fetchAuthFiles()` effect 移到 `downloadAuthFileText` 定义之后，并紧接着加入详情 effect：

```js
useEffect(() => {
  fetchAuthFiles();
}, [fetchAuthFiles]);

useEffect(() => {
  const generation = ++credentialLoadGenerationRef.current;
  const files = authFiles.filter((file) => file.name);
  const loadingStates = Object.fromEntries(
    files.map((file) => [file.name, { status: 'loading' }])
  );
  setCredentialStates(loadingStates);

  if (!files.length) return undefined;
  let cancelled = false;

  const loadDetails = async () => {
    await mapWithConcurrency(files, 4, async (file) => {
      let nextState;
      try {
        const text = await downloadAuthFileText(file.name);
        nextState = {
          status: 'success',
          metadata: parseAuthCredentialMetadata(text),
        };
      } catch (error) {
        nextState = {
          status: 'error',
          error:
            error instanceof Error && error.message === '认证文件格式无效'
              ? error.message
              : '无法读取认证详情',
        };
      }
      if (cancelled || generation !== credentialLoadGenerationRef.current) {
        return;
      }
      setCredentialStates((current) => ({
        ...current,
        [file.name]: nextState,
      }));
    });
  };

  loadDetails();
  return () => {
    cancelled = true;
  };
}, [authFiles, downloadAuthFileText]);
```

- [ ] **Step 5: 渲染紧凑的三字段状态区域**

在 `renderQuotaInfo` 之前增加：

```js
const formatCredentialTime = (value) => {
  if (!value) return '未知';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '未知' : date.toLocaleString('zh-CN');
};

const renderCredentialInfo = (file) => {
  const detail = credentialStates[file.name];
  const itemStyle = { fontSize: '0.8rem', color: 'var(--text-secondary)' };
  if (!detail || detail.status === 'loading') {
    return (
      <div data-credential-status={file.name} style={itemStyle}>
        最近刷新: {formatCredentialTime(file.last_refresh)} · 认证详情读取中...
      </div>
    );
  }
  if (detail.status === 'error') {
    return (
      <div data-credential-status={file.name} style={itemStyle}>
        最近刷新: {formatCredentialTime(file.last_refresh)} · {detail.error}
      </div>
    );
  }

  const accessText =
    detail.metadata.accessStatus === 'valid'
      ? `有效至 ${formatCredentialTime(detail.metadata.expiresAt)}`
      : detail.metadata.accessStatus === 'expired'
        ? `已过期（${formatCredentialTime(detail.metadata.expiresAt)}）`
        : '未知';
  const refreshStatus = getRefreshTokenStatus(detail.metadata, {
    file,
    quotaState: quotaStates[quotaKey(file)],
  });
  const refreshText = {
    missing: '缺失',
    unverified: '存在但未验证',
    suspected_invalid: '疑似失效',
  }[refreshStatus];

  return (
    <div
      data-credential-status={file.name}
      style={{ display: 'flex', flexWrap: 'wrap', gap: '0.35rem 1rem' }}
    >
      <span style={itemStyle}>
        最近刷新: {formatCredentialTime(file.last_refresh)}
      </span>
      <span style={itemStyle}>Access Token: {accessText}</span>
      <span style={itemStyle}>Refresh Token: {refreshText}</span>
    </div>
  );
};
```

在文件名/邮箱/备注区域之后、`renderQuotaInfo(file)` 之前调用：

```jsx
{renderCredentialInfo(file)}
```

- [ ] **Step 6: 运行组件和纯函数测试并确认通过**

Run:

```powershell
cd web
npm test -- --watchAll=false --runInBand src/components/cpaAuthStatus.test.js src/components/CPAAuthFiles.test.js
```

Expected: 两个测试文件 PASS；原有列表 GET 次数断言已按路径调整。

- [ ] **Step 7: 格式化并提交加载/渲染单元**

Run:

```powershell
cd web
npx prettier --write src/components/CPAAuthFiles.js src/components/CPAAuthFiles.test.js
cd ..
git add web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
git commit -m "feat(cpa): display credential diagnostics"
```

### Task 3: 错误隔离、并发与旧请求回归

**Files:**
- Modify: `web/src/components/CPAAuthFiles.test.js`

- [ ] **Step 1: 写下载失败和 JSON 错误隔离测试**

新增两个文件的测试数据，一个下载成功、一个失败或返回非法 JSON。断言成功行仍显示 Token 状态，失败行分别显示“无法读取认证详情”或“认证文件格式无效”，并断言 DOM 不包含响应中的任何 Token 值：

```js
test('isolates credential download and parse failures per auth file', async () => {
  const files = [mockAuthFiles.files[0], mockAuthFiles.files[2]];
  mockCPAAuthGet({
    listData: { files },
    downloads: {
      'claude@example.com.json': JSON.stringify({
        expired: '2099-01-01T00:00:00Z',
        refresh_token: 'safe-row-secret',
      }),
      'antigravity.json': new Error('proxy response included hidden-secret'),
    },
  });

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });

  expect(
    container.querySelector('[data-auth-file="claude@example.com.json"]')
      .textContent
  ).toContain('存在但未验证');
  expect(
    container.querySelector('[data-auth-file="antigravity.json"]')
      .textContent
  ).toContain('无法读取认证详情');
  expect(container.textContent).not.toMatch(/safe-row-secret|hidden-secret/);
});
```

为非法 JSON 增加：

```js
test('reports invalid credential JSON without exposing its response', async () => {
  mockCPAAuthGet({
    listData: { files: [mockAuthFiles.files[0]] },
    downloads: {
      'claude@example.com.json': 'not-json-with-hidden-secret',
    },
  });

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });

  const row = container.querySelector(
    '[data-auth-file="claude@example.com.json"]'
  );
  expect(row.textContent).toContain('认证文件格式无效');
  expect(container.textContent).not.toContain('not-json-with-hidden-secret');
});
```

- [ ] **Step 2: 写四并发上限测试**

新增 9 个认证文件，下载 handler 用 20ms 定时器记录 active/peak；列表请求不计入 active。等待全部详情完成后断言 `peak === 4` 且下载调用为 9 次：

```js
test('limits credential detail downloads to four workers', async () => {
  const files = Array.from({ length: 9 }, (_, index) => ({
    name: `auth-${index}.json`,
    type: 'codex',
    auth_index: index + 1,
  }));
  let active = 0;
  let peak = 0;
  mockCPAAuthGet({
    listData: { files },
    downloadHandler: () =>
      new Promise((resolve) => {
        active += 1;
        peak = Math.max(peak, active);
        setTimeout(() => {
          active -= 1;
          resolve({ data: defaultCredential });
        }, 20);
      }),
  });

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await new Promise((resolve) => setTimeout(resolve, 250));
  });

  expect(peak).toBe(4);
  expect(getCallsFor('/v0/management/auth-files/download')).toHaveLength(9);
});
```

- [ ] **Step 3: 写刷新列表后忽略旧下载结果测试**

使用可控 Promise：第一次列表返回同名文件且其下载保持 pending；点击“刷新列表”后第二次下载立即返回 `refresh_token: ''`，等待 UI 显示“缺失”，再 resolve 第一次下载为有 Refresh Token，断言 UI 仍为“缺失”：

```js
test('ignores an old credential download after refreshing the list', async () => {
  const file = mockAuthFiles.files[0];
  let downloadCount = 0;
  let resolveFirstDownload;
  helpers.API.get.mockImplementation((path) => {
    if (path === '/v0/management/auth-files') {
      return Promise.resolve({ data: { files: [{ ...file }] } });
    }
    if (path === '/v0/management/auth-files/download') {
      downloadCount += 1;
      if (downloadCount === 1) {
        return new Promise((resolve) => {
          resolveFirstDownload = resolve;
        });
      }
      return Promise.resolve({
        data: JSON.stringify({
          expired: '2099-01-01T00:00:00Z',
          refresh_token: '',
        }),
      });
    }
    return Promise.reject(new Error(`unexpected GET ${path}`));
  });

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });

  await act(async () => {
    findButton(container, '刷新列表').click();
    await waitForUI();
  });
  const row = container.querySelector(
    '[data-auth-file="claude@example.com.json"]'
  );
  expect(row.textContent).toContain('Refresh Token: 缺失');

  await act(async () => {
    resolveFirstDownload({
      data: JSON.stringify({
        expired: '2099-01-01T00:00:00Z',
        refresh_token: 'old-refresh-secret',
      }),
    });
    await waitForUI();
  });

  expect(row.textContent).toContain('Refresh Token: 缺失');
  expect(row.textContent).not.toContain('old-refresh-secret');
  expect(getCallsFor('/v0/management/auth-files')).toHaveLength(2);
  expect(getCallsFor('/v0/management/auth-files/download')).toHaveLength(2);
});
```

- [ ] **Step 4: 运行新测试并修正暴露的最小缺陷**

Run:

```powershell
cd web
npm test -- --watchAll=false --runInBand src/components/CPAAuthFiles.test.js
```

Expected: PASS；旧请求的结果没有覆盖新列表状态。

- [ ] **Step 5: 提交生命周期回归测试和必要修正**

Run:

```powershell
git add web/src/components/CPAAuthFiles.test.js web/src/components/CPAAuthFiles.js
git commit -m "test(cpa): cover credential detail lifecycle"
```

### Task 4: 401 联动和最终验证

**Files:**
- Modify: `web/src/components/CPAAuthFiles.test.js`

- [ ] **Step 1: 扩展现有批量真实额度部分失败测试**

现有测试已经让 `auth_index === '6'` 的 Grok 返回内层 `status_code: 401`。在点击“获取全部真实额度”并等待后增加：

```js
const grokRow = container.querySelector('[data-auth-file="grok.json"]');
expect(grokRow.textContent).toContain('401 denied');
expect(grokRow.textContent).toContain('Refresh Token: 疑似失效');
```

再新增一个 502 用例，断言错误正常显示，但 Refresh Token 仍为“存在但未验证”：

```js
test('does not mark refresh token invalid for a non-401 quota error', async () => {
  const grok = mockAuthFiles.files.find((file) => file.name === 'grok.json');
  mockCPAAuthGet({ listData: { files: [grok] } });
  helpers.API.post.mockResolvedValue({
    data: { status_code: 502, body: { error: 'upstream failed' } },
  });

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });

  const row = container.querySelector('[data-auth-file="grok.json"]');
  await act(async () => {
    findButton(row, '获取真实额度').click();
    await waitForUI();
  });

  expect(row.textContent).toContain('502 upstream failed');
  expect(row.textContent).toContain('Refresh Token: 存在但未验证');
});
```

- [ ] **Step 2: 运行相关测试确认 401 联动通过**

Run:

```powershell
cd web
npm test -- --watchAll=false --runInBand src/components/cpaAuthStatus.test.js src/components/CPAAuthFiles.test.js src/components/cpaQuota.test.js
```

Expected: 三个测试文件 PASS；401 只影响对应认证行。

- [ ] **Step 3: 运行完整前端测试套件**

Run:

```powershell
cd web
npm test -- --watchAll=false --runInBand
```

Expected: 所有测试套件 PASS，无未处理 Promise 或 React `act` 警告。

- [ ] **Step 4: 运行生产构建和 diff 检查**

Run:

```powershell
cd web
npm run build
cd ..
git diff --check HEAD~3..HEAD
git status --short
```

Expected: production build 成功；`git diff --check` 无输出；状态中只保留任务开始前已经存在的无关未跟踪文件。

- [ ] **Step 5: 提交 401 联动测试（如有改动）**

Run:

```powershell
git add web/src/components/CPAAuthFiles.test.js
git commit -m "test(cpa): verify unauthorized credential status"
```

Expected: 提交不包含 build 产物或无关文件。
