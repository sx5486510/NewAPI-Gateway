# CPA Ghost Auth + Error Passthrough + Windows Atomic Write Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 下载失败透传真实错误、列表标红并一键清理 ghost 认证、Gateway `writeFileAtomic` 在 Windows 上安全覆盖。

**Architecture:** 三处独立改动：(1) `resolveGrokUserId` 把 download 错误与 JSON 解析错误分流；(2) `CPAAuthFiles` 用列表字段 `source==='memory' && !runtime_only` 识别 ghost，加徽章与批量 DELETE；(3) `service/cpa.writeFileAtomic` 改为 backup→rename→rollback，并通过可注入 `atomicRename` 覆盖失败路径测试。

**Tech Stack:** React + Jest（web）、Go 1.x + testing（service/cpa）。规格：`docs/superpowers/specs/2026-07-24-cpa-ghost-auth-atomic-write-design.md`。

**Spec:** `docs/superpowers/specs/2026-07-24-cpa-ghost-auth-atomic-write-design.md`

---

## File Structure

| File | Responsibility |
|------|----------------|
| `web/src/components/cpaQuota.js` | `resolveGrokUserId` 错误分流 |
| `web/src/components/cpaQuota.test.js` | 透传 / format invalid 测试 |
| `web/src/components/CPAAuthFiles.js` | `isGhostAuthFile`、徽章、清理按钮与逻辑 |
| `web/src/components/CPAAuthFiles.ghost.test.js` | ghost UI + 清理（新建） |
| `service/cpa/xai_quota_auth.go` | `atomicRename` + `writeFileAtomic` |
| `service/cpa/write_file_atomic_test.go` | 原子写成功/失败测试（新建） |

---

### Task 1: Grok 凭证错误透传

**Files:**
- Modify: `web/src/components/cpaQuota.js` (`resolveGrokUserId`)
- Modify: `web/src/components/cpaQuota.test.js`

- [ ] **Step 1: Write the failing tests**

在 `cpaQuota.test.js` 的 Grok 相关 describe 中（约「loads x-userid」用例附近）追加：

```js
test('Grok surfaces download failures instead of format-invalid', async () => {
  const post = jest.fn();
  const downloadText = jest.fn(() =>
    Promise.reject(new Error('file not found'))
  );

  await expect(
    fetchCPAQuota(
      {
        name: 'xai-missing.json',
        provider: 'xai',
        auth_index: 'runtime-index',
      },
      { post, downloadText }
    )
  ).rejects.toThrow('file not found');

  expect(downloadText).toHaveBeenCalledWith('xai-missing.json');
  expect(post).not.toHaveBeenCalled();
});

test('Grok still reports invalid credential JSON as format-invalid', async () => {
  const post = jest.fn();
  const downloadText = jest.fn(() => Promise.resolve('not-json'));

  await expect(
    fetchCPAQuota(
      {
        name: 'xai-bad.json',
        provider: 'xai',
        auth_index: 'runtime-index',
      },
      { post, downloadText }
    )
  ).rejects.toThrow('Grok credential file format is invalid');

  expect(post).not.toHaveBeenCalled();
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd web && npm test -- --watchAll=false --testPathPattern=cpaQuota.test.js
```

Expected: 上述两个新用例 FAIL（当前 download 失败也会变成 format invalid，或两者都变成 format invalid）。

- [ ] **Step 3: Implement minimal fix**

替换 `web/src/components/cpaQuota.js` 中 `resolveGrokUserId`：

```js
export const resolveGrokUserId = async (file, downloadText) => {
  const direct = extractGrokUserId(file);
  if (direct || typeof downloadText !== 'function' || !file?.name) return direct;

  let text;
  try {
    text = await downloadText(file.name);
  } catch (error) {
    // 下载/代理失败透传（如 file not found），不要伪装成格式错误
    throw error;
  }

  try {
    const credential = objectValue(JSON.parse(String(text).trim()));
    return extractGrokUserId(credential);
  } catch {
    throw new Error('Grok credential file format is invalid');
  }
};
```

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
cd web && npm test -- --watchAll=false --testPathPattern=cpaQuota.test.js
```

Expected: 全部 PASS（含旧 Grok userid 用例）。

- [ ] **Step 5: Commit**

```bash
git add web/src/components/cpaQuota.js web/src/components/cpaQuota.test.js
git commit -m "$(cat <<'EOF'
fix(cpa): pass through Grok credential download errors

Do not mask file-not-found as format-invalid; only real JSON parse
failures use the format error message.
EOF
)"
```

---

### Task 2: Ghost 判定 + 徽章 + 一键清理

**Files:**
- Modify: `web/src/components/CPAAuthFiles.js`
- Create: `web/src/components/CPAAuthFiles.ghost.test.js`

- [ ] **Step 1: Write the failing tests**

新建 `web/src/components/CPAAuthFiles.ghost.test.js`：

```js
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import CPAAuthFiles from './CPAAuthFiles';
import * as helpers from '../helpers';

jest.mock('../helpers', () => ({
  API: {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
    delete: jest.fn(),
  },
  showError: jest.fn(),
  showSuccess: jest.fn(),
}));

global.IS_REACT_ACT_ENVIRONMENT = true;

const waitForUI = () => new Promise((resolve) => setTimeout(resolve, 100));

const defaultCredential = JSON.stringify({
  expired: '2099-07-20T08:00:00Z',
  refresh_token: 'default-refresh-secret',
  sub: 'subject-1',
});

describe('CPAAuthFiles - ghost auth', () => {
  let container;

  beforeEach(() => {
    jest.clearAllMocks();
    window.confirm = jest.fn(() => true);
    container = document.createElement('div');
    document.body.appendChild(container);
  });

  afterEach(() => {
    document.body.removeChild(container);
    container = null;
  });

  const setupList = (files) => {
    helpers.API.get.mockImplementation((path, config) => {
      if (path === '/v0/management/auth-files') {
        return Promise.resolve({ data: { files } });
      }
      if (path === '/v0/management/auth-files/download') {
        const name = config?.params?.name;
        if (name === 'xai-ghost.json') {
          return Promise.resolve({
            data: { success: false, message: 'file not found' },
          });
        }
        return Promise.resolve({ data: defaultCredential });
      }
      return Promise.reject(new Error(`unexpected GET ${path}`));
    });
    helpers.API.delete.mockResolvedValue({ data: { success: true } });
  };

  test('shows disk-missing badge for memory source non-runtime auth', async () => {
    setupList([
      {
        name: 'xai-ghost.json',
        type: 'xai',
        provider: 'xai',
        auth_index: 'g1',
        source: 'memory',
        runtime_only: false,
        disabled: false,
      },
      {
        name: 'xai-ok.json',
        type: 'xai',
        provider: 'xai',
        auth_index: 'g2',
        source: 'file',
        disabled: false,
      },
      {
        name: 'xai-apikey.json',
        type: 'xai',
        provider: 'xai',
        auth_index: 'g3',
        source: 'memory',
        runtime_only: true,
        disabled: false,
      },
    ]);

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const ghostRow = container.querySelector('[data-auth-file="xai-ghost.json"]');
    const okRow = container.querySelector('[data-auth-file="xai-ok.json"]');
    const runtimeRow = container.querySelector(
      '[data-auth-file="xai-apikey.json"]'
    );

    expect(ghostRow.textContent).toContain('磁盘缺失');
    expect(okRow.textContent).not.toContain('磁盘缺失');
    expect(runtimeRow.textContent).not.toContain('磁盘缺失');

    const cleanupBtn = container.querySelector(
      '#cpa-auth-files-delete-ghost-btn'
    );
    expect(cleanupBtn).toBeTruthy();
    expect(cleanupBtn.getAttribute('data-delete-ghost-count')).toBe('1');
  });

  test('bulk cleans only ghost auth files', async () => {
    setupList([
      {
        name: 'xai-ghost.json',
        type: 'xai',
        provider: 'xai',
        auth_index: 'g1',
        source: 'memory',
        runtime_only: false,
        disabled: false,
      },
      {
        name: 'xai-ok.json',
        type: 'xai',
        provider: 'xai',
        auth_index: 'g2',
        source: 'file',
        disabled: false,
      },
    ]);

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const cleanupBtn = container.querySelector(
      '#cpa-auth-files-delete-ghost-btn'
    );
    await act(async () => {
      cleanupBtn.click();
      await waitForUI();
    });

    expect(helpers.API.delete).toHaveBeenCalledWith(
      '/v0/management/auth-files',
      { params: { name: 'xai-ghost.json' } }
    );
    const deletedNames = helpers.API.delete.mock.calls.map(
      (call) => call[1]?.params?.name
    );
    expect(deletedNames).toEqual(['xai-ghost.json']);
    expect(helpers.showSuccess).toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd web && npm test -- --watchAll=false --testPathPattern=CPAAuthFiles.ghost.test.js
```

Expected: FAIL（无 `磁盘缺失` / 无 `#cpa-auth-files-delete-ghost-btn`）。

- [ ] **Step 3: Implement helpers + state + cleanup handler**

在 `CPAAuthFiles.js` 顶部辅助区（`isInvalidAuthQuotaState` 附近）增加：

```js
const isTruthyFlag = (value) => {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  return typeof value === 'string' && value.trim().toLowerCase() === 'true';
};

// Ghost：磁盘文件已不存在，但内存列表仍有条目。runtime_only 是配置/API Key，不算 ghost。
const isGhostAuthFile = (file) =>
  file?.source === 'memory' && !isTruthyFlag(file?.runtime_only);
```

在组件 state 区（`invalidDeleteProgress` 旁）增加：

```js
const [deletingGhostAuths, setDeletingGhostAuths] = useState(false);
const [ghostDeleteProgress, setGhostDeleteProgress] = useState(null);
const ghostDeleteInFlightRef = useRef(false);
```

在 `invalidAuthFiles` 的 `useMemo` 附近增加：

```js
const ghostAuthFiles = useMemo(
  () => authFiles.filter(isGhostAuthFile),
  [authFiles]
);
```

复制 `handleDeleteInvalidAuths` 模式新增 `handleDeleteGhostAuths`（文案改为磁盘缺失 / ghost）：

```js
const handleDeleteGhostAuths = async () => {
  const names = ghostAuthFiles.map((file) => file.name).filter(Boolean);
  if (!names.length || ghostDeleteInFlightRef.current) return;
  if (
    !window.confirm(
      `确定要清理 ${names.length} 个磁盘缺失的认证吗？\n（内存残留，磁盘文件已不存在）`
    )
  ) {
    return;
  }

  ghostDeleteInFlightRef.current = true;
  setDeletingGhostAuths(true);
  setGhostDeleteProgress({ completed: 0, total: names.length });

  try {
    let completed = 0;
    const results = await mapWithConcurrency(names, 4, async (name) => {
      try {
        requireCPASuccess(
          await API.delete('/v0/management/auth-files', { params: { name } })
        );
        completed += 1;
        setGhostDeleteProgress({ completed, total: names.length });
        return name;
      } catch (error) {
        completed += 1;
        setGhostDeleteProgress({ completed, total: names.length });
        throw error;
      }
    });
    const failedNames = results.flatMap((result, index) =>
      result.status === 'rejected' ? [names[index]] : []
    );
    const successCount = names.length - failedNames.length;

    await fetchAuthFiles(false);

    if (failedNames.length === 0) {
      showSuccess(`已清理 ${successCount} 个磁盘缺失认证`);
    } else {
      showError(
        `清理完成：成功 ${successCount}，失败 ${failedNames.length}：${failedNames.join(', ')}`
      );
    }
  } finally {
    ghostDeleteInFlightRef.current = false;
    setDeletingGhostAuths(false);
    setGhostDeleteProgress(null);
  }
};
```

- [ ] **Step 4: Implement UI badge + button**

在筛选区「一键删除失效」按钮旁增加：

```jsx
<Button
  id='cpa-auth-files-delete-ghost-btn'
  variant='danger'
  size='sm'
  onClick={handleDeleteGhostAuths}
  disabled={ghostAuthFiles.length === 0 || deletingGhostAuths}
  loading={deletingGhostAuths}
  title='清理磁盘文件已缺失的内存残留认证'
  data-delete-ghost-count={ghostAuthFiles.length}
>
  {!deletingGhostAuths && (
    <Trash2 size={14} style={{ marginRight: '0.375rem' }} />
  )}
  {deletingGhostAuths
    ? `清理中 ${ghostDeleteProgress?.completed || 0}/${
        ghostDeleteProgress?.total || 0
      }`
    : `清理磁盘缺失 (${ghostAuthFiles.length})`}
</Button>
```

在匹配计数文案中追加 ghost 数量（可选）：

```jsx
{ghostAuthFiles.length > 0 ? ` · 磁盘缺失 ${ghostAuthFiles.length}` : ''}
```

在行内状态徽章（`fileId-status-badge`）旁，对 ghost 再加：

```jsx
{isGhostAuthFile(file) && (
  <span
    id={`${fileId}-ghost-badge`}
    style={{
      padding: '0.25rem 0.75rem',
      borderRadius: '999px',
      fontSize: '0.75rem',
      fontWeight: 500,
      backgroundColor: '#FFEDD5',
      color: '#9A3412',
      whiteSpace: 'nowrap',
    }}
  >
    磁盘缺失
  </span>
)}
```

可选：ghost 行容器背景 `#FFF7ED`（轻微警示）。

- [ ] **Step 5: Run tests to verify they pass**

Run:

```bash
cd web && npm test -- --watchAll=false --testPathPattern='CPAAuthFiles\.(ghost|quota401|filter|test)\.js'
```

Expected: ghost 相关 PASS；既有 CPAAuthFiles 测试不回归。

- [ ] **Step 6: Commit**

```bash
git add web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.ghost.test.js
git commit -m "$(cat <<'EOF'
feat(cpa): mark and bulk-clean ghost auth files missing on disk

Show a disk-missing badge for memory-only non-runtime entries and add
one-click cleanup via the existing auth-files delete API.
EOF
)"
```

---

### Task 3: Windows 安全 `writeFileAtomic`

**Files:**
- Modify: `service/cpa/xai_quota_auth.go`
- Create: `service/cpa/write_file_atomic_test.go`

- [ ] **Step 1: Write the failing tests**

新建 `service/cpa/write_file_atomic_test.go`：

```go
package cpa

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomicOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	body := []byte("{\n  \"access_token\": \"new\"\n}\n")
	if err := writeFileAtomic(path, body, 0o600); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("content = %q, want %q", got, body)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("leftover files: %v", names)
	}
}

func TestWriteFileAtomicLeavesOriginalWhenBackupRenameFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	original := []byte("keep-me\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	old := atomicRename
	atomicRename = func(string, string) error { return errors.New("rename failed") }
	t.Cleanup(func() { atomicRename = old })

	err := writeFileAtomic(path, []byte("new\n"), 0o600)
	if err == nil || !strings.Contains(err.Error(), "rename failed") {
		t.Fatalf("expected rename failure, got %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("original changed: %q", got)
	}
}

func TestWriteFileAtomicRestoresOriginalWhenReplaceFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	original := []byte("keep-me\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	old := atomicRename
	calls := 0
	atomicRename = func(oldpath, newpath string) error {
		calls++
		// 1st: target -> bak succeeds via real rename
		// 2nd: temp -> target fails
		if calls == 1 {
			return old(oldpath, newpath)
		}
		return errors.New("replace failed")
	}
	t.Cleanup(func() { atomicRename = old })

	err := writeFileAtomic(path, []byte("new\n"), 0o600)
	if err == nil || !strings.Contains(err.Error(), "replace failed") {
		t.Fatalf("expected replace failure, got %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("original not restored: %q", got)
	}
	if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bak should be cleaned or restored, stat=%v", err)
	}
}
```

注意：第三个测试依赖 `writeFileAtomic` 实现中：

1. 第一次 `atomicRename` = target→bak  
2. 第二次 = temp→target  

若实现顺序不同，调整测试中的 call 编号。

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./service/cpa/ -count=1 -run 'TestWriteFileAtomic'
```

Expected:

- `TestWriteFileAtomicOverwritesExisting` 在 Windows 上可能 FAIL（裸 `os.Rename` 无法覆盖）；在 Unix 上可能 PASS
- 失败注入测试 FAIL（尚无 `atomicRename` / 回滚路径）

- [ ] **Step 3: Implement `atomicRename` + safe `writeFileAtomic`**

在 `service/cpa/xai_quota_auth.go` 中替换 `writeFileAtomic`，并增加包级变量：

```go
// atomicRename is os.Rename by default; tests may override to inject failures.
var atomicRename = os.Rename

func writeFileAtomic(path string, body []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	temporary, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err = temporary.Write(body); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}

	// Windows cannot rename over an existing file. Move the original aside
	// first, replace, and restore the backup if the replace fails.
	backupPath := path + ".bak"
	haveBackup := false
	if _, statErr := os.Stat(path); statErr == nil {
		_ = os.Remove(backupPath)
		if err = atomicRename(path, backupPath); err != nil {
			return fmt.Errorf("back up old file before replace: %w", err)
		}
		haveBackup = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat old file before replace: %w", statErr)
	}

	if err = atomicRename(temporaryPath, path); err != nil {
		if haveBackup {
			if restoreErr := atomicRename(backupPath, path); restoreErr != nil {
				return fmt.Errorf("replace file: %w (and restoring original failed: %v)", err, restoreErr)
			}
		}
		return fmt.Errorf("replace file: %w", err)
	}

	if haveBackup {
		_ = os.Remove(backupPath)
	}
	// Success path: temp was renamed away; prevent defer from deleting target
	// if temporary handle state is odd — temporaryPath no longer exists.
	return nil
}
```

确认文件顶部 import 已有：`errors`, `fmt`, `os`, `path/filepath`（`xai_quota_auth.go` 已使用这些包则无需新增）。

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
go test ./service/cpa/ -count=1 -run 'TestWriteFileAtomic'
```

Expected: PASS。

再跑相关回归：

```bash
go test ./service/cpa/ -count=1 -run 'XAI|Atomic|WriteFile'
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add service/cpa/xai_quota_auth.go service/cpa/write_file_atomic_test.go
git commit -m "$(cat <<'EOF'
fix(cpa): make writeFileAtomic Windows-safe with backup rollback

Replace bare os.Rename overwrite with backup→rename→restore so token
refresh cannot leave missing auth files when replace fails.
EOF
)"
```

---

### Task 4: 全量验收

**Files:** none（只跑测试）

- [ ] **Step 1: Frontend suite**

```bash
cd web && npm test -- --watchAll=false --testPathPattern='cpaQuota|CPAAuthFiles'
```

Expected: PASS。

- [ ] **Step 2: Go suite for service/cpa**

```bash
go test ./service/cpa/ -count=1
```

Expected: PASS（或仅与本次无关的既有 flaky 失败；若有需单独说明）。

- [ ] **Step 3: Manual checklist（可选）**

1. 列表有 `source=memory` 条目 → 见「磁盘缺失」
2. 点额度刷新 → 错误为 `file not found` 而非 format invalid
3. 点「清理磁盘缺失」→ 条目消失

- [ ] **Step 4: Final status commit only if dirty**

若 Task 4 无代码变更则跳过 commit。

---

## Spec Coverage Checklist

| Spec 要求 | Task |
|-----------|------|
| download 失败透传 | Task 1 |
| 非法 JSON → format invalid | Task 1 |
| `isGhostAuthFile` 判定 | Task 2 |
| 磁盘缺失徽章 | Task 2 |
| 一键清理 ghost | Task 2 |
| runtime_only 不算 ghost | Task 2 |
| writeFileAtomic backup→rename→rollback | Task 3 |
| 原子写失败保留原文件测试 | Task 3 |
| 不改上游 DownloadAuthFile / 列表 schema | 全程遵守 |

## Self-Review Notes

- 无 TBD/占位步骤
- `atomicRename` 名称在 Task 3 测试与实现一致
- Ghost 清理与 401 清理按钮分离，与 spec 一致
- 不引入后端 schema 变更
