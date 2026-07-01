# 客户端限制功能

## 功能概述

客户端限制功能允许管理员为每个供应商分组令牌配置客户端访问限制。当渠道供应商要求某些分组令牌只能被特定客户端使用时（如 Codex、ClaudeCode/CC），可以通过此功能实现自动过滤。

## 工作原理

### 1. 客户端识别

系统通过 `User-Agent` 请求头识别客户端类型：

- **Codex**: User-Agent 包含 `codex` 关键字
- **ClaudeCode/CC**: User-Agent 包含 `claudecode` 或 `claude-code` 关键字
- **不限制**: 其他所有客户端

识别逻辑位于 `middleware/agg_token_auth.go:identifyClientType()`

### 2. 令牌配置

每个 `ProviderToken` 新增两个字段：

- `allow_codex` (bool): 是否允许 Codex 客户端使用
- `allow_cc` (bool): 是否允许 ClaudeCode/CC 客户端使用

**默认行为（两者都为 false）**: 不限制，所有客户端都可以使用

**限制规则**:
- 如果只勾选 `allow_codex`，则只有 Codex 客户端可以使用此令牌
- 如果只勾选 `allow_cc`，则只有 ClaudeCode/CC 客户端可以使用此令牌
- 如果两者都勾选，则 Codex 和 ClaudeCode/CC 都可以使用
- 未识别的客户端（clientType 为空）始终可以使用未设置限制的令牌

### 3. 路由过滤

在路由选择阶段 (`model/model_route.go:BuildRouteAttemptsByPriority()`)，系统会：

1. 从上下文获取客户端类型（由中间件提取）
2. 遍历候选路由时，调用 `token.IsClientAllowed(clientType)` 检查
3. 过滤掉不符合限制的路由
4. 返回符合条件的路由列表供重试逻辑使用

过滤逻辑位于 `model/provider_token.go:IsClientAllowed()`

## 使用方法

### 配置步骤

#### 方式一：供应商令牌管理页面

1. 登录管理后台
2. 进入「供应商」→ 选择目标供应商 → 「令牌管理」标签页
3. 点击「编辑」按钮打开令牌编辑弹窗
4. 在「客户端限制」区域：
   - 默认不勾选任何选项 = 不限制（推荐）
   - 勾选「允许 Codex」= 只允许 Codex 客户端
   - 勾选「允许 ClaudeCode/CC」= 只允许 CC 客户端
   - 两者都勾选 = 同时允许两种客户端
5. 点击「保存」

#### 方式二：模型路由页面（新增）

1. 登录管理后台
2. 进入「路由」→ 选择目标模型
3. 在路由详情表格中，查看「客户端限制」列
4. 每条路由会显示对应令牌的客户端限制状态：
   - **不限制** (灰色徽章): 默认状态，所有客户端可用
   - **Codex** (蓝色徽章): 只允许 Codex
   - **CC** (紫色徽章): 只允许 ClaudeCode/CC
   - **Codex + CC** (两个徽章): 同时允许两种客户端

**注意**: 路由页面只展示限制状态，修改需要到供应商页面的令牌管理中进行。

### 查看状态

#### 供应商令牌列表

令牌列表的「客户端限制」列显示当前配置：

- **不限制** (灰色徽章): 默认状态，所有客户端可用
- **Codex** (蓝色徽章): 只允许 Codex
- **CC** (紫色徽章): 只允许 ClaudeCode/CC
- **Codex + CC** (两个徽章): 同时允许两种客户端

#### 模型路由列表

路由详情表格新增「客户端限制」列，显示该路由对应令牌的客户端限制状态。这让管理员可以直观地看到每个模型的各个路由的客户端限制情况，便于排查路由问题。

## 技术实现

### 数据库迁移

新增字段需要数据库迁移：

```sql
ALTER TABLE provider_token ADD COLUMN allow_codex BOOLEAN DEFAULT FALSE;
ALTER TABLE provider_token ADD COLUMN allow_cc BOOLEAN DEFAULT FALSE;
```

### 核心代码

#### 1. 中间件层 (middleware/agg_token_auth.go)

```go
// 5. Extract client type from User-Agent
userAgent := strings.ToLower(strings.TrimSpace(c.GetHeader("User-Agent")))
clientType := identifyClientType(userAgent)
c.Set("client_type", clientType)

func identifyClientType(userAgent string) string {
    if strings.Contains(userAgent, "codex") {
        return "codex"
    }
    if strings.Contains(userAgent, "claudecode") || strings.Contains(userAgent, "claude-code") {
        return "cc"
    }
    return ""
}
```

#### 2. 模型层 (model/provider_token.go)

```go
func (pt *ProviderToken) IsClientAllowed(clientType string) bool {
    // No restriction set - allow all
    if !pt.AllowCodex && !pt.AllowCC {
        return true
    }
    // Unrestricted client - allow all
    if clientType == "" {
        return true
    }
    // Check specific restrictions
    if clientType == "codex" && pt.AllowCodex {
        return true
    }
    if clientType == "cc" && pt.AllowCC {
        return true
    }
    return false
}
```

#### 3. 路由层 (model/model_route.go)

```go
func BuildRouteAttemptsByPriority(modelName string, clientType string) ([][]RouteAttempt, error) {
    // ... 候选路由筛选逻辑 ...
    
    for _, route := range candidateRoutes {
        token := tokenLookup[route.ProviderTokenId]
        // Filter by client type restrictions
        if !token.IsClientAllowed(clientType) {
            continue
        }
        // ... 加入路由列表 ...
    }
}
```

#### 4. 控制器层 (controller/relay.go)

```go
// Extract client type from context
clientType := ""
if ct, exists := c.Get("client_type"); exists {
    if ctStr, ok := ct.(string); ok {
        clientType = ctStr
    }
}

// Build retry plan for all selectable routes with client type filtering
plan, err := model.BuildRouteAttemptsByPriority(routingModel, clientType)
```

#### 5. 前端展示 (web/src/components/ModelRoutesTable.js)

在模型路由详情表格中新增「客户端限制」列：

```javascript
const tokenAllowCodex = route.token_allow_codex || false;
const tokenAllowCC = route.token_allow_cc || false;

// 渲染客户端限制状态
{!tokenAllowCodex && !tokenAllowCC ? (
    <Badge color="gray">不限制</Badge>
) : (
    <div style={{ display: 'flex', gap: '0.25rem', flexWrap: 'wrap' }}>
        {tokenAllowCodex && <Badge color="blue">Codex</Badge>}
        {tokenAllowCC && <Badge color="purple">CC</Badge>}
    </div>
)}
```

路由总览 API 响应中包含令牌的客户端限制字段：

```go
type ModelRouteOverviewItem struct {
    // ... 其他字段 ...
    TokenAllowCodex bool `json:"token_allow_codex"`
    TokenAllowCC    bool `json:"token_allow_cc"`
    // ... 其他字段 ...
}
```

## 注意事项

1. **默认行为**: 两个字段都为 `false` 时表示不限制，这是最宽松的配置，适合大多数场景
2. **向后兼容**: 现有令牌默认不限制，不影响已有功能
3. **未识别客户端**: 对于未识别的客户端（clientType 为空），只能访问未设置限制的令牌
4. **大小写不敏感**: User-Agent 匹配时已转为小写，不区分大小写
5. **同步问题**: 上游同步令牌时不会覆盖本地的 `allow_codex` 和 `allow_cc` 设置
6. **路由页面只读**: 模型路由页面只展示客户端限制状态，修改需要到供应商页面进行

## 故障排查

### 问题：某些客户端无法访问模型

1. 检查客户端 User-Agent 是否包含识别关键字
2. 查看令牌列表的「客户端限制」列，确认配置是否正确
3. 查看路由页面的「客户端限制」列，确认该模型的路由是否有客户端限制
4. 查看日志中的 `client_type` 值（在 LLM Trace 中可见）

### 问题：限制不生效

1. 确认数据库字段已添加
2. 确认前端已重新构建 (`npm run build`)
3. 确认后端已重新编译 (`go build`)
4. 重启服务

### 问题：路由页面不显示客户端限制

1. 确认后端 API 返回了 `token_allow_codex` 和 `token_allow_cc` 字段
2. 检查浏览器控制台是否有 JavaScript 错误
3. 清除浏览器缓存并刷新页面

## 相关文件

### 后端

- `model/provider_token.go` - 令牌模型，包含限制字段和校验逻辑
- `model/model_route.go` - 路由选择，应用客户端过滤；路由总览 API，包含令牌限制字段
- `middleware/agg_token_auth.go` - 认证中间件，提取客户端类型
- `controller/relay.go` - 代理控制器，传递客户端类型

### 前端

- `web/src/pages/Provider/ProviderDetail.js` - 供应商令牌管理界面，配置客户端限制
- `web/src/components/ModelRoutesTable.js` - 模型路由页面，展示客户端限制状态

