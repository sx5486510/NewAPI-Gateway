# CPA Ghost Auth + 错误透传 + Windows 原子写

**日期**: 2026-07-24  
**状态**: 待实现  
**范围**: 前端错误透传、Ghost 认证 UI/清理、Gateway `writeFileAtomic` Windows 安全加固  
**方案**: B（纯前端 + gateway 本地原子写，不改上游 CPA 核心）

## 1. 问题

### 1.1 现象

1. 前端额度探测报错：`Grok credential file format is invalid`
2. 网络日志实际为：`GET /v0/management/auth-files/download?name=xai-0b52ce170c07e70c.json` → `404 {"error":"file not found"}`
3. 列表里仍能看到该认证条目

### 1.2 根因

| 层 | 行为 |
|----|------|
| 列表 API | 读内存 `authManager.List()`，磁盘缺失时仅把 `source` 标为 `"memory"`，不立即隐藏 |
| 下载 API | 只读磁盘 `AuthDir/name`；文件不存在 → `404 file not found` |
| 前端 `resolveGrokUserId` | `downloadText` 任意失败都被 `catch` 伪装成 format invalid |
| Gateway `writeFileAtomic` | 用裸 `os.Rename(temp, path)` 覆盖；Windows 上无法 rename 到已存在文件，刷新落盘可能失败并加剧状态不一致 |

### 1.3 Ghost 定义

**Ghost auth**：内存列表仍有条目，但磁盘凭证文件已不存在。

后端已暴露的字段足够前端判定：

- `source === "memory"`：磁盘路径缺失或仅内存
- `runtime_only === true`：配置/API Key 类内存凭证，**不是 ghost**

```
isGhostAuthFile(file) =
  file.source === 'memory' && !file.runtime_only
```

## 2. 目标

1. 下载/读凭证失败时，前端透传真实错误，不再伪装成格式无效
2. Ghost 条目在列表中可见标红，并支持一键批量清理
3. Gateway 写 xAI 凭证时使用 Windows 安全的 backup→rename→rollback，避免覆盖失败或原文件丢失

## 3. 非目标

- 不修改上游 CPA `DownloadAuthFile`（不从内存回退读内容）
- 不修改列表 API schema（不新增 `disk_missing` 字段）
- 不做定时自动清理 ghost
- 不把 `writeFileAtomic` 强行与 `SnapshotStore.writeAtomic` 共享实现（避免耦合；模式对齐即可）

## 4. 方案概览

### 方案 B（已选）

| 项 | 改动位置 | 说明 |
|----|----------|------|
| 错误透传 | `web/src/components/cpaQuota.js` | download 失败 rethrow；仅解析失败才 format invalid |
| Ghost UI + 清理 | `web/src/components/CPAAuthFiles.js` | 徽章/警示 + 顶部「清理磁盘缺失」批量删除 |
| 原子写 | `service/cpa/xai_quota_auth.go` | backup→rename→rollback |

### 备选（未选）

- **A 纯前端**：不修原子写，写盘风险残留
- **C 全链路**：改上游 download/列表 schema，成本高且 vendor 改动大

## 5. 详细设计

### 5.1 错误透传

**文件**: `web/src/components/cpaQuota.js`  
**函数**: `resolveGrokUserId`

当前：

```js
try {
  const credential = objectValue(JSON.parse(String(await downloadText(file.name)).trim()));
  return extractGrokUserId(credential);
} catch {
  throw new Error('Grok credential file format is invalid');
}
```

改为：

1. `await downloadText(file.name)` 放在 try 外或单独 catch：**下载抛错原样 rethrow**
2. 仅 `JSON.parse` / `objectValue` 失败时：
   - 抛 `Grok credential file format is invalid`
3. 下载成功但抽不出 user id 时：保持现有行为（返回 `null`，由调用方决定是否带 `x-userid`），不把“无 sub”误报成 format invalid

**连带影响**

- `fetchGrokQuota` / `sendCPATestMessage` 会看到真实 `file not found` 等错误文案
- 额度错误筛选（401 / prepare credentials）逻辑不变；`file not found` 不自动归入 401 失效清理，由 Ghost 清理按钮处理

**测试** (`cpaQuota.test.js`)

- download 抛 `Error('file not found')` → `fetchCPAQuota` / `resolveGrokUserId` 错误消息仍为 `file not found`
- download 返回非法 JSON → 错误为 `Grok credential file format is invalid`
- download 返回合法 JSON 含 `sub` → 正常注入 `x-userid`（已有用例保留）

### 5.2 Ghost UI + 一键清理

**文件**: `web/src/components/CPAAuthFiles.js`（及对应测试）

#### 5.2.1 判定辅助

```js
const isGhostAuthFile = (file) =>
  file?.source === 'memory' && !file?.runtime_only;
```

- `runtime_only` 兼容后端 bool / 字符串 `"true"`（若已有 disabled 解析工具可复用同类布尔判定）

#### 5.2.2 展示

对每个 `isGhostAuthFile(file)` 的条目：

1. 状态区增加橙色徽章：**「磁盘缺失」**
   - `id`：`${fileId}-ghost-badge`
2. 行容器可加轻微警示背景（与现有 disabled 风格一致，不喧宾夺主）
3. 凭证详情区：若下载失败，显示透传错误（如 `file not found`），不再被 format invalid 掩盖
4. Ghost 条目仍可点单条删除（现有 `handleDelete`）

#### 5.2.3 批量清理

对齐现有「一键删除失效认证」模式：

- 计算：`ghostAuthFiles = authFiles.filter(isGhostAuthFile)`
- 顶部筛选区旁按钮：**「清理磁盘缺失 (N)」**
  - 仅当 `N > 0` 显示
  - 文案与 confirm：明确是 ghost / 磁盘已无文件的内存残留
- 流程：
  1. `window.confirm`
  2. `mapWithConcurrency(names, 4, DELETE /v0/management/auth-files?name=...)`
  3. 进度条可复用 invalid-delete 的 ProgressBar 状态机，或共用一套进度 state（二选一，优先复用模式、独立 state 以免互相打架）
  4. 成功/部分失败 toast + `fetchAuthFiles(false)`

**清理语义**

- 调用现有管理删除 API，清内存残留（磁盘本就无文件）
- 单条失败不中断整批；汇总成功/失败名

**不与「删除 401」合并**

- 401 失效：凭证在盘但鉴权失败
- Ghost：文件已不在盘
- 两类问题正交，按钮分开

#### 5.2.4 测试

- `source: 'memory'` 且非 runtime_only → 可见「磁盘缺失」徽章
- `runtime_only: true` + `source: 'memory'` → **不**显示徽章、不进清理列表
- 清理按钮点击后对 ghost names 调用 DELETE；非 ghost 不删

### 5.3 Windows 安全 `writeFileAtomic`

**文件**: `service/cpa/xai_quota_auth.go`  
**调用方**: `persistXAIToken`（token 刷新后写回 auth 文件）

#### 5.3.1 算法（对齐 `SnapshotStore.writeAtomic`）

1. `CreateTemp` 写 body，`Chmod(mode)`，`Sync`，`Close`
2. 若目标存在：
   - 删除旧 `target.bak`（若有）
   - `rename(target → target.bak)`，`haveBackup = true`
3. `rename(temp → target)`
4. 若步骤 3 失败且 `haveBackup`：`rename(bak → target)` 回滚；错误信息可附带 restore 失败
5. 若步骤 3 成功且 `haveBackup`：`Remove(bak)`
6. 任意失败路径清理残留 temp（defer）

**禁止**：先 `Remove(target)` 再 rename（会永久丢原文件）。

#### 5.3.2 可测性

为便于测试注入失败，将 rename 抽成包级可替换变量：

```go
var atomicRename = os.Rename // tests may override
```

`writeFileAtomic` 内部全部通过 `atomicRename` 移动文件。

#### 5.3.3 测试 (`xai_quota_auth_test.go` 或新 `write_file_atomic_test.go`)

1. **成功覆盖**：目标已存在 → 写入新内容 → 读回为新内容；无 `.tmp` / `.bak` 残留
2. **备份阶段失败**：注入 `atomicRename` 第一次调用失败 → 原文件内容不变
3. **替换阶段失败**：注入第一次 rename 成功、第二次失败 → 原文件经回滚恢复；temp/bak 清理合理

## 6. 数据流

```
列表 GET /auth-files
  → entry: { name, source: "memory", runtime_only: false, ... }
  → UI: 「磁盘缺失」徽章 + 可清理

用户刷新 Grok 额度
  → downloadAuthFileText(name)
  → 404 file not found
  → requireCPASuccess 抛 Error("file not found")
  → resolveGrokUserId rethrow 原错误
  → 额度状态 error: "file not found"（不再 format invalid）

用户点「清理磁盘缺失」
  → DELETE 各 ghost name
  → 刷新列表，ghost 消失
```

Token 刷新写盘：

```
persistXAIToken
  → writeFileAtomic(path, body)
  → backup → rename temp → drop bak
  → Windows 上可安全覆盖
```

## 7. 文件清单

| 文件 | 变更 |
|------|------|
| `web/src/components/cpaQuota.js` | `resolveGrokUserId` 错误分流 |
| `web/src/components/cpaQuota.test.js` | 透传 / format invalid 用例 |
| `web/src/components/CPAAuthFiles.js` | ghost 判定、徽章、清理按钮与逻辑 |
| `web/src/components/CPAAuthFiles*.test.js` | ghost UI + 清理测试 |
| `service/cpa/xai_quota_auth.go` | `writeFileAtomic` + `atomicRename` |
| `service/cpa/*_test.go` | 原子写成功/失败用例 |

## 8. 验收标准

1. 磁盘无文件的条目下载失败时，前端错误文案包含 `file not found`（或后端原 message），**不是** `Grok credential file format is invalid`
2. 真格式错误（非法 JSON）仍报 format invalid
3. `source=memory && !runtime_only` 显示「磁盘缺失」；runtime_only 不显示
4. 「清理磁盘缺失」只删除 ghost，并刷新列表
5. `writeFileAtomic` 在目标已存在时能覆盖；rename 失败不丢原文件（测试覆盖）

## 9. 实现顺序建议

1. 先写/改失败测试（错误透传、ghost 判定、原子写失败保留）
2. 实现 `resolveGrokUserId` 分流
3. 实现 ghost UI + 清理
4. 实现 `writeFileAtomic` 加固
5. 跑相关前端 jest 与 `go test ./service/cpa/ -count=1`
