# Token 自动刷新功能

## 概述

从 v7.2.103+ 开始，NewAPI-Gateway 支持自动刷新即将过期的 OAuth 访问令牌。该功能适用于使用 OAuth 2.0 授权流程（如 Refresh Token Grant）的上游提供商。

## 功能特性

- **自动检测过期令牌**：每 2 分钟检查即将在 10 分钟内过期的令牌
- **后台自动刷新**：无需手动干预，在令牌过期前自动刷新
- **无缝更新**：刷新后的令牌自动更新到数据库和路由表
- **日志记录**：所有刷新操作均记录到系统日志

## 数据库变更

### 新增字段

`provider_tokens` 表新增两个字段：

```sql
refresh_token VARCHAR(512)  -- OAuth 刷新令牌
expires_at BIGINT           -- 访问令牌过期时间（Unix 时间戳）
```

### 迁移脚本

执行以下 SQL 脚本迁移现有数据库：

```bash
# MySQL / PostgreSQL
mysql -u your_user -p your_database < migrate_refresh_token.sql

# SQLite
sqlite3 newapi.db < migrate_refresh_token.sql
```

或者重启 Gateway 时 GORM 会自动创建新列（AutoMigrate）。

## 上游 API 支持

### 上游提供商要求

上游 NewAPI / One API 实例需要支持：

1. **Token 列表接口**返回 `refresh_token` 和 `expires_at`：
   ```
   GET /api/token/?p=1&page_size=100
   ```
   响应：
   ```json
   {
     "success": true,
     "data": {
       "items": [
         {
           "id": 123,
           "key": "sk-xxx",
           "refresh_token": "rt-xxx",
           "expires_at": 1722086400,
           ...
         }
       ]
     }
   }
   ```

2. **Token 刷新接口**：
   ```
   POST /api/token/:id/refresh
   ```
   请求体：
   ```json
   {
     "refresh_token": "rt-xxx"
   }
   ```
   响应：
   ```json
   {
     "success": true,
     "data": {
       "access_token": "sk-new-xxx",
       "refresh_token": "rt-new-xxx",
       "expires_at": 1722090000
     }
   }
   ```

### CLIProxyAPI 集成

CLIProxyAPI v7.2.102+ 原生支持此功能：
- Claude Web 账户通过 OAuth device flow 获取 refresh token
- Kimi、xAI 等平台的 refresh token 自动同步
- 嵌入式 CPA 实例的令牌会自动刷新并同步到 Gateway

## 定时任务调度

### 刷新策略

- **检查间隔**：每 2 分钟
- **过期窗口**：提前 10 分钟检测
- **刷新条件**：
  - `status = 1`（启用状态）
  - `expires_at > 0`（有明确过期时间）
  - `expires_at <= now + 600`（10 分钟内过期）
  - `refresh_token != ''`（存在刷新令牌）

### 日志示例

```
[SysLog] token refresh completed: 3 refreshed, 0 failed
[SysLog] token refreshed successfully: id=45, provider=claude-web, expires_at=2026-07-27 18:30:00
```

## 手动刷新

如果需要立即刷新特定令牌，可通过管理 API：

```bash
# 手动触发同步（会拉取最新的 refresh_token）
curl -X POST http://localhost:3000/api/provider/:id/sync \
  -H "Authorization: Bearer your-admin-token"
```

## 故障处理

### 刷新失败场景

1. **Refresh Token 过期**
   - 日志：`refresh token rejected (status 401)`
   - 处理：需重新授权获取新的 refresh token

2. **上游接口不支持**
   - 日志：`upstream refresh failed: 404`
   - 处理：上游需升级到支持刷新接口的版本

3. **网络错误**
   - 日志：`upstream refresh failed: connection timeout`
   - 处理：检查网络连接和代理设置

### 监控建议

定期检查日志中的刷新失败：

```bash
grep "refresh token failed" logs/newapi.log | tail -n 20
```

## 兼容性

- **向后兼容**：旧版本上游（不返回 refresh_token）仍正常工作，只是不会自动刷新
- **Key-Only 提供商**：直接使用 API Key 的提供商（如 OpenAI 官方）不受影响
- **混合环境**：同时支持有/无 refresh token 的提供商共存

## 相关文件

- `model/provider_token.go` - 数据模型（新增字段）
- `service/token_refresh.go` - 刷新逻辑
- `service/upstream_client.go` - 上游 API 客户端（新增刷新接口）
- `service/cron.go` - 定时任务调度
- `service/sync.go` - 同步时保存 refresh_token
- `migrate_refresh_token.sql` - 数据库迁移脚本
