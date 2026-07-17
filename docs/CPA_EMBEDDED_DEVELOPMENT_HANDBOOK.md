# 内置 CPA 完整管理开发手册

> [返回文档中心](./README.md)

**最后更新：** 2026-07-17  
**目标读者：** 接手具体开发任务的 Opus 4.8 或其他高级编码代理  
**当前分支：** `feature/embedded-cpa-management`  
**工作树：** `E:\NewAPI-Gateway\.worktrees\embedded-cpa-management`

## 1. 目标与规范源

目标是在 Gateway 内完整控制内置 CPA，包括配置、认证文件导入/下载/修改/禁用/删除、OAuth、配额检查与重置、日志、API Key、Provider、插件以及生命周期管理。

实现必须复用 CPA 官方管理面板和 `/v0/management` API，不重新实现一套不完整的管理 UI，也不把 CPA 端口暴露给外部。

按以下优先级读取规范：

1. 用户约束和仓库 `AGENTS.md`。
2. [完整实施计划](superpowers/plans/2026-07-16-embedded-cpa-full-management.md)。每个 Task 的文件、测试、命令和提交消息以该文件为准。
3. [批准的架构设计](superpowers/specs/2026-07-16-embedded-cpa-full-management-design.md)。
4. CPA 官方管理 API 文档：<https://help.router-for.me/cn/management/api>。
5. 当前固定依赖 `github.com/router-for-me/CLIProxyAPI/v7 v7.2.80` 的实际源码。接口行为有疑问时必须查源码，禁止猜测。

本手册用于交接和导航，不替代实施计划。

### 1.1 用户要求的开发纪律

当前工作树没有物理 `AGENTS.md` 文件，因此接手新会话时必须从本手册继承用户明确给出的规则：

- 以瞎猜接口为耻，以认真查询为荣。
- 以模糊执行为耻，以寻求确认为荣。
- 以臆想业务为耻，以人类确认为荣。
- 以创造接口为耻，以复用现有为荣。
- 以跳过验证为耻，以主动测试为荣。
- 以破坏架构为耻，以遵循规范为荣。
- 以假装理解为耻，以诚实无知为荣。
- 以盲目修改为耻，以谨慎重构为荣。

实践含义：固定依赖能查到的接口必须查；计划与仓库冲突必须明确记录并选择保守行为；无法执行的验证必须如实报告，不能写成通过。

## 2. 当前进度

已完成并提交：

| Task | 内容 | 提交 |
| --- | --- | --- |
| 设计 | 内置 CPA 完整管理设计 | `db45143` |
| 计划 | 10 个 Task 的 TDD 实施计划 | `4c05003` |
| Task 1 | 完整 YAML 快照、迁移、恢复、原子落盘 | `b3f9658` |
| Task 2 | 安全的嵌入式 CPA 管理运行时 | `d2b0841` |
| Task 3 | 生命周期状态机、密码生命周期、请求排空 | `4fc515a` |

下一步从 **Task 4** 开始。不要重做 Task 1-3，也不要修改它们的公开契约，除非新测试证明存在缺陷。

Task 3 提交前的验证结果：

- `go test ./service/cpa -run 'TestManager' -count=10 -timeout 4m`：通过。
- `go test ./... -count=1 -timeout 8m`：通过。
- `go vet ./service/cpa`：通过。
- `git diff --check`：通过。
- `go test -race ...`：当前机器无法执行，因为 `CGO_ENABLED=0`。这只是环境限制，不可用普通测试冒充 race 测试通过。

### 2.1 已知计划与环境差异

| 项目 | 计划或期望 | 当前事实 | 执行规则 |
| --- | --- | --- | --- |
| `web/build` | Task 9/10 的 `git add` 示例包含该目录 | `web/.gitignore` 忽略 `/build`，Git 当前未跟踪任何 build 文件 | 必须执行 `npm run build` 供 Go embed/E2E 使用；未经明确策略变更，不使用 `git add -f` 强制提交 |
| race tests | 每个 backend Task 应跑 `-race` | 当前 `CGO_ENABLED=0`，Go 在编译前拒绝 | 跑普通测试继续开发，记录缺失，并在支持 CGO 的环境补跑 |
| `AGENTS.md` | 用户规则应约束所有代理 | 工作树和上级目录当前无物理文件 | 新会话必须先读本手册 1.1，不得假设规则不存在 |

如果后续仓库状态改变，应更新本表和对应实施计划，避免长期保留两套说法。

## 3. 已建立的核心契约

### 3.1 完整配置持久化

- Gateway option `CPAConfigYAML` 保存完整 CPA YAML。
- `SnapshotStore` 保留未知字段、插件节点、字段顺序和注释。
- 旧的 `CPAEnabled`、`CPAAPIKeys`、`CPAAuthDir`、`CPAPort` 只用于首次迁移和基础兼容接口。
- 运行时文件为 `$CPA_RUNTIME_DIR/config.yaml`；默认目录为进程工作目录下的 `cpa`。
- 启动恢复优先使用有效的磁盘文件，以覆盖“CPA 已修改磁盘但数据库持久化尚未完成”的中断窗口；否则从数据库快照恢复。
- YAML 拒绝重复键、alias 和多文档输入。
- 写盘必须原子替换，运行目录权限 `0700`，配置文件权限 `0600`。

### 3.2 Gateway 强制不变量

无论数据库、磁盘或管理 API 提交什么值，都必须强制：

```yaml
host: 127.0.0.1
remote-management:
  allow-remote: false
  disable-control-panel: true
  disable-auto-update-panel: true
  secret-key: <Gateway 生成的随机 bcrypt sentinel>
```

CPA 自带面板和自动更新必须关闭，Gateway 只提供固定版本、哈希校验后的面板。

### 3.3 管理密码

- 每次 CPA 启动生成 32 字节安全随机数并进行 base64url 编码。
- 真实密码只保存在 `Manager` 和单次服务端管理 lease 中，通过 `WithLocalManagementPassword` 注入 CPA。
- 密码不得出现在 API 响应、浏览器、本地存储、日志或格式化错误中。
- Stop、启动失败和异常退出后必须清除 Manager 中的密码与 target。
- `EmbedResult` 不得增加 `ManagementPassword` 字段。

CPA v7.2.80 有一个关键行为：即使使用 `WithLocalManagementPassword`，管理鉴权仍会先检查配置中的 `remote-management.secret-key`。因此配置里必须存在非空 bcrypt sentinel，否则真实管理密码会得到 `403 remote management key not set`。

另一个关键行为：CPA 的 `LoadConfig` 和 `ParseConfigBytes` 会把明文 `secret-key` 自动哈希。为了拒绝不安全的原始配置，`StartEmbedded` 必须在调用 CPA loader 之前检查原始 YAML AST 中的 bcrypt 值。

### 3.4 生命周期

固定状态：

```text
stopped -> starting -> running -> stopping -> stopped
                \-> error
```

固定类型和错误位于 `service/cpa/manager.go`：

- `State`、`Status`、`LifecycleHooks`
- `ErrTransitionConflict`
- `ErrUnavailable`
- `ManagementLease`
- `Manager.Start/Stop/Shutdown/Restart/StartFromDB/Status/AcquireManagement`

行为约束：

- lifecycle mutation 使用一个非阻塞 transition token；并发操作立即返回 `ErrTransitionConflict`。
- Start 最多等待健康检查 30 秒，成功后才原子发布 target、密码和 `running/ready`。
- Stop 先禁止新 lease，再等待已有 lease，排空上限 10 秒，然后停止 CPA，停止上限 35 秒。
- Stop 修改 `CPAEnabled=false`；Start 修改为 `true`；Restart 不修改期望状态；Shutdown 不修改期望状态。
- `Status.Endpoint` 仅在 running/ready 时返回内部 URL，其他状态固定为 `offline`。
- `Status.Version` 固定为 `v7.2.80`。
- inflight WaitGroup 按每次启动分代。排空超时后的旧 lease 不能阻塞下一运行代，也不能与新一代发生 Wait/Add 复用。
- `EmbedResult.Errors` 在 `Done` 关闭前关闭；异常退出 watcher 将状态设为 error，并立即撤销 target、密码和可接收状态。

### 3.5 目标模块图

```text
Root browser
  |
  +-- /cpa ---------------------------- React 生命周期工作区
  |                                      |
  |                                      +-- /api/cpa/status|start|stop|restart
  |                                      +-- /api/cpa/panel (iframe)
  |
  +-- /v0/management/* ---------------- RootAuth + NoTokenAuth + SameOrigin
  |                                      |
  |                                      v
  |                                  Controller
  |                                      |
  |                                      v
  |                               ManagementProxy
  |                                |     |      |
  |                         acquire lease |      +-- audit without secrets
  |                                |     +-- mutation -> PersistRuntime
  |                                v
  |                             Manager -------- LifecycleHooks
  |                                |                    |
  |                     target + runtime password       v
  |                                |          CPAProviderCoordinator
  |                                v                    |
  |                      127.0.0.1:<port> CPA           v
  |                                               runtime availability
  |                                                      |
  +------------------------------------------------------v
                                              Gateway route selection

Public OAuth provider callback
  +-- exact callback path -> OAuthRelay -> Manager lease -> loopback CPA
```

调用方向是单向的：Controller 读取默认 `Runtime`；Runtime 组合 Store、Manager、Proxy、OAuth 和 Panel；Proxy/OAuth 只能通过 `AcquireManagement` 获取临时 target 和密码；Manager 通过 `LifecycleHooks` 通知 Provider coordinator。不得让 CPA 包反向 import `controller`、`router` 或顶层 `service`，也不得让前端知道 loopback target 或真实密码。

### 3.6 文件职责与所有权

| 文件或模块 | 唯一职责 | 可以依赖 | 不得负责 |
| --- | --- | --- | --- |
| `service/cpa/snapshot.go` | 完整 YAML 验证、迁移、恢复、原子持久化 | option store、CPA config parser | 生命周期、HTTP proxy、Provider DB |
| `service/cpa/embed.go` | 从安全配置构建并运行一个 CPA SDK 实例 | CPA SDK、bcrypt | 持久化、全局状态、浏览器鉴权 |
| `service/cpa/manager.go` | 生命周期、就绪、运行时密码、target lease、排空 | SnapshotStore、EmbedResult、hooks | HTTP 路由、Provider DB 细节、面板 |
| `common/provider_runtime.go` | Provider ID 的进程内运行时可用性 | 标准库 | 修改 Provider 持久状态 |
| `service/cpa_provider.go` | 内置 Provider upsert、运行时可用性、同步防抖 | model、common、同步服务 | CPA 密码、HTTP management proxy |
| `service/cpa/management_proxy.go` | 完整 management HTTP 转发、header 清洗、mutation 持久化 | Manager lease、SnapshotStore | 生命周期状态转换、业务 endpoint 枚举 |
| `middleware/same-origin.go` | mutation 的 Gateway 同源校验 | Gin、request metadata | 用户角色鉴权、OAuth state |
| `service/cpa/oauth_relay.go` | 三个 callback 的 state/provider 校验与一次性转发 | CPA OAuth session API、Manager lease | wildcard proxy、Gateway session 登录 |
| `service/cpa/runtime.go` | 构造并原子注册唯一运行时对象 | 上述 CPA 组件 | 业务实现和额外状态机 |
| `controller/cpa.go` | Gin binding、调用 Runtime、稳定 HTTP 错误映射和审计 | Runtime | 直接访问 CPA loopback、保存密码 |
| `router/api-router.go` | 字面路由和 middleware 顺序 | controller、middleware | 业务判断、动态 wildcard callback |
| `service/cpa/panel.go` | 提供固定哈希的单文件官方面板 | `go:embed` | 运行时下载、CPA 内置面板更新 |
| `web/src/pages/CPA` | 生命周期状态和 iframe 工作区 | Gateway same-origin API | 调用 loopback CPA、存储真实密码 |

### 3.7 三条关键数据流

**启动流：** `Manager.Start` -> 持久化 `CPAEnabled=true` -> SnapshotStore 校验并物化 -> 创建 auth dir -> 生成一次性运行代密码 -> `StartEmbedded` -> `/healthz` 成功 -> 原子发布 target/password -> `OnCPAReady` -> coordinator upsert、标记 runtime available、同步模型。

**管理 mutation 流：** Root 浏览器 -> Gateway session/no-token/same-origin -> ManagementProxy 获取 lease -> 删除浏览器凭证 -> 注入 runtime password -> CPA 应用 mutation -> Gateway 在提交响应前执行 `PersistRuntime` -> 成功后安排防抖 Provider sync -> 释放 lease。CPA 已应用但持久化失败时返回 `persistence_failed`，不能伪装为成功。

**停止流：** `Manager.Stop` -> 状态改为 stopping 且拒绝新 lease -> `OnCPAUnavailable` 立即使 Provider 不可选 -> 等待当前运行代 lease -> cancel CPA -> 等待 Done -> 清除 target/password/current -> 持久化 `CPAEnabled=false`。`Shutdown` 使用相同运行时清理，但不修改 `CPAEnabled`。

### 3.8 实现期间的临时兼容状态

Task 3 为保证旧 `main.go` 和旧 controller 可编译，在 `manager.go` 底部保留了 package-level `StartFromDB`、`Stop`、`Reload`、`IsRunning` 薄包装。它们只是过渡设施：

- Task 4-6 不要扩展这些包装，不要向其中增加新状态。
- Task 7 建立 `Runtime` 并完成启动、controller 和 router 接线后，必须移除过渡状态和旧 callback 路径。
- Task 7 完成后的进程只能有一个 `Runtime.Manager`，不能同时保留 legacy Manager。

## 4. 剩余任务

必须按顺序执行，每个 Task 单独 RED、GREEN、全量回归和提交。

| Task | 交付内容 | 计划位置 | 提交消息 |
| --- | --- | --- | --- |
| 4 | Provider 运行时可用性注册表和 750ms 防抖同步 | `Task 4` | `feat(cpa): gate provider routes on runtime availability` |
| 5 | 完整 `/v0/management` 透明反向代理 | `Task 5` | `feat(cpa): proxy complete management API` |
| 6 | mutation 同源保护和三个精确 OAuth callback relay | `Task 6` | `feat(cpa): protect mutations and relay OAuth callbacks` |
| 7 | Runtime、Root controller、字面路由、兼容 API、启动/退出接线 | `Task 7` | `feat(cpa): expose Root lifecycle and management routes` |
| 8 | 固定并内嵌官方 Management Center v1.18.3 | `Task 8` | `feat(cpa): embed pinned official management panel` |
| 9 | Root-only `/cpa` 页面、导航、iframe 和响应式布局 | `Task 9` | `feat(cpa): add Root management workspace` |
| 10 | 真实 CPA 集成、Playwright 桌面/移动端和完整回归 | `Task 10` | `test(cpa): verify full embedded management workflow` |

### 4.1 任务依赖和停点

```text
Task 1-3 complete
       |
       v
Task 4 Provider runtime state
       |
       v
Task 5 Management proxy
       |
       v
Task 6 Same-origin + OAuth relay
       |
       v
Task 7 Runtime + controller + routes + startup
       |
       v
Task 8 Pinned panel asset
       |
       v
Task 9 Root frontend workspace
       |
       v
Task 10 Real integration + browser acceptance
```

不要并行跨 Task 改同一组文件。Task 5 依赖 Task 3 的 lease；Task 6 复用 Task 5 的 `managementLeaseProvider`；Task 7 才把 Task 4-6 组合成唯一 Runtime；Task 9 依赖 Task 7 的 API 和 Task 8 的 panel；Task 10 只做真实链路验证和测试设施，不应再发明生产接口。

### 4.2 Task 4 作业卡：Provider 运行时可用性

**前置条件：** HEAD 历史包含 `4fc515a`；`go test ./common ./model ./service` 基线通过。

**文件边界：**

- 新建 `common/provider_runtime.go`、`common/provider_runtime_test.go`。
- 修改 `model/model_route.go`、`model/model_route_test.go`。
- 重写 `service/cpa_provider.go`、`service/cpa_provider_test.go`。
- 修改 `service/cpa_provider_integration_test.go`。

**测试拆分：**

1. Registry：未注册 ID 默认 available；显式 false 不可用；clear 后恢复默认。
2. Route selection：持久状态都正常但 runtime false 的 Provider 被跳过；恢复 true 后使用原 token。
3. Coordinator：已有 `__embedded_cpa__` 的 `Status/Priority/Weight` 不变；ready 后 runtime available；三次快速 schedule 只同步一次；unavailable 立即标记 false 并取消 timer。
4. Close：关闭 coordinator 后 timer 不再触发，测试 cleanup 不泄漏 goroutine。

**正确 RED：** 缺少 registry API、route selector 仍选择 runtime unavailable Provider、旧 CPA callback 没有防抖 coordinator。若 RED 是数据库 fixture、导入环或计时不稳定，先修测试，不得进入实现。

**推荐实现顺序：**

1. 用 `sync.Map` 实现通用 registry；默认值必须是 true。
2. 在 `BuildRouteAttemptsByPriority` 已有持久 Provider/token 状态判断旁增加 runtime guard，不改变其他排序逻辑。
3. 把当前单次 callback 重构为 `CPAProviderCoordinator`，upsert 只更新 `BaseURL`、`ApiKey`、`ProviderType`、`CheckinEnabled`。
4. `OnCPAReady` 先发布 available，再做一次立即同步；失败不得把操作员期望状态改成 disabled。
5. `ScheduleCPASync` 使用 750ms timer 合并调用；持锁区只管理状态，不在锁内执行慢同步。
6. `OnCPAUnavailable` 取消 timer，并对已知 ID 或按名称查到的内置 Provider 设置 unavailable。

**验证命令：**

```powershell
go test ./common ./model ./service -run 'Test(ProviderRuntime|BuildRouteAttemptsSkipsRuntime|CPACoordinator)' -count=1
go test ./common ./model ./service -run 'Test(ProviderRuntime|BuildRouteAttempts|CPACoordinator|RegisterEmbeddedCPAProvider)' -count=1
go test ./... -count=1 -timeout 8m
git diff --check
```

支持 CGO 时，将第二条改为 `go test -race ...`。当前机器必须记录 race 未执行。

**完成门槛：** 普通 Provider 行为零变化；CPA 停止后不参与路由；数据库 Provider 的期望字段保留；防抖测试稳定；只提交计划列出的七个文件。

**禁止事项：** 不通过设置 `Provider.Status=disabled` 表示临时离线；不修改 route `enabled`；不在 `common` registry 中写数据库；不删除现有 operator tuning。

### 4.3 Task 5 作业卡：完整 Management Proxy

**前置条件：** Task 4 已提交；Manager lease 的 target/password/release 契约保持不变。

**文件边界：** 只新建 `service/cpa/management_proxy.go` 和对应测试。

**测试拆分：**

1. 方法与载荷表：GET、POST、PUT、PATCH、DELETE；JSON、YAML、multipart；path 和 raw query 原样转发。
2. 凭证清洗：浏览器的 Authorization、X-Management-Key、Cookie、Proxy-Authorization 和 hop-by-hop headers 全部消失，只剩 Gateway 注入的 `Bearer <runtime password>`。
3. 响应透传：CPA 的业务 4xx/5xx、body、Content-Type、Content-Disposition、版本/commit/build/plugin headers 原样返回。
4. Streaming：下载不做全量缓存；lease 在 body 完成复制后才 Release。
5. Mutation durability：CPA 2xx 后先阻塞于 `PersistRuntime`，响应不能提前 commit；持久化成功后 schedule 一次 sync。
6. Persistence failure：CPA 已应用但保存失败，返回固定 500 `persistence_failed`，不能透传原 CPA success。
7. Transport mapping：无 lease -> 503；connection refused -> 502；response header timeout -> 504。
8. OAuth polling：只有成功且 completed 的 `GET /v0/management/get-auth-status` 才 schedule sync，buffer 上限 1 MiB 后恢复 body。
9. Audit：包含 Root username、method、规范化 path、status、duration；不包含 raw query、body、header secret、OAuth code 或上传内容。

**正确 RED：** `ManagementProxy` 类型不存在。不要先建空 handler 让测试变成无意义的状态码失败。

**推荐实现顺序：**

1. 定义最小 `managementLeaseProvider` 接口和 constructor injection。
2. 克隆 `http.DefaultTransport`，设置 10s dial/TLS、30s response-header timeout，不设置 whole-response timeout。
3. 每个请求 acquire 一次 lease；用 per-request `httputil.ReverseProxy` Director 重写 scheme/host 并清洗 headers。
4. 在 `ModifyResponse` 中处理 mutation persistence 和 auth-status 小响应；不要在 handler 返回后异步持久化。
5. 使用 typed `persistenceError` 让 `ErrorHandler` 区分 durability、timeout 和普通 transport failure。
6. 审计从 typed context key 读取 username，只记录允许字段。

**验证命令：**

```powershell
go test ./service/cpa -run 'TestManagementProxy' -count=1 -v
go test ./service/cpa -count=1 -timeout 4m
go test ./... -count=1 -timeout 8m
git diff --check
```

**完成门槛：** 测试明确证明响应提交顺序、流式 lease 生命周期、header 清洗、业务错误透传和审计脱敏；提交中只有 proxy 两个文件。

**禁止事项：** 不枚举 CPA management endpoint；不缓存上传/下载用于日志；不把 username 转成 upstream header；不把 CPA 4xx 统一改成 Gateway 错误；不在浏览器 header 上追加真实密码。

### 4.4 Task 6 作业卡：Same-Origin 与 OAuth Relay

**前置条件：** Task 5 已提交；确认 CPA v7.2.80 的 OAuth session 导出 API 实际签名。

**文件边界：**

- 新建 `middleware/same-origin.go`、`middleware/same-origin_test.go`。
- 新建 `service/cpa/oauth_relay.go`、`service/cpa/oauth_relay_test.go`。

**Same-Origin 测试矩阵：**

- GET、HEAD、OPTIONS 无 Origin/Referer 也通过。
- mutation 优先验证 Origin；Origin 缺失时才回退 Referer。
- scheme、hostname、effective port 完全匹配；host 大小写归一化，默认 80/443 等价。
- request scheme 优先 `TLS`，其次单个可信 `X-Forwarded-Proto`，最后 http。
- 缺失、畸形、多值、foreign Origin/Referer 返回 403 `origin_rejected`。

**OAuth Relay 测试矩阵：**

- 只接受 `/anthropic/callback`、`/codex/callback`、`/antigravity/callback`。
- state 格式有效、session 存在、provider 匹配、error status 为空才能转发。
- state 在转发前原子 claim；并发或重复 callback 只有一次成功。
- claimed state 保留 31 分钟并机会式清理。
- CPA stopped 返回 503；callback 不携带 Gateway Cookie、management password 或浏览器 auth header。
- path/query 原样转发，但 audit 不记录 state、code、query 或 body。

**正确 RED：** middleware 和 relay 类型不存在。若测试依赖真实外部 OAuth 服务则 fixture 设计错误；测试必须使用 CPA session state 和本地 upstream。

**推荐实现顺序：**

1. 先完成纯 middleware 的 URL 归一化和 table tests。
2. 定义 literal callback-to-provider map，不允许 prefix 或 wildcard 匹配。
3. 按 `ValidateOAuthState` -> `GetOAuthSession` -> provider/error 检查 -> atomic claim 的顺序处理。
4. claim 成功后 acquire lease，构造无敏感 header 的 upstream request，复制响应后 Release。
5. 错误响应使用稳定 Gateway JSON；CPA 正常 callback status/body 透传。

**验证命令：**

```powershell
go test ./middleware ./service/cpa -run 'Test(SameOrigin|OAuthRelay)' -count=1 -v
go test ./middleware ./service/cpa -count=1 -timeout 4m
go test ./... -count=1 -timeout 8m
git diff --check
```

**完成门槛：** 三个 literal callback、一次性 state、同源端口归一化、stopped 映射和日志脱敏都有测试；只提交四个新文件。

**禁止事项：** 不给 callback 加 RootAuth；不接受 arbitrary provider path；不把 state 仅当作非空字符串；不在 claim 前转发；不信任逗号分隔或多值 `X-Forwarded-Proto`。

### 4.5 Task 7 作业卡：唯一 Runtime、Root API 与启动接线

**前置条件：** Task 4-6 已提交并各自通过包级测试。

**文件边界：** 新建 `service/cpa/runtime.go` 和测试；重写 `controller/cpa.go` 并建测试；修改 `router/api-router.go` 并建 route tests；修改 `main.go`。

**先写的测试：**

1. Runtime：Store/Manager/Proxy/OAuth/Panel 均非 nil；`SetDefaultRuntime` 原子替换；并发读取 race-safe。
2. Controller：status 不含密码；start/stop/restart 映射成功、409、503、500；legacy basic patch 保留未知/plugin YAML；reload 只调用 restart。
3. Route auth：真实 Gin session chain 下覆盖 Root、Admin、user token、anonymous 和 foreign Origin。
4. Audit：生命周期行包含 username/action/status/duration，不包含提交 API key、auth dir 内容、runtime password 或未脱敏 error。

**推荐实现顺序：**

1. 构造 `Runtime{Store, Manager, Proxy, OAuth, Panel}`；Task 8 前 Panel 用 `http.NotFoundHandler()`。
2. 用 `atomic.Pointer[Runtime]` 实现唯一默认 Runtime。
3. Controller 只从 `DefaultRuntime()` 获取组件，不再调用 package-level legacy lifecycle 包装。
4. `UpdateCPAConfig` 只 bind/validate 四个兼容字段，调用 `PatchBasic` 后 `Manager.Restart`；不要重建最小 YAML。
5. Router 使用计划给出的 literal groups 和 middleware 顺序。
6. `main.go` 在 `model.InitOptionMap()` 后创建 coordinator 和 Runtime，在 router 注册前 `SetDefaultRuntime` 并 `StartFromDB`；defer 顺序保证 `Manager.Shutdown` 和 coordinator `Close`。
7. 删除 Task 3 的 legacy Manager 状态与旧 provider callback adapter，确认进程只有一个 Manager。

**固定路由：**

```text
GET  /api/cpa/status
POST /api/cpa/start
POST /api/cpa/stop
POST /api/cpa/restart
GET  /api/cpa/panel
GET  /api/cpa/config
PUT  /api/cpa/config
POST /api/cpa/reload
ANY  /v0/management
ANY  /v0/management/*path
GET  /anthropic/callback
GET  /codex/callback
GET  /antigravity/callback
```

**验证命令：**

```powershell
go test ./service/cpa ./controller ./router -run 'Test(Runtime|CPA|LegacyConfig|Lifecycle)' -count=1 -v
go test ./controller ./router ./service/cpa -count=1 -timeout 4m
go test ./... -count=1 -timeout 8m
git diff --check
```

**完成门槛：** route authorization matrix 全过；legacy API 保留；未知 YAML 不丢；startup/shutdown 使用同一 Runtime；临时 legacy lifecycle 状态删除；只提交计划列出的文件。

**禁止事项：** 不添加 generic `/cpa/*` upstream proxy；不代理 `/v0/resource`；不把 management route 放进 `/api` group；不把 OAuth callback 放进 RootAuth；不在 controller 缓存密码或 target。

### 4.6 Task 8 作业卡：固定官方面板

**前置条件：** Task 7 Runtime 已存在，Panel 当前为 404 placeholder。

**固定资产：**

```text
Version: v1.18.3
URL: https://github.com/router-for-me/Cli-Proxy-API-Management-Center/releases/download/v1.18.3/management.html
SHA-256: 941a49a619a719a59e4c7917c6888a53eb3f41a4fa2fbb5c1cc94f2d1fc9cd4b
```

**文件边界：** 新建 `service/cpa/assets/management.html`、`panel.go`、`panel_test.go`；修改 `runtime.go`。

**执行顺序：**

1. 先写 asset exact SHA-256 和 handler header 测试并确认 RED。
2. 从固定 URL 下载到目标文件：

```powershell
New-Item -ItemType Directory -Force service/cpa/assets | Out-Null
Invoke-WebRequest -UseBasicParsing 'https://github.com/router-for-me/Cli-Proxy-API-Management-Center/releases/download/v1.18.3/management.html' -OutFile 'service/cpa/assets/management.html'
$panelHash = (Get-FileHash 'service/cpa/assets/management.html' -Algorithm SHA256).Hash.ToLower()
if ($panelHash -ne '941a49a619a719a59e4c7917c6888a53eb3f41a4fa2fbb5c1cc94f2d1fc9cd4b') {
    throw "management panel hash mismatch: $panelHash"
}
```

3. 立即计算 SHA-256；不匹配则停止、删除不匹配资产，不允许继续实现。
4. 用 `//go:embed` 嵌入 bytes，定义固定 version/hash const。
5. Handler 返回 `text/html; charset=utf-8`、`Cache-Control: no-store`、`nosniff` 和计划指定 CSP，其中 `frame-ancestors 'self'`。
6. `NewRuntime` 使用 `NewPanelHandler()` 替换 placeholder。

**验证命令：**

```powershell
go test ./service/cpa -run 'Test(EmbeddedManagementPanel|PanelHandler)' -count=1 -v
(Get-FileHash 'service/cpa/assets/management.html' -Algorithm SHA256).Hash.ToLower()
go test ./... -count=1 -timeout 8m
git diff --check
```

**完成门槛：** 测试和命令均输出精确固定哈希；运行时不联网；panel header 稳定；提交只含资产、panel 文件和 runtime 修改。

**禁止事项：** 不格式化或手工改上游 HTML；不使用 `go generate` 在线拉取；不回退到 CPA 自带 control panel；不接受“接近”的哈希。

### 4.7 Task 9 作业卡：Root 前端工作区

**前置条件：** Task 7 API 和 Task 8 panel 已提交；先检查现有 `Layout`、`UserContext`、API helper 和 CSS 变量。

**文件边界：** 新建 `RootRoute` 和 CPA page 及测试；修改 `Layout.js`、其测试、`App.js`、`index.css`。

**测试拆分：**

1. RootRoute：role 1、10 被拒绝，role 100 看到 children；context 优先，localStorage fallback 对坏 JSON 安全。
2. Navigation：只有 Root 显示 `/cpa`，Admin 仍保留原 admin 导航但没有 CPA 项。
3. Status states：running、stopped、starting、stopping、error 均有确定 UI；action failure 显示 alert；polling unmount 后停止。
4. Panel bootstrap：删除旧 `cli-proxy-auth`，设置当前 origin、`managementKey=gateway-managed`、`isLoggedIn=true`；DOM 和 storage 无真实密码。
5. Actions：stopped 时 iframe 不挂载且 Start 调 `/api/cpa/start`；transition/in-flight 时全部 action disabled；running+ready 才挂载 `/api/cpa/panel`。

**实现顺序：**

1. 实现 RootRoute，并在 App 中 lazy route。
2. 在 Layout 增加 `Cpu` icon 项，以独立 `root` 条件过滤，不复用 admin 判断。
3. CPA page 首次和每 2 秒读取 status；effect cleanup 清 timer/忽略过期响应。
4. 实现 start/stop/restart action map、busy 状态和稳定生命周期栏。
5. 只在挂载 iframe 前执行 harmless session bootstrap。
6. 添加计划中的稳定尺寸和移动端 CSS；复用变量，检查按钮文字、图标和最长错误信息不溢出。

**验证命令：**

```powershell
Set-Location web
$env:CI='true'
npm test -- --runInBand --watchAll=false --testMatch='**/?(*.)+(spec|test).js'
npm run build
Set-Location ..
git diff --check
```

可先用计划给出的三个目标测试文件做 RED/GREEN，再跑全套。隐藏 worktree 中不要使用无法发现测试的默认 CRA absolute `testMatch`。

**完成门槛：** Root-only route/nav、五种状态、poll cleanup、placeholder storage、action 调用和 iframe 条件全部自动测试；production build 无新增警告；桌面/移动 CSS 有稳定尺寸。

**禁止事项：** 不在浏览器请求或 storage 写真实密码；不加载 loopback URL；不做第二套 CPA 功能 UI；不做 landing page；不使用嵌套卡片；不让 viewport width 直接缩放字体。

`npm run build` 是本 Task 的验证前置，但当前 build 目录被忽略且未跟踪。不要为了机械匹配计划中的 `git add web/build` 使用 `-f`；提交源文件和测试即可，除非用户明确改变产物提交策略。

### 4.8 Task 10 作业卡：真实集成与浏览器验收

**前置条件：** Task 4-9 全部独立提交；工作树干净；`web/build/index.html` 可生成。

**文件边界：** 新建 `service/cpa/full_management_integration_test.go`；修改 `web/package.json`、`web/package-lock.json`；新建 `web/playwright.config.js`、`web/e2e/start-gateway.ps1`、`web/e2e/cpa-management.spec.js`。`web/build` 必须生成用于验证，但按 2.1 的当前仓库规则不强制提交。

**Backend 真实集成顺序：**

1. 启动真实 CPA，但所有 management 操作通过 Gateway `ManagementProxy` client 发起。
2. GET config -> PATCH debug -> multipart upload auth file -> list -> download -> status patch -> fields patch -> delete。
3. 使用导出的 core auth manager fixture 构造 exceeded quota，POST reset-quota 后直接检查 auth/model retry/quota state 被清除。
4. 修改 SnapshotStore 中 port，调用 Manager.Restart，断言 proxy target 改变且 debug 配置仍持久。
5. Stop 后同一 Gateway client 请求 config，断言 503 `cpa_unavailable`。
6. plugin upload 仅在 pinned CPA 声明支持时验证 native loading；无支持标志不能把依赖能力误判为 Gateway 失败。

**Playwright 环境：**

- 安装 `@playwright/test` 和 Chromium，只在 Task 10 修改 lockfile。
- webServer PowerShell 脚本在 `%TEMP%/newapi-cpa-e2e-*` 创建独立工作目录、数据库、exe 和 `CPA_RUNTIME_DIR`。
- 服务固定 `127.0.0.1:3031`，前台运行；finally 删除前验证目标仍位于 TEMP 且目录名前缀正确。
- `reuseExistingServer=false`，避免误连开发者已有服务。

**Browser 必验：**

1. 使用 `page.context().request` 登录 Root，让 cookie 与页面 context 相同。
2. `/cpa` Start 后变 running，iframe body 非空并产生成功 management response。
3. 浏览器 request headers 最多包含 harmless `gateway-managed`，永不含 runtime password。
4. localStorage 只允许 placeholder；performance resource URL 无真实密码。
5. role 10 访问 `/cpa` 被重定向；foreign Origin PUT management 返回 403。
6. desktop 和 Pixel 7 下 lifecycle bar 可见、控件不重叠、iframe bounding box 宽高为正。
7. 两个 project 均截图，并人工查看不是空白、第二登录页或遮挡状态。

**完整验证顺序：**

```powershell
go test ./service/cpa -run 'TestFullManagementRoundTripAgainstRealCPA' -count=1 -v -timeout 3m
go test ./... -count=1 -timeout 8m
Set-Location web
$env:CI='true'
npm test -- --runInBand --watchAll=false --testMatch='**/?(*.)+(spec|test).js'
npm run build
npx playwright install chromium
npx playwright test
Set-Location ..
git diff --check
```

在支持 CGO 的环境补跑计划列出的全包 race 命令。完成后执行本手册安全扫描，并逐项勾选实施计划 Completion Checklist。

**完成门槛：** 真实 CPA management round-trip、quota fixture、restart target、stopped 503、前端单测、build、desktop/mobile Playwright 和人工截图检查均有证据；测试日志不含 fixture token、runtime password、OAuth code 或上传 body。

**禁止事项：** 不访问真实外部账号；不绕过 proxy 完成 management 断言；不复用仓库数据库；不删除未验证路径；不因插件 native loading 不受支持而跳过 header/route 透传验证。

## 5. 每个 Task 的标准工作流

### 5.1 开始前门禁

1. 进入隔离工作树，确认分支、提交历史和状态。

```powershell
Set-Location 'E:\NewAPI-Gateway\.worktrees\embedded-cpa-management'
git branch --show-current
git log -5 --oneline
git status --short
```

2. 确认当前 Task 的前置提交存在，上一个 Task 没有未提交文件。
3. 阅读完整计划中当前 Task 的全部步骤、文件列表和 expected RED/GREEN。
4. 阅读所有将修改文件及其直接消费者；CPA 行为有疑问时查询固定版本依赖源码。
5. 先运行当前受影响包的基线测试。基线失败时先记录并定位，不能把既有失败混入新功能。

### 5.2 RED 证据门禁

1. 只添加当前行为的测试和必要 fixture，不添加生产实现。
2. 运行计划指定 focused command。
3. 记录失败测试名和核心错误，确认失败原因是目标能力不存在。
4. 若失败来自 typo、编译 fixture、端口冲突、测试发现或环境依赖，修复测试并重跑，直到出现正确 RED。
5. 若测试立即通过，说明断言覆盖了既有行为或没有命中生产路径；重新设计测试，不能直接进入 GREEN。

建议在工作记录中保留：

```text
RED command:
Expected missing behavior:
Observed failing test:
Failure reason verified:
```

### 5.3 GREEN 与重构门禁

1. 写使当前 RED 通过的最小生产实现，不顺手进入下一 Task。
2. 重跑同一个 focused test，确认目标测试 GREEN。
3. 跑当前 package 全套测试，发现回归立即处理。
4. 只在 GREEN 后消除明显重复、改善命名或拆分复杂块；每次重构后重跑 focused test。
5. 新增的失败分支、并发分支和安全边界必须有对应测试，不能仅靠 code review。

### 5.4 验证梯度

按风险从小到大执行，不用较小范围通过推断较大范围通过：

```text
focused test
  -> modified package tests
  -> directly affected package tests
  -> go test ./...
  -> frontend tests/build when applicable
  -> integration/Playwright when applicable
  -> diff/security scans
```

每条命令必须检查 exit code 和失败数。当前机器无法执行 race 时，报告准确命令和 `CGO_ENABLED=0` 原因，并把补跑列为待办。

### 5.5 提交门禁

提交前执行：

```powershell
git status --short
git diff --check
git diff --stat
git diff
```

逐项确认：

- diff 只包含当前 Task 计划文件；生成物仅在计划明确要求时提交。
- 没有调试日志、真实 token、临时端口、fixture body 或本机绝对临时路径。
- 没有改依赖版本、格式化无关目录或更新不相关 metadata。
- 提交消息与计划完全一致。
- 提交后 `git status --short` 为空，再报告提交号。

### 5.6 固定依赖接口查询方法

不要凭记忆写 CPA API。先让 Go 返回当前锁定 module 的真实目录，再搜索导出 builder、鉴权、route 和 handler：

```powershell
$cpaDir = go list -m -f '{{.Dir}}' github.com/router-for-me/CLIProxyAPI/v7
Write-Output $cpaDir
rg -n 'WithLocalManagementPassword|AuthenticateManagementKey|RegisterOAuthSession|ValidateOAuthState|GetOAuthSession' $cpaDir --glob '*.go'
rg -n 'v0/management|auth-files|reset-quota|get-auth-status|oauth-callback' $cpaDir --glob '*.go'
```

查询后必须读取命中的完整函数和直接调用者，不能只根据一行 grep 结果推断。优先复用导出 SDK 类型；只有固定依赖没有导出能力且批准计划明确要求时，才考虑其他方案。将影响设计的依赖行为写入测试名或注释，例如“local password 仍要求 configured sentinel”。

出现测试失败或意外行为时，先定位根因和依赖源码，再修改。不要通过放宽断言、删除测试或添加无依据兼容分支来“修绿”。

## 6. 跨层契约矩阵

### 6.1 路由与权限矩阵

| 路由 | Root session | Admin session | User API token | Anonymous | Mutation 同源 |
| --- | --- | --- | --- | --- | --- |
| `GET /api/cpa/status` | 允许 | 拒绝 | 拒绝 | 401 | 不要求 |
| `POST /api/cpa/start` | 允许或 409 | 拒绝 | 拒绝 | 401 | 必须 |
| `POST /api/cpa/stop` | 允许或 409 | 拒绝 | 拒绝 | 401 | 必须 |
| `POST /api/cpa/restart` | 允许或 409 | 拒绝 | 拒绝 | 401 | 必须 |
| `GET /api/cpa/panel` | 允许 | 拒绝 | 拒绝 | 401 | 不要求 |
| `GET /api/cpa/config` | 允许 | 拒绝 | 拒绝 | 401 | 不要求 |
| `PUT /api/cpa/config` | 允许 | 拒绝 | 拒绝 | 401 | 必须 |
| `POST /api/cpa/reload` | 允许 | 拒绝 | 拒绝 | 401 | 必须 |
| `GET /v0/management/*` | 允许并透传 CPA | 拒绝 | 拒绝 | 401 | 安全方法不要求 |
| mutation `/v0/management/*` | 允许并透传 CPA | 拒绝 | 拒绝 | 401 | 必须 |
| 三个精确 OAuth callback | 按有效 state | 按有效 state | 按有效 state | 按有效 state | GET，不要求 |

`RootAuth()` 必须先于 `NoTokenAuth()`，否则 middleware 没有 `authByToken` 上下文。OAuth callback 不能要求 Gateway session，因为外部 provider 回跳通常不携带该 cookie；安全性来自 pending state/provider/one-time claim。

### 6.2 生命周期状态矩阵

| State | Ready | Endpoint | Management lease | Provider runtime | iframe |
| --- | ---: | --- | --- | --- | --- |
| `stopped` | false | `offline` | 拒绝 | unavailable | 不挂载 |
| `starting` | false | `offline` | 拒绝 | unavailable | 不挂载 |
| `running` | true | loopback URL | 允许 | available | 挂载 |
| `stopping` | false | `offline` | 新请求拒绝，旧请求排空 | unavailable | 移除 |
| `error` | false | `offline` | 拒绝 | unavailable | 不挂载 |

`Status.Enabled` 表示下一次启动的持久化期望，不等同于 State。`enabled=true,state=error` 是合法状态，表示期望运行但启动或运行失败；不要自动把 enabled 改成 false。

### 6.3 Gateway 稳定错误矩阵

| 场景 | HTTP | `code` | 责任层 |
| --- | ---: | --- | --- |
| 无 Gateway session | 401 | 复用现有 auth 响应 | auth middleware |
| 非 Root 或 token auth | denied | 复用现有 auth/no-token 响应 | auth middleware |
| foreign/missing mutation origin | 403 | `origin_rejected` | SameOrigin |
| lifecycle transition 正在进行 | 409 | `transition_conflict` | controller |
| CPA stopped/starting/error，无 lease | 503 | `cpa_unavailable` | proxy/controller/relay |
| loopback connection failure | 502 | `upstream_failure` | ManagementProxy |
| upstream response-header timeout | 504 | `upstream_timeout` | ManagementProxy |
| CPA mutation 成功但快照保存失败 | 500 | `persistence_failed` | ManagementProxy |
| 其他 lifecycle 失败 | 500 | `lifecycle_failed` | controller |
| CPA 业务 validation 4xx/5xx | 原状态 | 原 body | CPA，Gateway 原样透传 |

Gateway-generated JSON 至少包含 `success:false`、稳定 `code` 和无敏感信息的 `message`。不要把底层 error 原文直接返回，除非已经通过 runtime secret redaction 且计划允许。

### 6.4 请求 Header 处理矩阵

| Browser incoming header | 转发给 CPA | 处理 |
| --- | --- | --- |
| `Authorization` | 否 | 删除后设置 `Bearer <runtime password>` |
| `X-Management-Key` | 否 | 删除，不重用浏览器 placeholder |
| `Cookie` | 否 | 只用于 Gateway session，upstream 必须删除 |
| `Proxy-Authorization` | 否 | 删除 |
| `Connection`、`Proxy-Connection` | 否 | 删除 |
| `Keep-Alive`、`TE`、`Trailer` | 否 | 删除 |
| `Transfer-Encoding`、`Upgrade` | 否 | 删除 |
| `Content-Type` | 是 | 原样保留 JSON/YAML/multipart |
| `Accept`、条件下载 header | 是 | 非敏感 end-to-end header 保留 |

返回方向保留业务所需的 `Content-Type`、`Content-Disposition`、`X-CPA-VERSION`、`X-CPA-COMMIT`、`X-CPA-BUILD-DATE`、`X-CPA-SUPPORT-PLUGIN`。标准 reverse proxy hop-by-hop 清理仍适用。

### 6.5 持久化与同步矩阵

| 请求/事件 | PersistRuntime | ScheduleCPASync | 备注 |
| --- | ---: | ---: | --- |
| management GET | 否 | 否 | 纯读取 |
| 成功 POST/PUT/PATCH/DELETE | 是，同步且响应提交前 | 持久化成功后 | CPA 非成功状态不保存 |
| mutation 持久化失败 | 已尝试 | 否 | 返回 `persistence_failed` |
| completed `get-auth-status` | 否 | 是 | OAuth credential 可能已产生 |
| pending/error `get-auth-status` | 否 | 否 | 不做无效同步 |
| OAuth callback relay | 否 | 由完成状态路径触发 | callback 自身不直接猜完成 |
| Manager OnCPAReady | 否 | 一次立即同步 | 先 runtime available |
| Manager OnCPAUnavailable | 否 | 取消待执行 timer | 立即 runtime unavailable |

### 6.6 敏感数据边界

| 数据 | 允许存在的位置 | 禁止位置 |
| --- | --- | --- |
| bcrypt management sentinel | DB 完整 YAML、runtime config、CPA config memory | browser、日志、API 明文说明 |
| runtime management password | Manager、当前 lease、CPA builder、upstream Authorization | DB、磁盘、status、browser、audit |
| harmless `gateway-managed` | browser localStorage/request placeholder | 不得被当作真实 upstream credential |
| CPA proxy API key | Root basic config、完整 YAML、Provider connection | 普通用户、audit、错误日志 |
| credential upload/download body | browser 与 CPA 之间的认证流 | Gateway log、DB 副本、审计内容 |
| OAuth state/code | callback request、CPA pending session、短期 claim map | audit、错误 message、资源 URL 检查输出 |
| Root username | Gateway session、typed audit context、audit identity | upstream header |

### 6.7 完整管理能力覆盖矩阵

Gateway 不逐个实现 CPA endpoint，而是通过完整 wildcard bridge 保持能力覆盖。下表用于验收代表性功能，不是代理 allowlist。

| 能力 | 代表路径或事件 | Gateway 责任 | Task 10 证据 |
| --- | --- | --- | --- |
| 生命周期 | `/api/cpa/start|stop|restart|status` | Root-only、状态机、排空、稳定错误 | 启动、端口变更 restart、停止后 503 |
| 完整配置 | `/v0/management/config` 及配置字段 endpoint | 原样代理、成功 mutation 后保存完整 YAML | GET config、PATCH debug、restart 后仍为 true |
| 基础兼容配置 | `/api/cpa/config`、`/reload` | 只 patch 四字段并保留未知/plugin 节点 | controller unit tests |
| 认证文件导入 | multipart `/v0/management/auth-files` | 不记录 body、保持 Content-Type、持久化/同步 | upload 后 list 可见 |
| 认证文件下载 | auth-file download endpoint | 流式透传 body、Disposition、Content-Type | 下载内容一致且 lease 生命周期正确 |
| 认证文件修改 | status、fields endpoint | PATCH 原样代理、成功后保存并 schedule sync | disabled false、label 更新 |
| 认证文件删除 | auth-files DELETE | query 原样保留、成功后保存并 sync | 删除后成功响应 |
| OAuth | management 启动/状态 + 三个 public callback | Root 创建、state/provider/one-time relay、完成后 sync | relay unit tests + browser management response |
| 配额管理 | `/v0/management/reset-quota` | JSON 原样代理，不解释 provider 业务 | exported core auth fixture 状态完全清除 |
| 日志与使用量 | `/v0/management/*` 对应 read/download | 业务状态/header/body 原样透传 | proxy table 与真实 dashboard 读取 |
| API Key 管理 | `/v0/management/*` 对应 mutation | mutation durability 和审计脱敏 | wildcard proxy tests；dashboard smoke |
| Provider 管理/刷新 | management mutation、OAuth/credential completion | 750ms 防抖 sync，不覆盖期望 tuning | coordinator tests、sync call assertions |
| 插件管理 | management plugin routes/header | wildcard 透传；按 CPA support header 判断 native 能力 | header/route 必验，native upload 条件验证 |
| 官方面板 | `/api/cpa/panel` | 固定 hash、CSP、no-store、Root-only | exact hash unit test + desktop/mobile iframe |

若官方文档新增 `/v0/management` 子路径，Gateway proxy 原则上无需新增 route；仍需评估该 mutation 是否会改 runtime YAML、是否需要 Provider sync，以及固定面板是否使用该能力。

## 7. 故障排查手册

| 现象 | 首先检查 | 常见根因 | 正确处理 |
| --- | --- | --- | --- |
| runtime password 得到 403 `management key not set` | runtime YAML 的原始 `remote-management.secret-key` | sentinel 为空；只调用了 `WithLocalManagementPassword` | 从 SnapshotStore 生成 bcrypt sentinel；保留回归测试 |
| 明文 sentinel 被启动接受 | `StartEmbedded` 检查发生在 CPA loader 前还是后 | CPA loader 已自动哈希并回写 | 读取原始 YAML AST 后先用 `bcrypt.Cost` 验证 |
| Stop 后新启动仍被旧请求卡住 | lease 是否绑定每一运行代 WaitGroup | 跨运行代复用 WaitGroup | 每次 starting 创建新 generation，lease 捕获所属 generation |
| CPA stopped 但仍被路由选中 | Provider runtime registry | 误把 DB Status 当运行状态或漏调 unavailable hook | 独立 runtime=false；保持 DB tuning 不变 |
| management mutation 返回成功但重启丢配置 | proxy response commit 与 PersistRuntime 顺序 | 异步保存或响应先提交 | 在 `ModifyResponse` 同步保存，失败改写为 500 |
| 下载截断或内存暴涨 | transport/handler 是否全量 buffer | whole-response timeout 或读取完整 body | 正常下载流式；仅 auth-status 小响应限 1 MiB |
| 外部 callback 可重复使用 | claim 在转发前还是后 | 转发后才标记或未加锁 | provider/state 校验后原子 claim，再转发 |
| reverse proxy behind HTTPS 错拒绝 Origin | request scheme/effective port | 忽略 TLS/X-Forwarded-Proto/default port | 按计划归一化 scheme、host、effective port |
| Jest 显示 No tests found | worktree 路径和 `testMatch` | CRA 忽略隐藏 `.worktrees` | 使用本手册显式相对 `testMatch` 命令 |
| `go test ./...` 报缺少 web embed | `web/build/index.html` | 未先 build frontend | 在 `web` 执行 `npm run build` 后重跑 |
| panel 空白或第二登录页 | asset hash、CSP、localStorage bootstrap、management responses | 错误面板版本、placeholder 未设置、proxy auth 失败 | 逐层检查固定 hash、storage 和浏览器 network |
| panel hash 不匹配 | URL、版本和实际文件 hash | upstream asset 变化或下载错误 | 立即停止，不提交；核实 release，不更新常量掩盖 |
| race 命令立即失败 | `go env CGO_ENABLED` | 当前环境为 0 | 在支持 CGO/C toolchain 环境补跑，准确记录限制 |
| 新路由 Admin 也能访问 | middleware 顺序和 role threshold | 使用了 AdminAuth 或漏 NoTokenAuth | 使用 RootAuth 后接 NoTokenAuth，增加真实 session route test |

排障时遵循：复现 -> 缩小到 component boundary -> 查实际输入/输出 -> 对照固定依赖源码 -> 写失败测试 -> 单点修复。连续三次修复仍出现不同共享状态问题时停止叠补丁，重新检查架构边界。

## 8. 环境与验证命令

### Go

```powershell
go test ./service/cpa -count=1 -timeout 4m
go test ./... -count=1 -timeout 8m
go vet ./service/cpa
git diff --check
```

当前环境 `CGO_ENABLED=0`，所以 `go test -race` 会直接报：

```text
go: -race requires cgo; enable cgo by setting CGO_ENABLED=1
```

在具备 C toolchain 的 CI 或开发机上必须补跑计划指定的 race tests。不要修改业务代码来规避这个环境错误。

### Frontend

依赖已通过 `npm ci` 安装。由于 CRA/Jest 默认 `testMatch` 对隐藏 `.worktrees` 路径发现失败，在本工作树中使用：

```powershell
Set-Location web
$env:CI='true'
npm test -- --runInBand --watchAll=false --testMatch='**/?(*.)+(spec|test).js'
npm run build
Set-Location ..
```

已知基线：6 个 suite、38 个 test 通过。`npm audit` 有 56 个既有漏洞，本功能不要顺手升级依赖。

根 Go 包通过 `go:embed` 使用 `web/build`。运行 `go test ./...` 前若构建目录缺失，先执行 `npm run build`；不能把真实编译失败误判为 embed 前置条件。

`web/build` 当前被 `web/.gitignore` 忽略且没有 tracked 文件。构建命令可以更新本地目录，但 `git status` 不会显示它；不要据此误判 build 未执行，也不要未经确认强制纳入提交。

### Playwright

Task 10 才安装 `@playwright/test` 和 Chromium。启动脚本必须在 `%TEMP%` 下使用独立数据库与 `CPA_RUNTIME_DIR`，不得污染仓库或用户数据。

## 9. 安全审计清单

每次涉及 backend bridge、controller、route 或 frontend 时检查：

```powershell
rg -n "runtime-secret|ManagementPassword|X-Management-Key|Authorization" service/cpa controller/cpa.go router/api-router.go web/src/pages/CPA
rg -n "0\.0\.0\.0|allow-remote|disable-control-panel|disable-auto-update-panel" service/cpa
rg -n "v0/management|anthropic/callback|codex/callback|antigravity/callback" router controller service/cpa
```

预期：

- secret 只出现在测试 fixture 和服务端 header 注入代码。
- 没有 API/Status/frontend 字段暴露真实密码。
- CPA 永远绑定 `127.0.0.1`。
- 只有三个精确 public OAuth callback。
- 没有 generic CPA root proxy，也没有 `/v0/resource` proxy。

## 10. 不得破坏的仓库状态

主工作区存在与本功能无关的用户文件，不能删除、移动、纳入提交或回滚：

```text
check_dgb.sql
error.txt
gateway-aggregator
req - 副本.txt
req.txt
resp.txt
```

所有开发只在本手册顶部列出的隔离工作树进行。不要在主工作区执行 reset、checkout 覆盖或清理命令。

## 11. 最终验收

完成 Task 10 后必须同时满足：

- Root 打开 `/cpa`，无需第二个密码即可进入官方 v1.18.3 dashboard。
- CPA v7.2.80 的配置、认证文件、OAuth、配额、日志、API Key、Provider 和插件管理请求完整可用。
- 完整 YAML 的未知节点和插件配置跨 restart 保留。
- Stop 立即停止新管理请求、排空已有请求，并从路由选择中移除 CPA，但不覆盖 Provider 期望状态。
- Admin、普通用户、API token 和跨域 mutation 均无法进入管理面。
- 浏览器、响应和日志中没有真实 management password、OAuth code 或认证文件内容。
- 面板文件 SHA-256 与固定值完全一致。
- focused tests、全仓 Go tests、可用环境中的 race tests、frontend unit tests、production build、真实 CPA integration 和 Playwright desktop/mobile 全部通过。

## 12. Opus 4.8 会话与交付模板

### 12.1 会话启动提示

可将下面内容作为新会话第一条任务：

```text
在 E:\NewAPI-Gateway\.worktrees\embedded-cpa-management 的
feature/embedded-cpa-management 分支继续内置 CPA 完整管理开发。

先完整阅读：
1. docs/CPA_EMBEDDED_DEVELOPMENT_HANDBOOK.md
2. docs/superpowers/specs/2026-07-16-embedded-cpa-full-management-design.md
3. docs/superpowers/plans/2026-07-16-embedded-cpa-full-management.md

Task 1-3 已完成，提交历史中必须包含 4fc515a。请从 Task 4 开始，严格按计划执行 TDD：
先写测试并观察正确 RED，再实现 GREEN，跑 focused tests、受影响包、go test ./...、
go vet 和 git diff --check，最后只提交 Task 4 文件，使用计划指定提交消息。

不要猜 CPA 接口；有疑问时查询 v7.2.80 依赖源码和官方管理 API 文档。
遵守本手册 1.1 的八条开发纪律；当前仓库没有物理 AGENTS.md，不能因此忽略用户约束。
不要暴露 CPA 端口或真实 management password，不要覆盖 Provider 的期望 Status/Priority/Weight，
不要添加 generic CPA root proxy，不要修改主工作区的无关用户文件。

当前机器 CGO_ENABLED=0，race tests 无法本地执行，需明确记录并在支持 CGO 的环境补跑。
完成 Task 4 后先汇报改动、验证证据和提交号，再继续 Task 5。
```

### 12.2 单 Task 启动模板

后续 Task 使用同一模板，只替换尖括号内容：

```text
执行内置 CPA 计划中的 Task <N>: <名称>。

工作目录：E:\NewAPI-Gateway\.worktrees\embedded-cpa-management
分支：feature/embedded-cpa-management
前置提交：<上一 Task commit>

先阅读开发手册的 Task <N> 作业卡和完整实施计划对应章节，再检查计划列出的所有现有文件。
本次只允许修改 Task <N> 文件列表，不进入 Task <N+1>，不修改主工作区无关文件。

严格执行：
1. 跑受影响包基线。
2. 只写当前 Task 测试。
3. 执行计划 focused command，给出正确 RED 的测试名和原因。
4. 写最小实现到 GREEN。
5. 跑 focused、package、affected packages、go test ./...、git diff --check 和对应安全扫描。
6. 当前环境不能跑 race 时，给出实际错误并列出补跑要求。
7. 审查 diff 仅包含当前 Task 文件后，用计划指定消息提交。

不要猜 CPA v7.2.80 接口；不确定时先查询依赖源码和官方管理 API 文档。
最终只汇报：改动、关键设计判断、RED/GREEN 证据、完整验证结果、race 限制、提交号、剩余风险。
```

### 12.3 完成报告模板

```markdown
Task <N> 已完成，提交 `<sha> <message>`。

实现：
- <用户可观察行为或核心契约 1>
- <行为 2>

安全/兼容性：
- <未暴露 secret、保留兼容字段、未覆盖 operator state 等>

TDD 证据：
- RED：`<command>`，`<test>` 因 `<missing behavior>` 失败。
- GREEN：`<same command>`，<数量> tests passed。

验证：
- `<focused command>`：通过。
- `<package command>`：通过。
- `go test ./... -count=1 -timeout 8m`：通过。
- `git diff --check`：无输出。
- race：<通过，或 CGO_ENABLED=0 未执行并需补跑>。

剩余风险：
- <没有则写“无当前 Task 已知未处理风险”；不要省略环境限制>。
```

### 12.4 阻塞报告模板

只有缺少外部权限、固定资产哈希不匹配、批准计划与实际 CPA API 不可兼容，或同一根因连续验证仍无法推进时才停止：

```markdown
Task <N> 在 `<步骤>` 阻塞。

目标行为：<计划原文摘要>
实际证据：<命令、状态码、测试名、源码位置>
已排除：<至少列出已验证的替代根因>
根因判断：<已知或明确写未知>
继续所需：<用户选择、权限、外部状态或计划调整>
未执行：<后续步骤和提交>
工作树状态：<列出未提交文件，确认没有回滚用户改动>
```

不要把“实现较复杂”“测试耗时”或“race 环境缺失但普通开发仍可继续”报告为阻塞。

## 相关文档

- [内置 CPA 完整管理设计](superpowers/specs/2026-07-16-embedded-cpa-full-management-design.md)
- [内置 CPA 完整管理实施计划](superpowers/plans/2026-07-16-embedded-cpa-full-management.md)
