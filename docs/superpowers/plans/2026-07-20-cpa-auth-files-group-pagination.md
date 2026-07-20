# CPA Auth Files Group Pagination Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add independent pagination to every CPA auth-file provider group, defaulting to 50 rows with 20/50/100 selectors, while loading credential details only for visible rows.

**Architecture:** Keep the official `/v0/management/auth-files` list contract unchanged and paginate its grouped result in `CPAAuthFiles`. Store page and page-size values by provider key, reuse the existing `ui/Pagination`, and maintain a file-name credential cache so page navigation only downloads uncached visible credentials.

**Tech Stack:** React 18 hooks, Jest and React DOM test utilities, existing Gateway UI components, Prettier, Create React App.

---

## File Structure

- Modify `web/src/components/CPAAuthFiles.js`: own group pagination state, derive visible slices, render group controls, clamp invalid pages, and load visible credential details through a cache.
- Modify `web/src/components/CPAAuthFiles.test.js`: cover independent pages, independent page sizes, visible-only detail loading, cache reuse, and page clamping after list changes.

No backend file changes. The official CPA list endpoint continues returning the complete list.

### Task 1: Independent Group Pagination UI

**Files:**
- Modify: `web/src/components/CPAAuthFiles.test.js:15-152`
- Modify: `web/src/components/CPAAuthFiles.js:1-45,384-404,565-920`

- [ ] **Step 1: Add a test-data helper and failing default-pagination test**

Add this helper after `mockAuthFiles` in `CPAAuthFiles.test.js`:

```javascript
const buildAuthFiles = (type, count) =>
  Array.from({ length: count }, (_, index) => ({
    name: `${type}-${String(index + 1).padStart(3, '0')}.json`,
    type,
    auth_index: index + 1,
    disabled: false,
  }));
```

Add this test after `fetches and displays auth files on mount`:

```javascript
test('paginates every group independently with a default of 50 files', async () => {
  mockCPAAuthGet({
    listData: {
      files: [...buildAuthFiles('claude', 55), ...buildAuthFiles('codex', 55)],
    },
  });

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });

  const claudeGroup = container.querySelector('[data-auth-group="claude"]');
  const codexGroup = container.querySelector('[data-auth-group="codex"]');
  expect(claudeGroup.querySelectorAll('[data-auth-file]')).toHaveLength(50);
  expect(codexGroup.querySelectorAll('[data-auth-file]')).toHaveLength(50);
  expect(claudeGroup.querySelector('select')).toHaveValue('50');
  expect(codexGroup.querySelector('select')).toHaveValue('50');

  await act(async () => {
    findButton(claudeGroup, '2').click();
  });

  expect(claudeGroup.querySelectorAll('[data-auth-file]')).toHaveLength(5);
  expect(codexGroup.querySelectorAll('[data-auth-file]')).toHaveLength(50);
  expect(claudeGroup.textContent).toContain('claude-055.json');
  expect(codexGroup.textContent).toContain('codex-001.json');
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run from `web`:

```powershell
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js -t "paginates every group independently"
```

Expected: FAIL because `[data-auth-group]` and per-group pagination controls do not exist and all 55 files are rendered.

- [ ] **Step 3: Add a failing test for independent page-size selectors**

Add:

```javascript
test('changes one group page size without changing another group', async () => {
  mockCPAAuthGet({
    listData: {
      files: [...buildAuthFiles('claude', 55), ...buildAuthFiles('codex', 55)],
    },
  });

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });

  const claudeGroup = container.querySelector('[data-auth-group="claude"]');
  const codexGroup = container.querySelector('[data-auth-group="codex"]');
  const claudePageSize = claudeGroup.querySelector('select');
  expect(Array.from(claudePageSize.options).map((option) => option.value)).toEqual([
    '20',
    '50',
    '100',
  ]);

  await act(async () => {
    findButton(claudeGroup, '2').click();
  });
  expect(claudeGroup.textContent).toContain('claude-055.json');

  await act(async () => {
    claudePageSize.value = '20';
    claudePageSize.dispatchEvent(new Event('change', { bubbles: true }));
  });

  expect(claudePageSize).toHaveValue('20');
  expect(codexGroup.querySelector('select')).toHaveValue('50');
  expect(claudeGroup.querySelectorAll('[data-auth-file]')).toHaveLength(20);
  expect(codexGroup.querySelectorAll('[data-auth-file]')).toHaveLength(50);
  expect(claudeGroup.textContent).toContain('claude-001.json');
  expect(claudeGroup.textContent).not.toContain('claude-055.json');
});
```

- [ ] **Step 4: Run both pagination tests and verify RED**

Run from `web`:

```powershell
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js -t "group"
```

Expected: the two new tests FAIL because group-specific page and page-size state are absent.

- [ ] **Step 5: Add group pagination constants and pure calculations**

In `CPAAuthFiles.js`, add `useMemo` to the React import, import `Pagination`, and move the existing type labels and grouping function above the component. Add these pagination rules:

```javascript
import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import Pagination from './ui/Pagination';

const AUTH_FILE_PAGE_SIZES = [20, 50, 100];
const DEFAULT_AUTH_FILE_PAGE_SIZE = 50;

const typeLabels = {
  antigravity: { name: 'Antigravity', color: '#006064' },
  claude: { name: 'Claude', color: '#C4612F' },
  codex: { name: 'Codex', color: '#10B981' },
  kimi: { name: 'Kimi', color: '#0560CF' },
  grok: { name: 'Grok', color: '#3B82F6' },
  other: { name: '其他', color: '#6B7280' },
};

const groupFilesByType = (files) => {
  const groups = Object.fromEntries(
    Object.keys(typeLabels).map((key) => [key, []])
  );
  files.forEach((file) => {
    const provider = getQuotaProvider(file);
    if (provider === 'xai') groups.grok.push(file);
    else if (provider && groups[provider]) groups[provider].push(file);
    else groups.other.push(file);
  });
  return groups;
};

const normalizePageSize = (value) =>
  AUTH_FILE_PAGE_SIZES.includes(value) ? value : DEFAULT_AUTH_FILE_PAGE_SIZE;

const paginateFiles = (files, requestedPage, requestedPageSize) => {
  const pageSize = normalizePageSize(requestedPageSize);
  const totalPages = Math.max(1, Math.ceil(files.length / pageSize));
  const page = Math.min(Math.max(requestedPage || 1, 1), totalPages);
  return {
    files: files.slice((page - 1) * pageSize, page * pageSize),
    page,
    pageSize,
    totalPages,
  };
};
```

Remove the old in-component `groupFilesByType` and `typeLabels` declarations.

- [ ] **Step 6: Derive per-group pages and render controls**

Add state inside `CPAAuthFiles`:

```javascript
const [pageByGroup, setPageByGroup] = useState({});
const [pageSizeByGroup, setPageSizeByGroup] = useState({});

const groupedFiles = useMemo(() => groupFilesByType(authFiles), [authFiles]);
const paginatedGroups = useMemo(
  () =>
    Object.fromEntries(
      Object.entries(groupedFiles).map(([key, files]) => [
        key,
        paginateFiles(files, pageByGroup[key], pageSizeByGroup[key]),
      ])
    ),
  [groupedFiles, pageByGroup, pageSizeByGroup]
);
```

Remove the old `const groupedFiles = groupFilesByType(authFiles);` near the return. In each group render, replace `const files = groupedFiles[key]` with:

```javascript
const group = paginatedGroups[key];
const files = groupedFiles[key];
const visibleFiles = group.files;
```

Add `data-auth-group={key}` to the group wrapper, replace `files.map` with `visibleFiles.map`, and append this footer after the file-list container:

```javascript
<div
  style={{
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    gap: '1rem',
    marginTop: '0.75rem',
    flexWrap: 'wrap',
  }}
>
  <span style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
    共 {files.length} 条
  </span>
  <label style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
    <span style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
      每页
    </span>
    <select
      aria-label={`${name} 每页条数`}
      value={group.pageSize}
      onChange={(event) => {
        const pageSize = Number(event.target.value);
        setPageSizeByGroup((current) => ({ ...current, [key]: pageSize }));
        setPageByGroup((current) => ({ ...current, [key]: 1 }));
      }}
    >
      {AUTH_FILE_PAGE_SIZES.map((pageSize) => (
        <option key={pageSize} value={pageSize}>
          {pageSize}
        </option>
      ))}
    </select>
    <span style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
      条
    </span>
  </label>
  <Pagination
    activePage={group.page}
    totalPages={group.totalPages}
    onPageChange={(_, { activePage }) =>
      setPageByGroup((current) => ({ ...current, [key]: activePage }))
    }
  />
</div>
```

- [ ] **Step 7: Format and verify GREEN**

Run from `web`:

```powershell
npx prettier --write src/components/CPAAuthFiles.js src/components/CPAAuthFiles.test.js
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js -t "group"
```

Expected: both new pagination tests PASS.

- [ ] **Step 8: Commit independent pagination**

```powershell
git add web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
git commit -m "feat(cpa): paginate auth file groups independently"
```

### Task 2: Visible Credential Loading and Cache Reuse

**Files:**
- Modify: `web/src/components/CPAAuthFiles.test.js:238-332`
- Modify: `web/src/components/CPAAuthFiles.js:39-137`

- [ ] **Step 1: Add a failing visible-only download test**

Add after the existing credential concurrency test:

```javascript
test('downloads credential details only for files visible on group pages', async () => {
  const files = buildAuthFiles('codex', 55);
  mockCPAAuthGet({ listData: { files } });

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForCondition(
      () => getCallsFor('/v0/management/auth-files/download').length === 50,
      '50 visible credential downloads'
    );
  });

  expect(getCallsFor('/v0/management/auth-files/download')).toHaveLength(50);
  expect(
    getCallsFor('/v0/management/auth-files/download').map(
      ([, config]) => config.params.name
    )
  ).not.toContain('codex-055.json');
});
```

- [ ] **Step 2: Run the test and verify RED**

Run from `web`:

```powershell
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js -t "only for files visible"
```

Expected: FAIL because the existing effect downloads details for all 55 auth files.

- [ ] **Step 3: Add a failing cache-reuse test**

Add:

```javascript
test('loads a new page once and reuses cached credential details', async () => {
  const files = buildAuthFiles('codex', 55);
  mockCPAAuthGet({ listData: { files } });

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForCondition(
      () => getCallsFor('/v0/management/auth-files/download').length === 50,
      'first page credential downloads'
    );
  });

  const codexGroup = container.querySelector('[data-auth-group="codex"]');
  await act(async () => {
    findButton(codexGroup, '2').click();
    await waitForCondition(
      () => getCallsFor('/v0/management/auth-files/download').length === 55,
      'second page credential downloads'
    );
  });
  await act(async () => {
    findButton(codexGroup, '1').click();
    await waitForUI();
  });

  expect(getCallsFor('/v0/management/auth-files/download')).toHaveLength(55);
});
```

- [ ] **Step 4: Derive the visible credential queue**

After `paginatedGroups`, add:

```javascript
const visibleAuthFiles = useMemo(
  () =>
    Object.values(paginatedGroups).flatMap((group) => group.files),
  [paginatedGroups]
);
```

Add a completed-result cache next to the existing refs:

```javascript
const credentialCacheRef = useRef({});
```

- [ ] **Step 5: Replace the all-files credential effect with visible cached loading**

Replace the effect that currently filters `authFiles` and resets every credential state with:

```javascript
useEffect(() => {
  const generation = ++credentialLoadGenerationRef.current;
  const validNames = new Set(
    authFiles.filter((file) => file.name).map((file) => file.name)
  );
  Object.keys(credentialCacheRef.current).forEach((name) => {
    if (!validNames.has(name)) delete credentialCacheRef.current[name];
  });

  const files = visibleAuthFiles.filter(
    (file) => file.name && !credentialCacheRef.current[file.name]
  );
  setCredentialStates(() =>
    Object.fromEntries(
      visibleAuthFiles
        .filter((file) => file.name)
        .map((file) => [
          file.name,
          credentialCacheRef.current[file.name] || { status: 'loading' },
        ])
    )
  );

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
      credentialCacheRef.current[file.name] = nextState;
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
}, [authFiles, downloadAuthFileText, visibleAuthFiles]);
```

This caches only completed safe metadata/error states. Cancelled in-flight loads remain uncached and are eligible for a later visible-page request.

- [ ] **Step 6: Format and verify GREEN plus existing async safeguards**

Run from `web`:

```powershell
npx prettier --write src/components/CPAAuthFiles.js src/components/CPAAuthFiles.test.js
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js -t "credential|old credential|visible|cached"
```

Expected: visible-only, cache-reuse, four-worker concurrency, failure isolation, and stale-response tests PASS.

- [ ] **Step 7: Commit visible credential loading**

```powershell
git add web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
git commit -m "perf(cpa): load visible auth credentials only"
```

### Task 3: Page Clamping and Full Regression Verification

**Files:**
- Modify: `web/src/components/CPAAuthFiles.test.js:140-380`
- Modify: `web/src/components/CPAAuthFiles.js:35-160`

- [ ] **Step 1: Add a failing page-clamping test**

Add after the pagination tests:

```javascript
test('persists the clamped group page after refreshed data grows again', async () => {
  let listRequest = 0;
  helpers.API.get.mockImplementation((path) => {
    if (path === '/v0/management/auth-files') {
      listRequest += 1;
      return Promise.resolve({
        data: {
          files:
            listRequest === 2
              ? buildAuthFiles('codex', 10)
              : buildAuthFiles('codex', 55),
        },
      });
    }
    if (path === '/v0/management/auth-files/download') {
      return Promise.resolve({ data: defaultCredential });
    }
    return Promise.reject(new Error(`unexpected GET ${path}`));
  });

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });
  const codexGroup = container.querySelector('[data-auth-group="codex"]');
  await act(async () => {
    findButton(codexGroup, '2').click();
  });
  expect(codexGroup.textContent).toContain('codex-055.json');

  await act(async () => {
    findButton(container, '刷新列表').click();
    await waitForUI();
  });

  expect(codexGroup.querySelectorAll('[data-auth-file]')).toHaveLength(10);
  expect(codexGroup.textContent).toContain('codex-001.json');
  expect(codexGroup.textContent).not.toContain('codex-055.json');

  await act(async () => {
    findButton(container, '刷新列表').click();
    await waitForUI();
  });

  expect(codexGroup.querySelectorAll('[data-auth-file]')).toHaveLength(50);
  expect(codexGroup.textContent).toContain('codex-001.json');
  expect(codexGroup.textContent).not.toContain('codex-055.json');
});
```

- [ ] **Step 2: Run the clamping test and verify RED**

Run from `web`:

```powershell
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js -t "persists the clamped group page"
```

Expected: FAIL after the list grows back to 55 files because the derived view temporarily clamps to page 1 but the stored page remains 2.

- [ ] **Step 3: Reconcile stored pages when group data or page size changes**

Add this effect after `paginatedGroups` is derived:

```javascript
useEffect(() => {
  setPageByGroup((current) => {
    let changed = false;
    const next = { ...current };
    Object.entries(paginatedGroups).forEach(([key, group]) => {
      if (next[key] !== group.page) {
        next[key] = group.page;
        changed = true;
      }
    });
    return changed ? next : current;
  });
}, [paginatedGroups]);
```

The derived slice already clamps synchronously, and this effect brings stored state back to the same valid page without changing other groups.

- [ ] **Step 4: Run the complete CPA auth-file component suite**

Run from `web`:

```powershell
npx prettier --write src/components/CPAAuthFiles.js src/components/CPAAuthFiles.test.js
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js
```

Expected: all `CPAAuthFiles` tests PASS with no unhandled React errors.

- [ ] **Step 5: Run related helper tests**

Run from `web`:

```powershell
npm test -- --runInBand --watchAll=false src/components/cpaAuthStatus.test.js src/components/cpaQuota.test.js src/helpers/async-pool.test.js src/components/CPAAuthFiles.test.js
```

Expected: all selected suites PASS.

- [ ] **Step 6: Build the frontend**

Run from `web`:

```powershell
npm run build
```

Expected: Create React App reports a successful production build. Existing repository-wide warnings must be recorded; new lint errors must be fixed.

- [ ] **Step 7: Inspect the scoped diff**

Run from the repository root:

```powershell
git diff --check
git diff -- web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
```

Expected: `git diff --check` exits successfully; the diff contains only the approved pagination, visible-detail cache, and tests.

- [ ] **Step 8: Commit clamping and regression coverage**

```powershell
git add web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
git commit -m "test(cpa): cover auth group pagination boundaries"
```

## Completion Criteria

- Every nonempty CPA auth-file group has its own page and 20/50/100 page-size selector.
- All groups start at 50 rows per page.
- Page and page-size changes stay local to one group.
- A shrinking list cannot leave a group on an empty out-of-range page.
- Only visible credential details are downloaded, completed results are reused, and stale requests cannot overwrite current state.
- Focused tests, related tests, formatting, diff checks, and the production frontend build pass.
