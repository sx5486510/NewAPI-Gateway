# CPA Auth Group Bulk Selection and Deletion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add independent per-group selection of the current auth-file page and deletion of each group's cross-page selected files.

**Architecture:** Keep selection and deletion-in-flight state inside `CPAAuthFiles`, keyed by the existing provider group keys. Reuse the existing group pagination slices and `DELETE /v0/management/auth-files?name=<file>` endpoint, running deletions through the existing four-worker concurrency helper and reconciling selection with the refreshed server list.

**Tech Stack:** React 18 hooks, existing Gateway UI components and Lucide icons, Axios helper wrappers, Jest with React DOM test utilities, Create React App, Prettier.

---

## File Structure

- Modify `web/src/components/CPAAuthFiles.js`: own selection and deletion state, render row/current-page controls, reset selection on filter changes, reconcile stale names, and execute per-group deletion.
- Modify `web/src/components/CPAAuthFiles.test.js`: cover current-page selection, cross-page retention, group isolation, filter resets, list reconciliation, cancellation, concurrency, success, and partial failure.

No backend files or API contracts change. The worktree already contains unrelated staged and unstaged changes in both target files; inspect both index and working-tree diffs before every commit and stage only this feature's hunks.

### Task 1: Per-Group Current-Page Selection

**Files:**
- Modify: `web/src/components/CPAAuthFiles.test.js:55-230`
- Modify: `web/src/components/CPAAuthFiles.js:119-170, 957-1255`

- [ ] **Step 1: Add failing tests for current-page selection, cross-page retention, and group isolation**

Add these helpers after `findButton` in `CPAAuthFiles.test.js`:

```javascript
const findGroupCheckbox = (group, groupName) =>
  group.querySelector(
    `input[aria-label="选择 ${groupName} 当前页认证文件"]`
  );

const findRowCheckbox = (group, fileName) =>
  group.querySelector(
    `input[aria-label="选择认证文件 ${fileName}"]`
  );
```

Add this test after the existing independent-pagination tests:

```javascript
test('selects only the current group page and retains selections across pages', async () => {
  mockCPAAuthGet({
    listData: {
      files: [...buildAuthFiles('claude', 55), ...buildAuthFiles('codex', 2)],
    },
  });

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });

  const claudeGroup = container.querySelector('[data-auth-group="claude"]');
  const codexGroup = container.querySelector('[data-auth-group="codex"]');
  const claudeSelectPage = findGroupCheckbox(claudeGroup, 'Claude');

  await act(async () => {
    claudeSelectPage.click();
  });

  expect(findButton(claudeGroup, '删除已选 (50)')).not.toBeNull();
  expect(findButton(codexGroup, '删除已选 (0)')).toBeDisabled();
  expect(findGroupCheckbox(codexGroup, 'Codex')).not.toBeChecked();

  await act(async () => {
    findButton(claudeGroup, '2').click();
  });

  expect(findGroupCheckbox(claudeGroup, 'Claude')).not.toBeChecked();
  expect(findButton(claudeGroup, '删除已选 (50)')).not.toBeNull();

  await act(async () => {
    findGroupCheckbox(claudeGroup, 'Claude').click();
  });

  expect(findButton(claudeGroup, '删除已选 (55)')).not.toBeNull();

  await act(async () => {
    findButton(claudeGroup, '1').click();
  });
  await act(async () => {
    findGroupCheckbox(claudeGroup, 'Claude').click();
  });

  expect(findButton(claudeGroup, '删除已选 (5)')).not.toBeNull();
  expect(findRowCheckbox(claudeGroup, 'claude-001.json')).not.toBeChecked();
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run from `web`:

```powershell
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js -t "retains selections across pages"
```

Expected: FAIL because the group and row selection controls do not exist.

- [ ] **Step 3: Add selection state and immutable update handlers**

In `CPAAuthFiles`, add state after the pagination state:

```javascript
const [selectedNamesByGroup, setSelectedNamesByGroup] = useState({});
```

Add these handlers after the pagination reconciliation effect:

```javascript
const handleToggleFileSelection = (groupKey, fileName, checked) => {
  setSelectedNamesByGroup((current) => {
    const nextNames = new Set(current[groupKey] || []);
    if (checked) nextNames.add(fileName);
    else nextNames.delete(fileName);
    return { ...current, [groupKey]: Array.from(nextNames) };
  });
};

const handleToggleVisibleSelection = (groupKey, files, checked) => {
  setSelectedNamesByGroup((current) => {
    const nextNames = new Set(current[groupKey] || []);
    files.forEach((file) => {
      if (!file.name) return;
      if (checked) nextNames.add(file.name);
      else nextNames.delete(file.name);
    });
    return { ...current, [groupKey]: Array.from(nextNames) };
  });
};
```

- [ ] **Step 4: Render each group's selector, count, and row checkboxes**

Inside the existing group render, immediately after `visibleFiles` is declared, derive:

```javascript
const selectedNames = selectedNamesByGroup[key] || [];
const selectedNameSet = new Set(selectedNames);
const visibleNames = visibleFiles
  .map((file) => file.name)
  .filter(Boolean);
const selectedVisibleCount = visibleNames.filter((fileName) =>
  selectedNameSet.has(fileName)
).length;
const allVisibleSelected =
  visibleNames.length > 0 && selectedVisibleCount === visibleNames.length;
const someVisibleSelected = selectedVisibleCount > 0 && !allVisibleSelected;
```

In the group title bar, before the existing quota refresh button, render:

```jsx
<label
  style={{
    display: 'inline-flex',
    alignItems: 'center',
    gap: '0.375rem',
    fontSize: '0.875rem',
    color: 'var(--text-secondary)',
  }}
>
  <input
    type='checkbox'
    aria-label={`选择 ${name} 当前页认证文件`}
    checked={allVisibleSelected}
    ref={(element) => {
      if (element) element.indeterminate = someVisibleSelected;
    }}
    onChange={(event) =>
      handleToggleVisibleSelection(key, visibleFiles, event.target.checked)
    }
  />
  选择当前页
</label>
<Button
  variant='danger'
  size='sm'
  disabled={selectedNames.length === 0}
  data-bulk-delete-group={key}
>
  <Trash2 size={14} style={{ marginRight: '0.375rem' }} />
  删除已选 ({selectedNames.length})
</Button>
```

Remove `marginLeft: 'auto'` from the quota refresh button and add an empty flexible spacer before it:

```jsx
<span style={{ flex: 1 }} />
```

At the start of each file row, before the file information container, render:

```jsx
<input
  type='checkbox'
  aria-label={`选择认证文件 ${file.name}`}
  checked={selectedNameSet.has(file.name)}
  onChange={(event) =>
    handleToggleFileSelection(key, file.name, event.target.checked)
  }
  style={{ flex: '0 0 auto', marginRight: '0.75rem' }}
/>
```

- [ ] **Step 5: Format and verify GREEN**

Run from `web`:

```powershell
npx prettier --write src/components/CPAAuthFiles.js src/components/CPAAuthFiles.test.js
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js -t "retains selections across pages"
```

Expected: PASS; the Claude count moves from 50 to 55 to 5 while the Codex group remains unselected.

- [ ] **Step 6: Commit only the selection hunks**

From the repository root, inspect both existing diff layers, then interactively stage only Task 1 hunks:

```powershell
git diff --cached -- web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
git diff -- web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
git add -p -- web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
git diff --cached --check
git commit -m "feat(cpa): select auth files per group page"
```

Expected: the commit contains only selection state, selection controls, and the new test. Preserve pre-existing 401-filter and unrelated worktree changes.

### Task 2: Filter Reset and Server-List Reconciliation

**Files:**
- Modify: `web/src/components/CPAAuthFiles.test.js:130-330`
- Modify: `web/src/components/CPAAuthFiles.js:140-210, 870-950`

- [ ] **Step 1: Add a failing test for page-size retention and filter reset**

Add:

```javascript
test('keeps selections for page-size changes but clears them and resets pages for filters', async () => {
  mockCPAAuthGet({ listData: { files: buildAuthFiles('claude', 55) } });

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });

  const group = container.querySelector('[data-auth-group="claude"]');
  await act(async () => {
    findButton(group, '2').click();
  });
  await act(async () => {
    findGroupCheckbox(group, 'Claude').click();
  });
  expect(findButton(group, '删除已选 (5)')).not.toBeNull();

  const pageSize = group.querySelector('select');
  await act(async () => {
    pageSize.value = '20';
    pageSize.dispatchEvent(new Event('change', { bubbles: true }));
  });
  expect(findButton(group, '删除已选 (5)')).not.toBeNull();
  expect(group.textContent).toContain('claude-001.json');

  const search = container.querySelector(
    'input[aria-label="搜索认证文件"]'
  );
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(
      window.HTMLInputElement.prototype,
      'value'
    ).set;
    setter.call(search, 'claude');
    search.dispatchEvent(new Event('input', { bubbles: true }));
  });

  expect(findButton(group, '删除已选 (0)')).toBeDisabled();
  expect(group.textContent).toContain('claude-001.json');
});
```

- [ ] **Step 2: Add a failing test for stale selection removal after refresh**

Add:

```javascript
test('removes selected names that disappear from the refreshed auth list', async () => {
  let listCalls = 0;
  const files = buildAuthFiles('codex', 2);
  helpers.API.get.mockImplementation((path) => {
    if (path === '/v0/management/auth-files') {
      listCalls += 1;
      return Promise.resolve({
        data: { files: listCalls === 1 ? files : [files[1]] },
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

  const group = container.querySelector('[data-auth-group="codex"]');
  await act(async () => {
    findRowCheckbox(group, 'codex-001.json').click();
    findButton(container, '刷新列表').click();
    await waitForUI();
  });

  expect(findButton(group, '删除已选 (0)')).toBeDisabled();
});
```

- [ ] **Step 3: Run both tests and verify RED**

Run from `web`:

```powershell
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js -t "page-size changes|disappear from the refreshed"
```

Expected: FAIL because filter changes do not clear selection and refreshed files do not prune selected names.

- [ ] **Step 4: Centralize filter changes and reset all group pages and selections**

Add next to the selection handlers:

```javascript
const handleFilterChange = (patch) => {
  setFilters((current) => ({ ...current, ...patch }));
  setSelectedNamesByGroup({});
  setPageByGroup({});
};

const handleClearFilters = () => {
  setFilters(DEFAULT_FILTERS);
  setSelectedNamesByGroup({});
  setPageByGroup({});
};
```

Replace the three inline filter setters with calls such as:

```jsx
onChange={(event) => handleFilterChange({ search: event.target.value })}
```

Use `{ type: event.target.value }` and `{ status: event.target.value }` for the selects. Replace the clear button callback with `handleClearFilters`.

- [ ] **Step 5: Reconcile selections whenever the server list changes**

Add after the selection handlers:

```javascript
useEffect(() => {
  const validNamesByGroup = Object.fromEntries(
    Object.entries(groupFilesByType(authFiles)).map(([key, files]) => [
      key,
      new Set(files.map((file) => file.name).filter(Boolean)),
    ])
  );

  setSelectedNamesByGroup((current) => {
    let changed = false;
    const next = {};
    Object.entries(current).forEach(([key, names]) => {
      const validNames = validNamesByGroup[key] || new Set();
      const kept = names.filter((name) => validNames.has(name));
      if (kept.length > 0) next[key] = kept;
      if (kept.length !== names.length) changed = true;
    });
    return changed ? next : current;
  });
}, [authFiles]);
```

- [ ] **Step 6: Format and verify GREEN plus existing filter tests**

Run from `web`:

```powershell
npx prettier --write src/components/CPAAuthFiles.js src/components/CPAAuthFiles.test.js
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js src/components/CPAAuthFiles.filter.test.js -t "page-size changes|disappear from the refreshed|filters"
```

Expected: new reset/reconciliation tests and existing filter tests PASS.

- [ ] **Step 7: Commit only reset and reconciliation hunks**

```powershell
git add -p -- web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
git diff --cached --check
git commit -m "feat(cpa): reconcile grouped auth selections"
```

Expected: the commit contains the shared filter handlers, list reconciliation effect, and their tests only.

### Task 3: Per-Group Bulk Deletion and Partial Failures

**Files:**
- Modify: `web/src/components/CPAAuthFiles.test.js:1080-end`
- Modify: `web/src/components/CPAAuthFiles.js:130-170, 540-590, 960-1260`

- [ ] **Step 1: Add failing cancellation and successful deletion tests**

Add:

```javascript
test('cancels grouped deletion without sending requests', async () => {
  mockCPAAuthGet({ listData: { files: buildAuthFiles('claude', 2) } });
  window.confirm = jest.fn(() => false);

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });

  const group = container.querySelector('[data-auth-group="claude"]');
  await act(async () => {
    findGroupCheckbox(group, 'Claude').click();
  });
  await act(async () => {
    findButton(group, '删除已选 (2)').click();
  });

  expect(window.confirm).toHaveBeenCalledWith(
    expect.stringMatching(/Claude.*2/)
  );
  expect(helpers.API.delete).not.toHaveBeenCalled();
  expect(findButton(group, '删除已选 (2)')).not.toBeNull();
});

test('deletes only the selected group and refreshes the list once', async () => {
  const files = [...buildAuthFiles('claude', 2), ...buildAuthFiles('codex', 1)];
  mockCPAAuthGet({ listData: { files } });
  helpers.API.delete.mockResolvedValue({ data: { success: true } });
  window.confirm = jest.fn(() => true);

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });

  const claudeGroup = container.querySelector('[data-auth-group="claude"]');
  const codexGroup = container.querySelector('[data-auth-group="codex"]');
  await act(async () => {
    findGroupCheckbox(claudeGroup, 'Claude').click();
    findRowCheckbox(codexGroup, 'codex-001.json').click();
  });
  await act(async () => {
    findButton(claudeGroup, '删除已选 (2)').click();
    await waitForUI();
  });

  expect(helpers.API.delete.mock.calls).toEqual([
    ['/v0/management/auth-files', { params: { name: 'claude-001.json' } }],
    ['/v0/management/auth-files', { params: { name: 'claude-002.json' } }],
  ]);
  expect(getCallsFor('/v0/management/auth-files')).toHaveLength(2);
  expect(findButton(codexGroup, '删除已选 (1)')).not.toBeNull();
  expect(helpers.showSuccess).toHaveBeenCalledWith('已删除 2 个认证文件');
});
```

- [ ] **Step 2: Add failing partial-failure and four-worker tests**

Add:

```javascript
test('keeps only failed auth files selected after grouped deletion', async () => {
  const files = buildAuthFiles('kimi', 3);
  mockCPAAuthGet({ listData: { files } });
  helpers.API.delete.mockImplementation((path, { params }) =>
    params.name === 'kimi-002.json'
      ? Promise.reject(new Error('locked'))
      : Promise.resolve({ data: { success: true } })
  );
  window.confirm = jest.fn(() => true);

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });

  const group = container.querySelector('[data-auth-group="kimi"]');
  await act(async () => {
    findGroupCheckbox(group, 'Kimi').click();
  });
  await act(async () => {
    findButton(group, '删除已选 (3)').click();
    await waitForUI();
  });

  expect(findButton(group, '删除已选 (1)')).not.toBeNull();
  expect(findRowCheckbox(group, 'kimi-002.json')).toBeChecked();
  expect(helpers.showError).toHaveBeenCalledWith(
    expect.stringMatching(/成功 2.*失败 1.*kimi-002\.json/)
  );
});

test('limits grouped auth deletion to four workers', async () => {
  const files = buildAuthFiles('codex', 9);
  mockCPAAuthGet({ listData: { files } });
  let active = 0;
  let peak = 0;
  helpers.API.delete.mockImplementation(
    () =>
      new Promise((resolve) => {
        active += 1;
        peak = Math.max(peak, active);
        setTimeout(() => {
          active -= 1;
          resolve({ data: { success: true } });
        }, 20);
      })
  );
  window.confirm = jest.fn(() => true);

  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });
  const group = container.querySelector('[data-auth-group="codex"]');
  await act(async () => {
    findGroupCheckbox(group, 'Codex').click();
  });
  await act(async () => {
    findButton(group, '删除已选 (9)').click();
    await new Promise((resolve) => setTimeout(resolve, 250));
  });

  expect(peak).toBe(4);
  expect(helpers.API.delete).toHaveBeenCalledTimes(9);
});
```

- [ ] **Step 3: Run the four bulk-delete tests and verify RED**

Run from `web`:

```powershell
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js -t "grouped deletion|grouped auth deletion"
```

Expected: FAIL because the group delete button has no handler and no deletion state exists.

- [ ] **Step 4: Add guarded per-group deletion state and the batch handler**

Add state and a ref beside the existing group operation state:

```javascript
const [deletingGroups, setDeletingGroups] = useState({});
const bulkDeleteInFlightRef = useRef(new Set());
```

Add after `handleDelete`:

```javascript
const handleBulkDelete = async (groupKey, groupName) => {
  const names = [...(selectedNamesByGroup[groupKey] || [])];
  if (!names.length || bulkDeleteInFlightRef.current.has(groupKey)) return;
  if (
    !window.confirm(
      `确定要删除 ${groupName} 组已选择的 ${names.length} 个认证文件吗？`
    )
  ) {
    return;
  }

  bulkDeleteInFlightRef.current.add(groupKey);
  setDeletingGroups((current) => ({ ...current, [groupKey]: true }));
  try {
    const results = await mapWithConcurrency(names, 4, async (name) => {
      requireCPASuccess(
        await API.delete('/v0/management/auth-files', { params: { name } })
      );
      return name;
    });
    const failedNames = results.flatMap((result, index) =>
      result.status === 'rejected' ? [names[index]] : []
    );
    const successCount = names.length - failedNames.length;

    setSelectedNamesByGroup((current) => ({
      ...current,
      [groupKey]: failedNames,
    }));
    await fetchAuthFiles(false);

    if (failedNames.length === 0) {
      showSuccess(`已删除 ${successCount} 个认证文件`);
    } else {
      showError(
        `批量删除完成：成功 ${successCount}，失败 ${failedNames.length}：${failedNames.join(', ')}`
      );
    }
  } finally {
    bulkDeleteInFlightRef.current.delete(groupKey);
    setDeletingGroups((current) => ({ ...current, [groupKey]: false }));
  }
};
```

The worker intentionally converts every thrown transport or CPA business error into the settled result returned by `mapWithConcurrency`; no single failure stops the remaining deletions.

- [ ] **Step 5: Wire the group button and disable conflicting group controls**

Update the group delete button:

```jsx
<Button
  variant='danger'
  size='sm'
  onClick={() => handleBulkDelete(key, name)}
  disabled={selectedNames.length === 0 || deletingGroups[key]}
  loading={Boolean(deletingGroups[key])}
  data-bulk-delete-group={key}
>
  {!deletingGroups[key] && (
    <Trash2 size={14} style={{ marginRight: '0.375rem' }} />
  )}
  删除已选 ({selectedNames.length})
</Button>
```

Set `disabled={Boolean(deletingGroups[key])}` on the current-page checkbox and every row checkbox. During a group deletion, also disable that group's cooldown reset, enable/disable, edit, and single-file delete buttons by combining their existing disabled expression with `Boolean(deletingGroups[key])`. Leave read-only download and quota-fetch buttons available.

- [ ] **Step 6: Format and verify GREEN**

Run from `web`:

```powershell
npx prettier --write src/components/CPAAuthFiles.js src/components/CPAAuthFiles.test.js
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js -t "grouped deletion|grouped auth deletion"
```

Expected: cancellation sends no request; success deletes only the selected group and refreshes once; partial failure retains only the failed file; peak deletion concurrency is 4.

- [ ] **Step 7: Commit only batch-deletion hunks**

```powershell
git add -p -- web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
git diff --cached --check
git commit -m "feat(cpa): delete selected auth files by group"
```

Expected: the commit contains only the grouped deletion handler, operation guards, UI wiring, and deletion tests.

### Task 4: Regression and Production Verification

**Files:**
- Verify: `web/src/components/CPAAuthFiles.js`
- Verify: `web/src/components/CPAAuthFiles.test.js`
- Verify: `web/src/components/CPAAuthFiles.filter.test.js`
- Verify: `web/src/components/CPAAuthFiles.groupQuota.test.js`
- Verify: `web/src/components/cpaAuthStatus.test.js`
- Verify: `web/src/components/cpaQuota.test.js`
- Verify: `web/src/helpers/async-pool.test.js`

- [ ] **Step 1: Run all auth-file and helper tests**

Run from `web`:

```powershell
npm test -- --runInBand --watchAll=false src/components/CPAAuthFiles.test.js src/components/CPAAuthFiles.filter.test.js src/components/CPAAuthFiles.groupQuota.test.js src/components/cpaAuthStatus.test.js src/components/cpaQuota.test.js src/helpers/async-pool.test.js
```

Expected: all selected suites PASS without unhandled Promise rejections or new React `act` warnings.

- [ ] **Step 2: Run the complete frontend test suite**

Run from `web`:

```powershell
npm test -- --runInBand --watchAll=false
```

Expected: all frontend suites PASS. Record any pre-existing failures separately; fix every failure caused by this feature.

- [ ] **Step 3: Build the frontend**

Run from `web`:

```powershell
npm run build
```

Expected: Create React App reports a successful production build with no new lint errors.

- [ ] **Step 4: Inspect formatting, diff scope, and preserved worktree changes**

Run from the repository root:

```powershell
git diff --check
git status --short
git diff -- web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
git diff --cached -- web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
```

Expected: `git diff --check` exits successfully; no generated build artifact is staged; the status still contains every pre-existing unrelated change that was not deliberately included in feature commits.

- [ ] **Step 5: Perform a final feature checklist**

Confirm from tests and rendered behavior:

```text
[ ] Each group selects and deletes independently.
[ ] Current-page select adds only visible files.
[ ] Current-page deselect preserves selections from other pages.
[ ] Pagination and page-size changes preserve selection.
[ ] Search/type/status changes clear all selection and reset every group to page 1.
[ ] Refresh removes selections for files absent from the server list.
[ ] Deletion is confirmed once, bounded to 4 workers, and refreshes once.
[ ] Partial failures keep only failed files selected and name them safely.
[ ] Existing single-file deletion, quota, filtering, and pagination behavior passes.
```

## Completion Criteria

- Every nonempty auth group has a row checkbox, a current-page checkbox, and a group-local delete count/button.
- Cross-page selections persist until a filter change, successful deletion, or server-list reconciliation removes them.
- Search, type, and status changes clear every group selection and restore all groups to page 1; page navigation and page-size changes do not clear selection.
- Grouped deletion reuses the official single-file endpoint with at most four concurrent requests and does not stop after an individual failure.
- Success and failure summaries match the actual settled results, and only failed existing files remain selected after a partial failure.
- Focused tests, related tests, the full frontend suite, the production build, and `git diff --check` complete successfully.
