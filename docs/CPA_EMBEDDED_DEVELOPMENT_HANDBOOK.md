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

### Task 4 特别注意

- Provider 数据库中的 `Status`、`Priority`、`Weight` 是操作员期望值，CPA 停止时不得覆盖。
- 运行时是否参与路由由独立的 `common` registry 控制。
- 未注册的普通 Provider 默认可用，避免影响现有路由。
- `OnCPAUnavailable` 必须立即从路由选择中排除内置 Provider，并取消待执行的防抖同步。

### Task 5 特别注意

- 代理必须支持所有方法、子路径、query、JSON/YAML/multipart、下载流和 CPA 版本/插件响应头。
- 删除浏览器提供的 `Authorization`、`X-Management-Key`、Cookie、proxy auth 和所有 hop-by-hop header，再注入真实 runtime password。
- 一个请求持有一个 lease，直到流式响应结束才 Release。
- 成功 mutation 必须在响应提交给浏览器之前同步执行 `PersistRuntime`；失败返回稳定的 `persistence_failed` JSON。
- CPA 业务层的 4xx/5xx、body 和下载头原样透传。
- 审计日志不能包含 body、query secret、Cookie、OAuth code、token 或文件内容。

### Task 6 特别注意

- `POST`、`PUT`、`PATCH`、`DELETE` 必须验证同源 `Origin` 或 `Referer`；GET/HEAD/OPTIONS 是安全方法。
- OAuth 只允许以下三个公开 GET 路径：

```text
/anthropic/callback
/codex/callback
/antigravity/callback
```

- 不允许 wildcard callback，不允许 generic CPA root proxy。
- callback state 必须与 CPA 当前 pending provider 匹配、未过期、一次性消费；审计不能记录 state/code。

### Task 7 特别注意

- `/api/cpa/*`、panel 和 `/v0/management*` 必须同时使用 `RootAuth()` 与 `NoTokenAuth()`。
- mutation route 额外使用 `SameOrigin()`。
- 兼容接口 `GET/PUT /api/cpa/config` 和 `POST /api/cpa/reload` 保留；reload 是 restart alias。
- 完成 Runtime 接线后，删除 Task 3 中临时保留的 package-level lifecycle 兼容状态，避免两套 Manager。
- 初始化顺序：`model.InitOptionMap()` 之后、router 注册之前创建并注册 Runtime。
- 正常进程退出调用 `Manager.Shutdown`，不能把 `CPAEnabled` 改成 false。

### Task 8 特别注意

固定资产：

```text
Version: v1.18.3
URL: https://github.com/router-for-me/Cli-Proxy-API-Management-Center/releases/download/v1.18.3/management.html
SHA-256: 941a49a619a719a59e4c7917c6888a53eb3f41a4fa2fbb5c1cc94f2d1fc9cd4b
```

下载后先校验哈希；不匹配立即停止，不得提交或运行。运行时不得下载面板。

### Task 9 特别注意

- `/cpa` 只允许 role >= 100，Admin role 10 不能看到导航且直接访问会跳转。
- iframe 仅在 `state === 'running' && ready` 时挂载。
- 浏览器只写入无权限占位符 `gateway-managed`，绝不能获得真实密码。
- 复用现有布局和颜色变量；页面是工作区，不做营销式 landing page，不嵌套卡片。
- 桌面和移动端 lifecycle 控件不得重叠，iframe 必须有稳定的正尺寸。

### Task 10 特别注意

- 真实集成必须通过 Gateway proxy 操作 CPA，不得绕过代理验证管理功能。
- 覆盖认证文件 upload/list/download/status/fields/delete、配置持久化、端口变更 restart、quota reset 和停止后的 503。
- quota reset 使用 CPA 导出的 core auth 类型构造本地 fixture，不依赖真实外部账号。
- Playwright 同时跑 desktop 和 Pixel 7，检查非空面板、Root/Admin 权限、同源拒绝、布局无重叠和浏览器侧无真实密码。

## 5. 每个 Task 的标准工作流

1. 进入隔离工作树，确认分支和状态。

```powershell
Set-Location 'E:\NewAPI-Gateway\.worktrees\embedded-cpa-management'
git branch --show-current
git status --short
```

2. 阅读完整计划中当前 Task 的全部步骤，并检查相关现有代码。不要凭接口名称猜行为。
3. 先写最小真实行为测试。
4. 运行计划指定的 focused test，确认因缺失功能而 RED，不是语法或 fixture 错误。
5. 写最小实现到 GREEN。
6. 跑当前包、受影响包和全仓回归。
7. 执行 `git diff --check` 和 Task 对应安全扫描。
8. 只提交当前 Task 的文件，使用计划中的提交消息。
9. 再进入下一 Task。不要把多个 Task 压成一个大提交。

出现测试失败或意外行为时，先定位根因和依赖源码，再修改。不要通过放宽断言、删除测试或添加无依据兼容分支来“修绿”。

## 6. 环境与验证命令

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

### Playwright

Task 10 才安装 `@playwright/test` 和 Chromium。启动脚本必须在 `%TEMP%` 下使用独立数据库与 `CPA_RUNTIME_DIR`，不得污染仓库或用户数据。

## 7. 安全审计清单

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

## 8. 不得破坏的仓库状态

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

## 9. 最终验收

完成 Task 10 后必须同时满足：

- Root 打开 `/cpa`，无需第二个密码即可进入官方 v1.18.3 dashboard。
- CPA v7.2.80 的配置、认证文件、OAuth、配额、日志、API Key、Provider 和插件管理请求完整可用。
- 完整 YAML 的未知节点和插件配置跨 restart 保留。
- Stop 立即停止新管理请求、排空已有请求，并从路由选择中移除 CPA，但不覆盖 Provider 期望状态。
- Admin、普通用户、API token 和跨域 mutation 均无法进入管理面。
- 浏览器、响应和日志中没有真实 management password、OAuth code 或认证文件内容。
- 面板文件 SHA-256 与固定值完全一致。
- focused tests、全仓 Go tests、可用环境中的 race tests、frontend unit tests、production build、真实 CPA integration 和 Playwright desktop/mobile 全部通过。

## 10. 给 Opus 4.8 的会话启动提示

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
不要暴露 CPA 端口或真实 management password，不要覆盖 Provider 的期望 Status/Priority/Weight，
不要添加 generic CPA root proxy，不要修改主工作区的无关用户文件。

当前机器 CGO_ENABLED=0，race tests 无法本地执行，需明确记录并在支持 CGO 的环境补跑。
完成 Task 4 后先汇报改动、验证证据和提交号，再继续 Task 5。
```

## 相关文档

- [内置 CPA 完整管理设计](superpowers/specs/2026-07-16-embedded-cpa-full-management-design.md)
- [内置 CPA 完整管理实施计划](superpowers/plans/2026-07-16-embedded-cpa-full-management.md)
