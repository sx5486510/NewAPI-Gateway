# CPA Auto-Refresh 修复文档

## 问题描述

CPA (CLIProxyAPI) 的自动刷新功能虽然已实现，但日志消息（如 "core auth auto-refresh started"）从未出现在 Gateway 的日志中，导致无法确认该功能是否正常运行。

## 根本原因分析

### 1. 日志系统不匹配
- **Gateway** 使用自定义日志系统（基于 `gin.DefaultWriter`）
  - 输出到 `stdout` + `logs/common.log`
  - 日志格式：`[SYS] 2026/07/27 - 15:04:05 | message`

- **CPA** 使用 `github.com/sirupsen/logrus`
  - 默认输出到 `os.Stderr`
  - 日志格式：logrus 的标准格式

### 2. 日志消息丢失流程
```
CPA启动 
  → service.Run() 执行
  → StartAutoRefresh() 被调用
  → log.Infof("core auth auto-refresh started...") 
  → 输出到 os.Stderr（未被 Gateway 捕获）
  → Gateway 的 logs/common.log 中看不到
```

### 3. 关键代码位置
- CPA 日志调用：`.tmp-cliproxyapi-*/sdk/cliproxy/service.go:1778`
  ```go
  log.Infof("core auth auto-refresh started (interval=%s)", interval)
  ```
- CPA 启动：`service/cpa/embed.go:98` (`service.Run(ctx)`)
- Gateway 日志系统：`common/logger.go`

## 解决方案

### 修改 1: 配置 logrus 输出到 Gateway 日志

**文件**: `service/cpa/embed.go`

#### 添加 logrus 导入
```go
import (
	// ... 其他导入
	log "github.com/sirupsen/logrus"
)
```

#### 在 CPA 启动前配置 logrus
```go
// Configure logrus to output to Gateway's gin.DefaultWriter
// so CPA's log messages (including "core auth auto-refresh started")
// appear in Gateway's logs instead of being lost to stderr.
log.SetOutput(common.GetGinWriter())
log.SetFormatter(&log.TextFormatter{
	FullTimestamp:   true,
	TimestampFormat: "2006/01/02 - 15:04:05",
})

service, err := cliproxy.NewBuilder().
	WithConfig(cfg).
	// ...
```

### 修改 2: 暴露 gin.DefaultWriter

**文件**: `common/logger.go`

```go
func GetGinWriter() io.Writer {
	return gin.DefaultWriter
}
```

## 验证方法

### 1. 编译并启动 Gateway
```bash
go build -o newapi-gateway ./main.go
./newapi-gateway
```

### 2. 检查日志
```bash
tail -f logs/common.log | grep -E "auto-refresh|watcher|embedded CPA"
```

### 3. 预期输出
启动后应该看到：
```
[SYS] 2026/07/27 - 16:30:00 | embedded CPA starting on 127.0.0.1:18317
time="2026/07/27 - 16:30:01" level=info msg="file watcher started for config and auth directory changes"
time="2026/07/27 - 16:30:01" level=info msg="core auth auto-refresh started (interval=15m0s)"
[SYS] 2026/07/27 - 16:30:01 | embedded CPA ready
```

## 技术细节

### Auto-Refresh 启动条件
在 `sdk/cliproxy/service.go:1775-1778`:
```go
if s.coreManager != nil && !homeEnabled {
    interval := 15 * time.Minute
    s.coreManager.StartAutoRefresh(context.Background(), interval)
    log.Infof("core auth auto-refresh started (interval=%s)", interval)
}
```

**条件**:
- `s.coreManager != nil` ✅ (总是满足，在 builder.go:231-267 中初始化)
- `!homeEnabled` ✅ (config.yaml 中未设置 `home.enabled: true`)

### Auto-Refresh 工作机制
- **检查间隔**: 15 分钟
- **刷新条件**: 
  - Auth 文件的 `expires_at` 存在且 `> 0`
  - 当前时间 ≥ `expires_at - 10分钟`（提前 10 分钟刷新）
  - `refresh_token` 不为空
- **刷新接口**: `POST /api/token/:id/refresh`

## 修改前后对比

### 修改前
- ❌ CPA 日志输出到 `os.Stderr`
- ❌ Gateway 无法捕获 CPA 日志
- ❌ 无法确认 auto-refresh 是否启动
- ❌ 调试困难

### 修改后
- ✅ CPA 日志输出到 `gin.DefaultWriter`
- ✅ 所有日志统一输出到 `logs/common.log`
- ✅ 可以看到 "core auth auto-refresh started" 消息
- ✅ 方便调试和监控

## 相关文件

- `service/cpa/embed.go` - CPA 嵌入式启动，配置 logrus
- `common/logger.go` - Gateway 日志工具，暴露 gin.DefaultWriter
- `sdk/cliproxy/service.go` - CPA 服务，auto-refresh 启动逻辑
- `sdk/cliproxy/auth/manager.go` - CPA auth manager，auto-refresh 实现

## 后续改进建议

1. **日志级别控制**: 考虑添加环境变量来控制 CPA 日志级别（info/debug/error）
2. **日志前缀**: 为 CPA 日志添加 `[CPA]` 前缀，方便区分 Gateway 和 CPA 的日志
3. **性能监控**: 添加 auto-refresh 成功/失败的统计指标

## 测试清单

- [x] 编译成功
- [ ] 启动后看到 "embedded CPA starting" 消息
- [ ] 启动后看到 "file watcher started" 消息
- [ ] 启动后看到 "core auth auto-refresh started" 消息
- [ ] 15 分钟后观察是否有刷新日志
- [ ] 验证 auth 文件的 `last_refresh` 时间戳是否更新

## 参考资料

- [CPA 嵌入集成文档](cpa-embed-integration.md)
- [上游账户余额令牌约定](upstream-account-balance-token.md)
- [logrus 文档](https://github.com/sirupsen/logrus)
