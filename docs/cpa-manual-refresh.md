# CPA 手动刷新令牌功能

## 快速开始

当 xAI 认证令牌过期时，有两种刷新方式：

### 1. 自动刷新（推荐）
- **触发条件**：令牌过期前 10 分钟
- **检查间隔**：15 分钟
- **前置条件**：`refresh_token` 有效且未过期

### 2. 手动刷新（本文档重点）
当自动刷新失败或令牌已过期时，可通过 UI 手动刷新。

## 手动刷新步骤

1. 打开管理界面 → CPA 认证文件管理
2. 找到目标认证文件
3. 检查 **Refresh Token** 状态：
   - **存在但未验证**（灰色）：正常，可以刷新
   - **疑似失效**（橙色）：quota 获取失败，可能失效
   - **已过期**（红色加粗）：确认失效，需重新登录
4. 点击"刷新令牌"按钮
5. 等待刷新完成（约 2-5 秒）

## 错误处理

### `auth_file_list_failed`
**原因**：认证文件列表加载失败  
**解决**：检查 CPA 服务状态，查看日志中的详细错误信息

### `refresh_failed: xAI refresh token expired or invalid`
**原因**：refresh_token 本身已过期（通常几个月有效期）  
**解决**：需要重新登录获取新的认证文件

```bash
# 方式 1：通过 CLIProxyAPI 重新登录
cliproxyapi auth xai

# 方式 2：删除旧文件，上传新的认证文件
```

## UI 状态说明

| 状态 | 显示 | 含义 | 操作 |
|------|------|------|------|
| `'unverified'` | 存在但未验证（灰色） | 有 refresh_token，未验证 | 可以尝试刷新 |
| `'suspected'` | 疑似失效（橙色） | quota 获取失败 | 可以尝试刷新 |
| `'expired'` | 已过期（红色加粗） | 手动刷新确认失效 | 必须重新登录 |
| `'missing'` | 不存在（灰色） | 无 refresh_token | 无法自动刷新 |

## 技术实现

### 后端接口
```
POST /api/cpa/auth-files/:filename/refresh
```

**响应示例（成功）：**
```json
{
  "success": true,
  "message": "Token refreshed successfully",
  "last_refresh": "2026-07-28T10:30:00Z"
}
```

**响应示例（失败）：**
```json
{
  "success": false,
  "code": "refresh_failed",
  "message": "xAI refresh token expired or invalid"
}
```

### 前端组件
- `CPAAuthFiles.js` - 主界面，显示认证文件列表
- `cpaAuthStatus.js` - 状态判断逻辑，根据错误类型标记状态
- `cpaAuthStatus.test.js` - 测试覆盖

### 关键代码
- `service/cpa/auth_refresh.go` - 手动刷新逻辑
- `controller/auth_refresh.go` - HTTP 接口（如果存在）
- `web/src/components/CPAAuthFiles.js:handleRefreshToken()` - 前端刷新触发

## 常见问题

**Q: 为什么点击刷新没反应？**  
A: 检查浏览器控制台是否有错误，确认后端服务正常运行。

**Q: 刷新后为什么还是显示"已过期"？**  
A: refresh_token 本身已失效，必须重新登录获取新的认证文件。

**Q: 多久需要手动刷新一次？**  
A: 正常情况下自动刷新会处理，只有在自动刷新失败时才需要手动操作。

**Q: refresh_token 有效期多久？**  
A: 通常几个月，具体取决于 xAI 的策略，建议每 2-3 个月检查一次。

## 参考文档
- [CPA Auto-Refresh 修复](cpa-autorefresh-fix.md)
- [Token 自动刷新机制](token-auto-refresh.md)
- [CPA 嵌入开发手册](CPA_EMBEDDED_DEVELOPMENT_HANDBOOK.md)
