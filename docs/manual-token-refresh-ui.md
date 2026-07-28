# 前端手动刷新令牌功能

## 功能概述

用户现在可以在前端界面直接点击"刷新令牌"按钮来手动刷新已过期或即将过期的认证令牌，无需通过命令行操作。

---

## 使用方法

### 1. 访问 CPA 认证文件管理页面

登录 Gateway 管理后台 → 导航到 **CPA** 页面

### 2. 找到需要刷新的令牌

在认证文件列表中，查看每个认证文件的状态：
- **Expired**: 显示过期时间
- **Refresh Token**: 显示刷新令牌状态
  - ✅ `有效` - 可以刷新
  - ⚠️ `即将过期` - 建议刷新
  - ❌ `已过期` - 必须刷新
  - ⚠️ `缺失` - 无法自动刷新

### 3. 点击"刷新令牌"按钮

每个认证文件的操作按钮区域有以下按钮：
- **重置冷却** - 重置 CPA 路由冷却状态
- **测试** - 向服务商发送测试消息
- **获取真实额度** - 获取服务商真实余额
- **刷新令牌** ⭐ - 手动刷新访问令牌
- **启用/禁用** - 切换认证状态
- **编辑** - 编辑备注和优先级
- **下载** - 下载认证文件
- **删除** - 删除认证文件

点击 **刷新令牌** 按钮后：
1. 按钮进入加载状态（禁用）
2. Gateway 调用后端 API `/api/auth/refresh`
3. 后端通过 CPA Management API 触发令牌刷新
4. 刷新成功后：
   - 显示成功提示（包含新的过期时间）
   - 自动刷新列表
   - 自动重新加载认证详情
5. 刷新失败时显示错误信息

---

## 成功示例

```
✅ 令牌刷新成功: xai-08av4ljy2n6l@me.23432453.xyz.json
新过期时间: 2026-07-27T20:30:00Z
```

---

## 失败示例

```
❌ 令牌刷新失败: xai-08av4ljy2n6l@me.23432453.xyz.json
Refresh token is missing or invalid
```

---

## 适用场景

### ✅ 适合使用前端刷新

1. **单个令牌已过期** - 需要立即恢复
2. **令牌即将过期** - 提前手动刷新
3. **刷新失败后重试** - Auto-Refresh 失败后手动重试
4. **测试刷新功能** - 验证 Refresh Token 是否有效

### ❌ 不适合使用前端刷新

1. **批量刷新** - 建议使用命令行工具
2. **定时任务** - 应该使用 Auto-Refresh 机制
3. **自动化脚本** - 使用 API 调用

---

## API 端点

### 请求

```http
POST /api/auth/refresh
Content-Type: application/json
Authorization: Bearer <admin-token>

{
  "filename": "xai-08av4ljy2n6l@me.23432453.xyz.json"
}
```

### 响应（成功）

```json
{
  "success": true,
  "message": "Token refreshed successfully",
  "data": {
    "filename": "xai-08av4ljy2n6l@me.23432453.xyz.json",
    "old_expired": "2026-07-20T14:30:00Z",
    "new_expired": "2026-07-27T20:30:00Z",
    "refreshed_at": "2026-07-27T14:35:22Z"
  }
}
```

### 响应（失败）

```json
{
  "success": false,
  "code": "refresh_failed",
  "message": "Token refresh failed: Refresh token expired"
}
```

---

## 技术实现

### 前端（React）

**文件**: `web/src/components/CPAAuthFiles.js`

**核心功能**:
1. `refreshTokenStates` - 管理每个文件的刷新状态
2. `refreshTokenInFlightRef` - 防止重复请求
3. `handleRefreshToken()` - 刷新令牌的核心逻辑
   - 调用 `/api/auth/refresh` API
   - 更新状态（loading → success/error）
   - 刷新列表和凭证详情
   - 显示成功/失败提示

**按钮位置**: 
- ID: `${fileId}-refresh-token-btn`
- 位置: 在"获取真实额度"按钮之后
- 条件: 
  - `getQuotaProvider(file)` 存在
  - `!isAuthFileDisabled(file)` 未禁用

### 后端（Go）

**文件**: `controller/auth_refresh.go`

**核心功能**:
1. 验证 CPA Runtime 是否初始化
2. 获取当前认证文件的元数据（包括旧的过期时间）
3. 调用 CPA Management API `/v0/management/auth-files/refresh`
4. 返回刷新结果（新旧过期时间对比）

**路由注册**: `router/api-router.go:158`
```go
authRoute.POST("/refresh", controller.RefreshAuthToken)
```

### CPA（嵌入式）

**文件**: `.tmp-cliproxyapi-20260723075833/internal/api/handlers/management/auth_files.go`

**核心功能**:
1. `RefreshAuthFile()` - HTTP Handler
   - 接收 `{ "filename": "..." }` 请求
   - 调用 Auth Manager 的 `RefreshAuthForRequest()`
   - 返回新的过期时间

**文件**: `.tmp-cliproxyapi-20260723075833/sdk/cliproxy/auth/conductor.go`

**核心功能**:
1. `RefreshAuthForRequest()` - 公开方法（新增）
   - 调用内部的 `refreshAuthForRequest()`
   - 触发实际的令牌刷新逻辑

**路由注册**: `.tmp-cliproxyapi-20260723075833/internal/api/modules/management.go`
```go
r.POST("/auth-files/refresh", h.RefreshAuthFile)
```

---

## 与 Auto-Refresh 的关系

### Auto-Refresh（自动）
- **间隔**: 每 15 分钟检查一次
- **条件**: 过期前 10 分钟
- **适用**: 正常运行的令牌
- **优点**: 无需人工干预

### Manual Refresh（手动）
- **触发**: 用户点击按钮
- **条件**: 任意时间
- **适用**: 已过期或需要立即刷新的令牌
- **优点**: 灵活、即时

**配合使用**:
1. 令牌已过期 → 手动刷新一次
2. 刷新后获得新的 6 小时有效期
3. 5 小时 50 分钟后 → Auto-Refresh 自动接管
4. 之后无需再手动操作 ✨

---

## 故障排查

### 问题 1: 按钮不可点击

**可能原因**:
- 认证文件已禁用 → 启用后再试
- 没有 Provider（getQuotaProvider 返回空）→ 检查文件格式
- 正在刷新中 → 等待完成

### 问题 2: 刷新失败 "Refresh token is missing"

**解决方法**:
1. 检查认证文件是否包含 `refresh_token` 字段
2. 如果缺失，需要重新上传完整的认证文件
3. 或者联系服务商获取新的 Refresh Token

### 问题 3: 刷新失败 "Refresh token expired"

**解决方法**:
1. Refresh Token 本身也有过期时间（通常较长）
2. 需要重新获取新的认证文件（包括新的 Refresh Token）
3. 从服务商控制台重新下载认证

### 问题 4: 刷新失败 "CPA runtime not initialized"

**解决方法**:
1. 检查 Gateway 是否正确启动
2. 检查 CPA 是否已嵌入（查看启动日志）
3. 重启 Gateway

---

## 安全性

1. **鉴权要求**: 需要管理员权限（Bearer Token）
2. **审计日志**: 所有刷新操作记录用户名
3. **幂等性**: 防止重复提交（使用 `refreshTokenInFlightRef`）
4. **错误隔离**: 单个刷新失败不影响其他令牌

---

## 相关文档

- [Token Auto-Refresh](token-auto-refresh.md) - 自动刷新机制
- [CPA Auto-Refresh Fix](cpa-autorefresh-fix.md) - CPA 自动刷新修复
- [How to Refresh Expired Token](../HOW_TO_REFRESH_EXPIRED_TOKEN.md) - 命令行刷新指南

---

## 总结

前端手动刷新令牌功能提供了一个便捷的方式来管理认证令牌生命周期：

✅ **无需命令行** - 点击按钮即可  
✅ **实时反馈** - 成功/失败立即显示  
✅ **自动更新** - 刷新后自动重载详情  
✅ **错误友好** - 清晰的错误提示  
✅ **防止误操作** - 加载中禁用按钮  

配合 Auto-Refresh 机制，用户只需在令牌首次过期时手动刷新一次，之后系统会自动接管，无需再关心过期问题！🎉
