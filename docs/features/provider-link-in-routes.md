# 模型路由页面 - 供应商链接功能

## 功能概述
在模型路由页面的路由详情表中，点击供应商名称可以在新标签页打开该供应商的上游 URL 地址（BaseURL）。

## 用户场景
当你在路由表中查看某个供应商的路由配置时，可能需要快速访问该供应商的上游 API 地址（如 OpenAI、Anthropic 等）来查看文档、测试接口或排查问题。现在只需点击供应商名称即可直达。

## 实现细节

### 后端改动

#### 1. 数据模型 (`model/model_route.go`)

**ModelRouteOverviewItem 结构** (第 197-238 行)
```go
type ModelRouteOverviewItem struct {
    Id                      int      `json:"id"`
    DisplayModelName        string   `json:"display_model_name"`
    ModelName               string   `json:"model_name"`
    ProviderId              int      `json:"provider_id"`
    ProviderName            string   `json:"provider_name"`
    ProviderBaseURL         string   `json:"provider_base_url"`  // 新增字段
    ProviderBalance         string   `json:"provider_balance"`
    // ... 其他字段
}
```

**modelRouteOverviewRow 结构** (第 241-252 行)
```go
type modelRouteOverviewRow struct {
    Id                int     `gorm:"column:id"`
    ModelName         string  `gorm:"column:model_name"`
    ProviderId        int     `gorm:"column:provider_id"`
    ProviderName      string  `gorm:"column:provider_name"`
    ProviderBaseURL   string  `gorm:"column:provider_base_url"`  // 新增字段
    ProviderBalance   string  `gorm:"column:provider_balance"`
    // ... 其他字段
}
```

#### 2. SQL 查询 (`model/model_route.go` 第 1051-1061 行)
```go
query := DB.Table("model_routes AS mr").
    Select(strings.Join([]string{
        "mr.id",
        "mr.model_name",
        "mr.provider_id",
        "COALESCE(p.name, '') AS provider_name",
        "COALESCE(p.base_url, '') AS provider_base_url",  // 新增查询字段
        "COALESCE(p.balance, '') AS provider_balance",
        // ... 其他字段
    }, ", "))
```

#### 3. 数据映射 (`model/model_route.go` 第 1130-1139 行)
```go
item := &ModelRouteOverviewItem{
    Id:                      row.Id,
    DisplayModelName:        "",
    ModelName:               row.ModelName,
    ProviderId:              row.ProviderId,
    ProviderName:            row.ProviderName,
    ProviderBaseURL:         row.ProviderBaseURL,  // 新增字段映射
    ProviderBalance:         row.ProviderBalance,
    // ... 其他字段
}
```

### 前端改动

#### 组件逻辑 (`web/src/components/ModelRoutesTable.js` 第 920-949 行)

```jsx
<Tr key={route.id}>
    <Td style={cellTopStyle}>
        <div style={{ fontWeight: '600', color: 'var(--text-primary)', lineHeight: 1.35 }}>
            {route.provider_deleted ? (
                // 已删除的供应商：红色警告文本，不可点击
                <span style={{ color: 'var(--color-red)' }}>
                    {route.provider_name || `供应商 #${route.provider_id}`} (已删除)
                </span>
            ) : route.provider_base_url ? (
                // 有 BaseURL：显示为链接，新标签页打开
                <a
                    href={route.provider_base_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    style={{
                        color: 'var(--primary-600)',
                        textDecoration: 'none',
                        cursor: 'pointer',
                        transition: 'color 0.2s'
                    }}
                    onMouseEnter={(e) => {
                        e.target.style.textDecoration = 'underline';
                    }}
                    onMouseLeave={(e) => {
                        e.target.style.textDecoration = 'none';
                    }}
                >
                    {route.provider_name || '未知供应商'}
                </a>
            ) : (
                // 无 BaseURL：普通文本，不可点击
                <span>{route.provider_name || '未知供应商'}</span>
            )}
        </div>
        {/* ... 其他内容 */}
    </Td>
</Tr>
```

#### 测试数据更新 (`web/src/components/ModelRoutesTable.test.js`)
```javascript
const route = {
  id: 1,
  display_model_name: 'gpt-4o',
  model_name: 'gpt-4o',
  provider_id: 10,
  provider_name: 'OpenAI',
  provider_base_url: 'https://api.openai.com/v1',  // 新增测试字段
  provider_status: 1,
  // ... 其他字段
}
```

## 功能特性

### 1. 智能显示逻辑
- **已删除的供应商**：显示红色警告文本，不可点击
- **有 BaseURL 的供应商**：显示为蓝色链接，可点击
- **无 BaseURL 的供应商**：显示为普通文本，不可点击

### 2. 用户体验
- **新标签页打开**：使用 `target="_blank"`，不影响当前路由页面状态
- **安全防护**：添加 `rel="noopener noreferrer"` 防止安全漏洞
- **视觉反馈**：
  - 链接颜色使用主题色 `var(--primary-600)`
  - 鼠标悬停时显示下划线
  - 平滑的颜色过渡效果

### 3. 兼容性
- 向后兼容：如果供应商没有配置 BaseURL，显示为普通文本
- 保持现有功能：已删除供应商的警告显示逻辑不变

## 使用示例

### 示例 1：OpenAI 供应商
- 供应商名称：OpenAI
- BaseURL：`https://api.openai.com/v1`
- 点击后：新标签页打开 OpenAI API 地址

### 示例 2：自建代理
- 供应商名称：My Proxy
- BaseURL：`https://my-proxy.example.com/v1`
- 点击后：新标签页打开自建代理地址

### 示例 3：已删除的供应商
- 供应商名称：Old Provider (已删除)
- 显示：红色警告文本
- 行为：不可点击

### 示例 4：未配置 BaseURL
- 供应商名称：Some Provider
- BaseURL：空
- 显示：普通黑色文本
- 行为：不可点击

## API 变更

### 响应数据格式
`GET /api/route/overview` 返回的每个路由项新增 `provider_base_url` 字段：

```json
{
  "success": true,
  "data": [
    {
      "id": 1,
      "model_name": "gpt-4o",
      "provider_id": 10,
      "provider_name": "OpenAI",
      "provider_base_url": "https://api.openai.com/v1",
      "provider_status": 1,
      "enabled": true,
      // ... 其他字段
    }
  ]
}
```

## 测试验证

### 后端测试
```bash
cd E:/NewAPI-Gateway
go test ./model/...
go build -o newapi.exe
```

### 前端测试
```bash
cd web
npm test -- ModelRoutesTable.test.js
```

## 兼容性说明

- **数据库变更**：无需迁移，使用已有的 `providers.base_url` 字段
- **API 变更**：仅新增字段，不影响现有客户端
- **前端兼容**：如果后端返回的路由数据缺少 `provider_base_url` 字段，前端会显示为普通文本

## 相关文件

### 后端
- `model/model_route.go` - 数据模型和查询逻辑
- `controller/route.go` - 路由 API 控制器

### 前端
- `web/src/components/ModelRoutesTable.js` - 路由表组件
- `web/src/components/ModelRoutesTable.test.js` - 组件测试

### 数据库
- `providers.base_url` - 供应商 BaseURL 字段（已存在）
