# CPA 额度刷新对接设计

## 背景

Gateway 的 CPA 认证文件管理页已经加入额度刷新入口，但当前实现与内嵌 CPA 管理前端的实际行为不一致：Claude 使用了错误的接口和认证头，Codex 与 Grok 缺少账号相关请求信息和回退路径，并且成功响应没有进入前端额度状态，而是错误地依赖重新拉取认证文件列表。

本设计让 Gateway 完整跟随当前内嵌 CPA 前端的额度查询能力，同时保留 Gateway 现有认证文件管理页面。

## 目标

- 支持 Antigravity、Claude、Codex、Kimi、Grok 五类凭证。
- 对齐 CPA 当前前端的请求地址、请求头、凭证字段提取、回退顺序和响应解析。
- 单文件刷新后立即在对应文件下展示额度，不依赖认证文件列表携带额度。
- 支持单文件刷新和“获取全部”。
- 每个文件的加载、成功和失败状态彼此隔离。
- 为前端额度逻辑和 Gateway 管理代理补充自动化测试。

## 非目标

- 不加入 Codex 主动消耗一次重置机会的操作。
- 不改变 CPA 的认证文件上传、编辑、启停、下载和删除语义。
- 不把 CPA 原生管理页面嵌入或跳转到 Gateway 页面中。
- 不在本次工作中重构无关的 CPA 管理功能。

## 总体架构

### 前端额度模块

新增一个独立的 CPA quota 模块，职责包括：

1. 规范化 `provider`/`type`，识别五类受支持凭证。
2. 从认证文件顶层、`metadata`、`attributes`、令牌载荷或认证文件内容中提取请求所需字段。
3. 按 provider 执行 CPA 原生的一个或多个 `POST /v0/management/api-call` 请求。
4. 识别 CPA 内层的 `status_code`、`header` 和 `body`，将字符串 JSON 安全解析为对象。
5. 把不同 provider 的响应转换为稳定的展示模型。

`CPAAuthFiles.js` 只负责列表交互、按文件保存额度状态和渲染统一展示模型，不再维护 provider 请求分支。

### 状态模型

额度状态以认证文件名为 key；若文件名缺失，则退化为 `auth_index`。每个条目独立维护：

```text
idle -> loading -> success
                -> error
```

成功状态保存 provider、套餐信息、额度行及可选附加信息。错误状态保存可显示的消息和可选 HTTP 状态。刷新某个文件只更新该文件；刷新全部使用独立任务聚合，单个失败不终止其他任务。

禁用凭证不发起额度请求。正在刷新的文件不能重复提交，但不阻塞其他文件。

## CPA API Call 契约

Gateway 前端统一调用：

```json
{
  "authIndex": "<CPA auth_index>",
  "method": "GET or POST",
  "url": "https://provider.example/path",
  "header": {
    "Authorization": "Bearer $TOKEN$"
  },
  "data": "<optional JSON string>"
}
```

CPA 返回：

```json
{
  "status_code": 200,
  "header": {},
  "body": {}
}
```

`body` 既可能是对象，也可能是 JSON 字符串。只有内层 `status_code` 为 2xx 才视为 provider 请求成功。

## Provider 行为

### Antigravity

- 从 `project_id`/`projectId`、`metadata`、`attributes.gemini_virtual_project` 中提取 project ID。
- 若列表数据不含 project ID，通过现有认证文件下载接口读取 JSON，并继续检查顶层、`installed` 和 `web` 中的 project ID。
- 依次尝试 CPA 当前前端定义的三个 `retrieveUserQuotaSummary` 地址：daily、daily sandbox、cloudcode。
- 使用 `POST`，请求体为 `{"project":"<id>"}`。
- 使用 `$TOKEN$` Bearer 认证及 CPA Antigravity CLI 请求头。
- 并行查询 `loadCodeAssist` 套餐信息；套餐查询失败不使额度查询失败。
- 解析 quota groups、bucket、剩余比例、窗口及重置时间。

### Claude

- 并行请求：
  - `https://api.anthropic.com/api/oauth/usage`
  - `https://api.anthropic.com/api/oauth/profile`
- 使用 `Authorization: Bearer $TOKEN$`、`Content-Type: application/json` 和 `anthropic-beta: oauth-2025-04-20`。
- usage 是主请求；usage 失败时整体失败，profile 失败时仍展示额度但套餐未知。
- 展示 5 小时、7 天及 OAuth Apps、Opus、Sonnet、Cowork 等 CPA 已识别窗口、额外用量和套餐。

### Codex

- 请求 `https://chatgpt.com/backend-api/wham/usage`。
- 使用 `$TOKEN$` Bearer 认证、CPA 当前 Codex CLI User-Agent，并从 ID token、`metadata` 或 `attributes` 中提取 `chatgpt_account_id`，存在时设置 `Chatgpt-Account-Id`。
- 同时查询 `https://chatgpt.com/backend-api/wham/rate-limit-reset-credits`，附带 `Accept: application/json`、`OpenAI-Beta: codex-1` 和 `Originator: Codex Desktop`。
- usage 失败时整体失败；重置次数查询失败时保留额度并单独显示附加信息错误。
- 展示主/次窗口、代码审查窗口、额外额度组、套餐、订阅到期时间和可用重置次数。
- 不调用 reset credits consume 接口。

### Kimi

- 请求 `https://api.kimi.com/coding/v1/usages`。
- 使用 `Authorization: Bearer $TOKEN$`。
- 展示各额度周期、已用/总量、剩余百分比及重置提示。

### Grok

- 使用 CPA 当前 Grok CLI 请求头及 `$TOKEN$` Bearer 认证。
- 从顶层、`metadata`、`attributes`、OAuth 或 user 数据中提取用户 ID，存在时设置 `x-userid`。
- 并行查询：
  - `https://cli-chat-proxy.grok.com/v1/billing?format=credits`
  - `https://cli-chat-proxy.grok.com/v1/billing`
- 合并两个成功响应；一个失败时使用另一个，两个都失败时报告真实错误。
- 展示周额度、月度积分、产品用量、套餐和按量付费信息。

## UI 设计

- 保留现有按 provider 分类的认证文件列表。
- 每个支持的、未禁用的文件显示“刷新额度”按钮。
- 页面级操作区加入“获取全部”，只请求支持且未禁用的文件。
- 每个文件下方根据状态显示：空闲提示、加载提示、额度内容或错误消息。
- 额度统一使用“名称、剩余百分比/金额、进度、重置时间”的行式结构；provider 特有套餐、订阅和按量付费信息放在同一卡片的元信息区域。
- 不支持的 provider 不显示额度刷新入口，也不报“未知 provider”噪声。

## 错误处理

- 缺少 `auth_index` 时在对应文件显示明确错误，不发送请求。
- Antigravity 缺少 project ID 时显示明确错误。
- CPA 外层请求失败、CPA 内层非 2xx、无效 JSON 和缺少预期额度结构分别转换为可读错误。
- 仅执行 CPA 原生定义的 provider 回退；不对认证、权限或格式错误做盲目重试。
- “获取全部”使用逐文件结果隔离，一个文件失败不会覆盖其他成功结果。
- 刷新失败保留文件管理列表，不触发认证文件重新加载。

## 后端代理

保留 `POST /v0/management/api-call` 专用转发路径。该请求是由 CPA 代凭证访问外部 provider，不是 CPA 配置变更，因此不能触发 Gateway 的运行时快照持久化或同步调度。

代理必须：

- 获取 CPA management lease。
- 将路径固定转发至 CPA 的 `/v0/management/api-call`。
- 原样传递受限大小内的 JSON 请求体。
- 清除浏览器侧管理凭证并注入临时 CPA management password。
- 传递上游状态码、响应头和响应体。
- 将 transport 错误映射为 Gateway JSON 错误。
- 记录管理审计日志。

## 测试设计

### quota 模块单元测试

- provider 规范化和禁用状态识别。
- 五类 provider 的 endpoint、方法、请求头、`authIndex` 和请求体。
- Codex account ID、Antigravity project ID、Grok user ID 的多位置提取。
- Claude 并发主/附属请求语义。
- Antigravity endpoint 回退、Grok 双响应回退、Codex 附加请求降级。
- 五类响应 fixture 到统一展示模型的解析。
- 内层非 2xx、字符串 JSON、无效 JSON 和空响应错误。

### React 组件测试

- 单文件刷新进入 loading，成功后显示额度。
- 响应成功后不重新请求 auth-files。
- 同一文件 loading 时忽略重复点击。
- “获取全部”跳过禁用和不支持的文件。
- “获取全部”中的局部失败不影响其他文件成功展示。

### Go 代理测试

- 只拦截目标 POST 路径，其他请求继续使用通用反向代理。
- 请求体与 CPA management Authorization 正确转发。
- 上游状态、头和响应体正确透传。
- body 读取和 transport 错误返回预期 JSON 错误。
- 成功调用不触发快照持久化和同步调度。

## 验收标准

1. 五类 CPA 凭证均能按当前内嵌 CPA 前端语义查询并显示额度。
2. 单文件成功响应立即更新对应 UI，不重新拉取认证文件列表。
3. “获取全部”支持部分成功，失败账号有独立错误。
4. 请求 payload 与内嵌 CPA 前端保持一致，尤其是 Claude OAuth、Codex account ID、Antigravity project ID 和 Grok 双 billing 路径。
5. `api-call` 不触发配置持久化或运行时同步。
6. 新增前端单元/组件测试、CPA Go 测试通过，前端生产构建成功。
