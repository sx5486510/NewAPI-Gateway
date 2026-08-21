# 客户端限制串行与语义修正 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 统一 `ModelRoute` 与 `ProviderToken` 两套 `IsClientAllowed` 的语义为同一张真值表，并在选路阶段把两者接成串行与检查，使令牌页配置的客户端限制真正生效。

**Architecture:** 抽出单一共享判定函数 `EvaluateClientRestriction()` 放在新文件 `model/client_restriction.go`，两个结构体的 `IsClientAllowed` 方法都委托给它（DRY）。随后在 `model/model_route.go:BuildRouteAttemptsByPriority()` 的候选路由循环里补上 `token.IsClientAllowed()` 检查，并加过滤日志以便观测放量影响。

**Tech Stack:** Go 1.26.1 / GORM / 测试用 `github.com/glebarez/sqlite` 内存库（仓库既有约定）

---

## 背景：为什么要做

`docs/client-restriction.md:36-43` 声明选路阶段会调用 `token.IsClientAllowed(clientType)`，但实际代码（`model/model_route.go:462`）只调用了 `route.IsClientAllowed(clientType)`。全局搜索确认 `ProviderToken.IsClientAllowed` **没有任何生产调用方**，只有 `model/provider_token_test.go` 在测。

后果：管理员在「供应商 → 令牌管理」页（文档列为**方式一**，主推路径）勾选的客户端限制**静默无效**。失败是静默的——请求正常返回 200，代价以上游封号的形式延迟到账。

同时两套实现对「泛客户端」（UA 识别不出，`clientType == ""`）的处理相反，不统一无法做与运算。

## 目标真值表（已确认）

| 勾选状态 | `clientType="codex"` | `clientType="cc"` | `clientType=""` |
|---|---|---|---|
| 都不勾 | ✅ | ✅ | ✅ |
| 仅 Codex | ✅ | ❌ | ❌ |
| 仅 CC | ❌ | ✅ | ❌ |
| Codex + CC | ✅ | ✅ | ❌ |
| 全禁用 | ❌ | ❌ | ✅ |

**语义解释：**
- 勾了 Codex/CC = 「这条线路**保留给**指定客户端」，泛客户端也算跑错客户端，一并拒绝
- 全禁用（`BlockClients`）= 「这条线路**禁止**已识别客户端」，只放行泛客户端。对应前端标签「全禁用」，与 `allow_codex`/`allow_cc` 互斥（`web/src/components/ModelRoutesTable.js:268-278`）

**相对现状的差异：**
- `ModelRoute`：仅「全禁用」行改变（原为无条件 `return false`，连泛客户端也拒）
- `ProviderToken`：「仅 Codex」「仅 CC」「Codex+CC」三行的泛客户端列改变（原为无条件放行）

## File Structure

| 文件 | 责任 |
|---|---|
| `model/client_restriction.go` **（新建）** | 唯一的客户端准入判定逻辑 + 归一化，无 DB 依赖，纯函数便于表驱动测试 |
| `model/client_restriction_test.go` **（新建）** | 覆盖完整真值表的表驱动测试 |
| `model/model_route.go` **（改）** | `IsClientAllowed` 委托给共享函数；`BuildRouteAttemptsByPriority` 补 token 层检查 |
| `model/provider_token.go` **（改）** | `IsClientAllowed` 委托给共享函数 |
| `model/model_route_test.go` **（改）** | 新增串行与的集成测试 |
| `docs/client-restriction.md` **（改）** | 补真值表，修正与代码不符的描述 |

---

### Task 1: 抽出共享判定函数

**Files:**
- Create: `model/client_restriction.go`
- Test: `model/client_restriction_test.go`

- [ ] **Step 1: 写失败测试（完整真值表）**

创建 `model/client_restriction_test.go`：

```go
package model

import "testing"

func TestEvaluateClientRestrictionTruthTable(t *testing.T) {
	tests := []struct {
		name         string
		allowCodex   bool
		allowCC      bool
		blockClients bool
		wantCodex    bool
		wantCC       bool
		wantGeneric  bool
	}{
		{
			name:      "no restriction allows everyone",
			wantCodex: true, wantCC: true, wantGeneric: true,
		},
		{
			name:       "codex only reserves route for codex",
			allowCodex: true,
			wantCodex:  true, wantCC: false, wantGeneric: false,
		},
		{
			name:      "cc only reserves route for cc",
			allowCC:   true,
			wantCodex: false, wantCC: true, wantGeneric: false,
		},
		{
			name:       "codex and cc excludes generic",
			allowCodex: true, allowCC: true,
			wantCodex: true, wantCC: true, wantGeneric: false,
		},
		{
			name:         "block clients rejects identified clients only",
			blockClients: true,
			wantCodex:    false, wantCC: false, wantGeneric: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EvaluateClientRestriction("codex", tt.allowCodex, tt.allowCC, tt.blockClients); got != tt.wantCodex {
				t.Errorf("codex: got %v, want %v", got, tt.wantCodex)
			}
			if got := EvaluateClientRestriction("cc", tt.allowCC && false || tt.allowCC, tt.allowCC, tt.blockClients); got != tt.wantCC {
				_ = got
			}
			if got := EvaluateClientRestriction("cc", tt.allowCodex, tt.allowCC, tt.blockClients); got != tt.wantCC {
				t.Errorf("cc: got %v, want %v", got, tt.wantCC)
			}
			if got := EvaluateClientRestriction("", tt.allowCodex, tt.allowCC, tt.blockClients); got != tt.wantGeneric {
				t.Errorf("generic: got %v, want %v", got, tt.wantGeneric)
			}
		})
	}
}

func TestEvaluateClientRestrictionNormalizesConflictingFlags(t *testing.T) {
	// blockClients 与 allow* 同时为真时，blockClients 优先（与
	// NormalizeClientRestrictions 一致），因此 codex 被拒、泛客户端放行。
	if EvaluateClientRestriction("codex", true, true, true) {
		t.Error("expected blockClients to win over allow flags for codex")
	}
	if !EvaluateClientRestriction("", true, true, true) {
		t.Error("expected blockClients to allow generic client")
	}
}

func TestEvaluateClientRestrictionRejectsUnknownClientType(t *testing.T) {
	// 未来新增客户端类型时默认拒绝受限线路，避免静默放量。
	if EvaluateClientRestriction("gemini-cli", true, false, false) {
		t.Error("expected unknown client type to be rejected on a restricted route")
	}
	if !EvaluateClientRestriction("gemini-cli", false, false, false) {
		t.Error("expected unknown client type to pass an unrestricted route")
	}
}
```

> 注意：上面第一个测试里有一行多余的 `if got := EvaluateClientRestriction("cc", tt.allowCC && false || tt.allowCC, ...)` 是笔误，删掉它，只保留紧随其后的那行 `EvaluateClientRestriction("cc", tt.allowCodex, tt.allowCC, tt.blockClients)`。清理后的循环体应为四个断言：codex、cc、generic 各一次。

清理后的循环体：

```go
		t.Run(tt.name, func(t *testing.T) {
			if got := EvaluateClientRestriction("codex", tt.allowCodex, tt.allowCC, tt.blockClients); got != tt.wantCodex {
				t.Errorf("codex: got %v, want %v", got, tt.wantCodex)
			}
			if got := EvaluateClientRestriction("cc", tt.allowCodex, tt.allowCC, tt.blockClients); got != tt.wantCC {
				t.Errorf("cc: got %v, want %v", got, tt.wantCC)
			}
			if got := EvaluateClientRestriction("", tt.allowCodex, tt.allowCC, tt.blockClients); got != tt.wantGeneric {
				t.Errorf("generic: got %v, want %v", got, tt.wantGeneric)
			}
		})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./model/ -run TestEvaluateClientRestriction -v`

Expected: 编译失败，`undefined: EvaluateClientRestriction`

- [ ] **Step 3: 实现共享函数**

创建 `model/client_restriction.go`：

```go
package model

// 客户端类型常量，与 middleware/agg_token_auth.go:identifyClientType 的返回值一致。
const (
	ClientTypeCodex   = "codex"
	ClientTypeCC      = "cc"
	ClientTypeGeneric = ""
)

// EvaluateClientRestriction 是客户端准入的唯一判定入口。ModelRoute 与
// ProviderToken 的 IsClientAllowed 都委托给它，保证两层串行与运算时语义一致。
//
// 语义：
//   - 三个开关都为 false：不限制，放行所有客户端
//   - allowCodex/allowCC 任一为 true：线路"保留给"指定客户端，其余（含泛客户端）拒绝
//   - blockClients 为 true：线路"禁止"已识别客户端，只放行泛客户端
//
// blockClients 与 allow* 冲突时 blockClients 优先，与 NormalizeClientRestrictions 一致。
func EvaluateClientRestriction(clientType string, allowCodex, allowCC, blockClients bool) bool {
	if blockClients {
		allowCodex = false
		allowCC = false
	}

	if !allowCodex && !allowCC && !blockClients {
		return true
	}

	if blockClients {
		return clientType == ClientTypeGeneric
	}

	switch clientType {
	case ClientTypeCodex:
		return allowCodex
	case ClientTypeCC:
		return allowCC
	default:
		// 泛客户端与未来新增的未知客户端类型：受限线路一律拒绝。
		return false
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./model/ -run TestEvaluateClientRestriction -v`

Expected: PASS，三个测试函数全绿

- [ ] **Step 5: 提交**

```bash
git add model/client_restriction.go model/client_restriction_test.go
git commit -m "feat(model): add shared client restriction evaluator"
```

---

### Task 2: 两个 IsClientAllowed 委托给共享函数

**Files:**
- Modify: `model/model_route.go:90-112`
- Modify: `model/provider_token.go:170-191`
- Test: `model/provider_token_test.go:24-36`（需更新既有断言）

- [ ] **Step 1: 更新既有 ProviderToken 测试以反映新语义**

`model/provider_token_test.go:24-36` 现有测试恰好只覆盖 `BlockClients: true` 这一行，而该行语义**未变**（codex ❌ / cc ❌ / 泛 ✅），所以它应当继续通过。补一个覆盖变化行的测试，追加到 `model/provider_token_test.go` 末尾：

```go
func TestProviderTokenIsClientAllowedExcludesGenericWhenReserved(t *testing.T) {
	codexOnly := ProviderToken{AllowCodex: true}
	if !codexOnly.IsClientAllowed("codex") {
		t.Error("expected codex client to be allowed on a codex-reserved token")
	}
	if codexOnly.IsClientAllowed("cc") {
		t.Error("expected cc client to be rejected on a codex-reserved token")
	}
	if codexOnly.IsClientAllowed("") {
		t.Error("expected generic client to be rejected on a codex-reserved token")
	}

	ccOnly := ProviderToken{AllowCC: true}
	if !ccOnly.IsClientAllowed("cc") {
		t.Error("expected cc client to be allowed on a cc-reserved token")
	}
	if ccOnly.IsClientAllowed("") {
		t.Error("expected generic client to be rejected on a cc-reserved token")
	}

	unrestricted := ProviderToken{}
	if !unrestricted.IsClientAllowed("") || !unrestricted.IsClientAllowed("codex") || !unrestricted.IsClientAllowed("cc") {
		t.Error("expected unrestricted token to allow every client type")
	}
}

func TestModelRouteIsClientAllowedBlockClientsKeepsGeneric(t *testing.T) {
	blocked := ModelRoute{BlockClients: true}
	if blocked.IsClientAllowed("codex") {
		t.Error("expected codex client to be blocked")
	}
	if blocked.IsClientAllowed("cc") {
		t.Error("expected cc client to be blocked")
	}
	if !blocked.IsClientAllowed("") {
		t.Error("expected generic client to remain allowed when only identified clients are blocked")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./model/ -run "TestProviderTokenIsClientAllowedExcludesGeneric|TestModelRouteIsClientAllowedBlockClientsKeepsGeneric" -v`

Expected: 两个测试都 FAIL —
- `TestProviderTokenIsClientAllowedExcludesGeneric...`：报 "expected generic client to be rejected on a codex-reserved token"（现实现无条件放行泛客户端）
- `TestModelRouteIsClientAllowedBlockClientsKeepsGeneric`：报 "expected generic client to remain allowed..."（现实现无条件 return false）

- [ ] **Step 3: 改 ModelRoute.IsClientAllowed**

把 `model/model_route.go:89-112` 整段替换为：

```go
// IsClientAllowed checks if a route allows the specified client type.
// 判定逻辑集中在 EvaluateClientRestriction，与 ProviderToken 共用同一张真值表。
func (mr *ModelRoute) IsClientAllowed(clientType string) bool {
	mr.NormalizeClientRestrictions()
	return EvaluateClientRestriction(clientType, mr.AllowCodex, mr.AllowCC, mr.BlockClients)
}
```

保留其下方 `NormalizeClientRestrictions()`（`model/model_route.go:114-122`）不动——它有 `model_route.go:204`、`:226` 两个独立调用方依赖其副作用。

- [ ] **Step 4: 改 ProviderToken.IsClientAllowed**

把 `model/provider_token.go:165-191` 整段替换为：

```go
// IsClientAllowed checks if a client type is allowed to use this token.
// 判定逻辑集中在 EvaluateClientRestriction，与 ModelRoute 共用同一张真值表。
func (pt *ProviderToken) IsClientAllowed(clientType string) bool {
	pt.NormalizeClientRestrictions()
	return EvaluateClientRestriction(clientType, pt.AllowCodex, pt.AllowCC, pt.BlockClients)
}
```

同样保留 `NormalizeClientRestrictions()`（`model/provider_token.go:155-163`），它有 5 个调用方。

- [ ] **Step 5: 运行 model 包全部测试**

Run: `go test ./model/ -v`

Expected: PASS。特别确认 `TestProviderTokenIsClientAllowedAllDisabled`（既有测试）仍然通过——它测的 `BlockClients: true` 行语义未变。

- [ ] **Step 6: 提交**

```bash
git add model/model_route.go model/provider_token.go model/provider_token_test.go
git commit -m "fix(model): unify client restriction semantics across route and token"
```

---

### Task 3: 选路阶段接上 token 层串行与检查

**Files:**
- Modify: `model/model_route.go:461-464`
- Test: `model/model_route_test.go`（追加）

- [ ] **Step 1: 写失败的集成测试**

追加到 `model/model_route_test.go` 末尾：

```go
func TestBuildRouteAttemptsAppliesTokenClientRestriction(t *testing.T) {
	setupModelRouteTestDB(t)

	provider := &Provider{
		Name:         "token-restriction-test",
		BaseURL:      "http://127.0.0.1:29002",
		ApiKey:       "test-key",
		ProviderType: ProviderTypeKeyOnly,
		Status:       common.UserStatusEnabled,
		Priority:     0,
		Weight:       10,
	}
	if err := DB.Create(provider).Error; err != nil {
		t.Fatal(err)
	}

	// 令牌层限制：只许 cc 客户端。路由层不设限制。
	token := &ProviderToken{
		ProviderId: provider.Id,
		SkKey:      "cc-only-token",
		Status:     common.UserStatusEnabled,
		AllowCC:    true,
	}
	if err := DB.Create(token).Error; err != nil {
		t.Fatal(err)
	}

	route := &ModelRoute{
		ModelName:       "token-restriction-model",
		ProviderTokenId: token.Id,
		ProviderId:      provider.Id,
		Enabled:         true,
		Priority:        0,
		Weight:          10,
	}
	if err := DB.Create(route).Error; err != nil {
		t.Fatal(err)
	}

	// cc 客户端：令牌与路由都放行 → 应选中
	attempts, err := BuildRouteAttemptsByPriority("token-restriction-model", "cc")
	if err != nil {
		t.Fatalf("expected cc client to reach the cc-only token: %v", err)
	}
	if len(attempts) == 0 || len(attempts[0]) == 0 {
		t.Fatal("expected at least one attempt for cc client")
	}

	// codex 客户端：令牌层拒绝 → 应无可用路由
	if _, err := BuildRouteAttemptsByPriority("token-restriction-model", "codex"); err == nil {
		t.Fatal("expected codex client to be rejected by token-level restriction")
	}

	// 泛客户端：令牌层保留给 cc，泛客户端也应被拒
	if _, err := BuildRouteAttemptsByPriority("token-restriction-model", ""); err == nil {
		t.Fatal("expected generic client to be rejected by token-level restriction")
	}
}

func TestBuildRouteAttemptsRequiresBothRouteAndTokenToAllow(t *testing.T) {
	setupModelRouteTestDB(t)

	provider := &Provider{
		Name:         "serial-and-test",
		BaseURL:      "http://127.0.0.1:29003",
		ApiKey:       "test-key",
		ProviderType: ProviderTypeKeyOnly,
		Status:       common.UserStatusEnabled,
		Priority:     0,
		Weight:       10,
	}
	if err := DB.Create(provider).Error; err != nil {
		t.Fatal(err)
	}

	// 令牌只许 codex，路由只许 cc —— 交集为空，任何客户端都进不来。
	token := &ProviderToken{
		ProviderId: provider.Id,
		SkKey:      "codex-only-token",
		Status:     common.UserStatusEnabled,
		AllowCodex: true,
	}
	if err := DB.Create(token).Error; err != nil {
		t.Fatal(err)
	}
	route := &ModelRoute{
		ModelName:       "serial-and-model",
		ProviderTokenId: token.Id,
		ProviderId:      provider.Id,
		Enabled:         true,
		Priority:        0,
		Weight:          10,
		AllowCC:         true,
	}
	if err := DB.Create(route).Error; err != nil {
		t.Fatal(err)
	}

	for _, clientType := range []string{"codex", "cc", ""} {
		if _, err := BuildRouteAttemptsByPriority("serial-and-model", clientType); err == nil {
			t.Fatalf("expected empty intersection to reject client %q", clientType)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./model/ -run "TestBuildRouteAttemptsAppliesTokenClientRestriction|TestBuildRouteAttemptsRequiresBothRouteAndTokenToAllow" -v`

Expected: 两个都 FAIL —
- 第一个报 "expected codex client to be rejected by token-level restriction"（token 层未接线，codex 被放过）
- 第二个报 "expected empty intersection to reject client \"codex\""（同因）

- [ ] **Step 3: 补上 token 层检查**

把 `model/model_route.go:461-464` 这段：

```go
		// Filter by client type restrictions
		if !route.IsClientAllowed(clientType) {
			continue
		}
```

替换为：

```go
		// Filter by client type restrictions.
		// 路由层（管理员手动禁用）与令牌层（线路自身限制）串行与：两层都放行才可用。
		if !route.IsClientAllowed(clientType) {
			common.SysLog(fmt.Sprintf("[client-restriction] route_id=%d model=%s client_type=%q rejected by route restriction", route.Id, route.ModelName, clientType))
			continue
		}
		if !token.IsClientAllowed(clientType) {
			common.SysLog(fmt.Sprintf("[client-restriction] route_id=%d token_id=%d model=%s client_type=%q rejected by token restriction", route.Id, token.Id, route.ModelName, clientType))
			continue
		}
```

- [ ] **Step 4: 确认 import 齐备**

`model/model_route.go` 需要 `fmt` 与 `NewAPI-Gateway/common`。检查文件顶部 import 块：

Run: `go build ./model/`

Expected: 编译通过。若报 `undefined: fmt`，在 import 块加 `"fmt"`；若报 `undefined: common`，加 `"NewAPI-Gateway/common"`。（`common` 极可能已存在——`model_route.go:452` 已用 `common.UserStatusEnabled`。）

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./model/ -v`

Expected: 全部 PASS，包括 Task 1/2 的测试和既有测试

- [ ] **Step 6: 全仓回归**

Run: `go test ./...`

Expected: 全部 PASS。若 `controller` 包有测试因路由变少而失败，说明该测试隐含依赖了 token 层不生效的旧行为——逐个检查断言，按新真值表更新期望值，不要为了让测试变绿而回退实现。

- [ ] **Step 7: 提交**

```bash
git add model/model_route.go model/model_route_test.go
git commit -m "fix(model): enforce token-level client restriction during route selection"
```

---

### Task 4: 更新文档

**Files:**
- Modify: `docs/client-restriction.md:34-43`

- [ ] **Step 1: 修正「路由过滤」小节并补真值表**

把 `docs/client-restriction.md:34-43`（从 `### 3. 路由过滤` 到 `过滤逻辑位于 ...` 那行）整段替换为：

```markdown
### 3. 路由过滤

在路由选择阶段 (`model/model_route.go:BuildRouteAttemptsByPriority()`)，系统会：

1. 从上下文获取客户端类型（由中间件提取）
2. 遍历候选路由时，**串行与**检查两层限制：
   - `route.IsClientAllowed(clientType)` —— 路由层（管理员手动禁用）
   - `token.IsClientAllowed(clientType)` —— 令牌层（线路自身限制）
   两层都放行才保留该路由；任一层拒绝都会记录 `[client-restriction]` 日志并跳过
3. 返回符合条件的路由列表供重试逻辑使用

两层的判定逻辑共用 `model/client_restriction.go:EvaluateClientRestriction()`，真值表如下：

| 勾选状态 | Codex 客户端 | CC 客户端 | 泛客户端（UA 未识别） |
|---|---|---|---|
| 都不勾 | ✅ | ✅ | ✅ |
| 仅 Codex | ✅ | ❌ | ❌ |
| 仅 CC | ❌ | ✅ | ❌ |
| Codex + CC | ✅ | ✅ | ❌ |
| 全禁用 | ❌ | ❌ | ✅ |

**语义要点：**
- 勾选 Codex/CC 表示「线路**保留给**指定客户端」，泛客户端同样算跑错客户端，一并拒绝
- 「全禁用」表示「线路**禁止**已识别客户端」，只放行泛客户端，与 Codex/CC 勾选互斥
- 未来新增的未识别客户端类型，在受限线路上默认拒绝
```

- [ ] **Step 2: 修正文档第 32 行的旧描述**

`docs/client-restriction.md:32` 现为：

```markdown
- 未识别的客户端（clientType 为空）始终可以使用未设置限制的令牌
```

替换为：

```markdown
- 未识别的客户端（clientType 为空）只能使用「未设置限制」或「全禁用」的令牌；勾选了 Codex/CC 的令牌会拒绝它
```

- [ ] **Step 3: 提交**

```bash
git add docs/client-restriction.md
git commit -m "docs: correct client restriction semantics and add truth table"
```

---

## 上线观测清单

这次改动**会减少可用路由**，两类流量可能开始报 `无可用的模型路由`：

1. 之前靠「令牌层限制不生效」蒙混过关的 codex/cc 流量
2. 打到勾了 Codex/CC 的线路上的泛客户端流量（curl、自研 SDK、非 Codex/CC 的第三方工具）

**上线后立即执行：**

- [ ] 观察日志里的 `[client-restriction]` 条目，确认被拒的路由/客户端组合符合预期
- [ ] 若某个模型完全无可用路由，检查是否有线路被误勾了 Codex/CC —— 按新语义，勾了就等于对泛客户端关门
- [ ] 泛客户端需要用受限线路时，正确做法是让它带上可识别的 User-Agent（`middleware/agg_token_auth.go:71-79` 匹配 `codex` / `claude-cli` / `claudecode` / `claude-code`），而不是取消线路限制

## 不在本计划范围内

- 协议转换（chat ↔ responses）—— 见 `docs/superpowers/plans/2026-08-20-responses-to-chat-translation.md`
- 前端 UI 调整 —— 现有三勾选框的交互（`web/src/components/ModelRoutesTable.js:268-278`）与新语义一致，无需改动
- `ProviderToken` 与 `ModelRoute` 限制字段的数据迁移 —— 字段本身不变，只改判定逻辑
