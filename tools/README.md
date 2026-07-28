# 工具目录

这个目录包含了独立的命令行工具。

---

## 📂 工具列表

### 1. refresh_token_via_api.go

**用途**: 通过 Gateway API 刷新认证令牌

**使用方法**:
```bash
# 刷新单个令牌
go run tools/refresh_token_via_api.go <admin-token> <auth-filename>

# 示例
go run tools/refresh_token_via_api.go sk-xxxxx xai-08av4ljy2n6l@me.23432453.xyz.json
```

**批量刷新**:
```bash
#!/bin/bash
ADMIN_TOKEN="sk-xxxxx"

for file in auths/xai-*.json; do
  filename=$(basename "$file")
  echo "Refreshing $filename..."
  go run tools/refresh_token_via_api.go $ADMIN_TOKEN $filename
  sleep 1
done
```

**环境变量**:
- `GATEWAY_BASE_URL` - Gateway 基础 URL（默认：http://localhost:3000）

**特性**:
- ✅ 通过 API 调用，不依赖内部包
- ✅ 适合批量操作和脚本自动化
- ✅ 支持自定义 Gateway URL
- ✅ 详细的错误提示

---

### 2. diagnose_cpa_autorefresh.go

**用途**: 诊断 CPA 自动刷新为何未启动

**使用方法**:
```bash
go run tools/diagnose_cpa_autorefresh.go <cpa-config-path>

# 示例
go run tools/diagnose_cpa_autorefresh.go cpa/config.yaml
```

**诊断内容**:
- ✅ 检查 Home.Enabled 配置
- ✅ 检查自动刷新启动条件
- ✅ 检查认证目录和文件
- ✅ 检查过期令牌数量
- ✅ 提供详细的诊断报告和建议

**输出示例**:
```
=== CPA Auto-Refresh Diagnostic ===

Config file: cpa/config.yaml

✅ Config loaded successfully

--- Home Configuration ---
  Home.Enabled: false
  Home.NodeID: ""
  Home.Host: ""
  Home.Port: 0

--- Auto-Refresh Start Condition ---
  homeEnabled = false
  !homeEnabled = true
  Will auto-refresh start? true

✅ Auto-refresh SHOULD start (homeEnabled = false)

--- Auth Directory ---
  AuthDir: "auths/"
  ✅ Directory exists
  Auth files: 5
  Potentially expired: 2

--- Expected Behavior ---
When CPA Service.Run() is called:
  1. ✅ homeEnabled = false
  2. ✅ Should call coreManager.StartAutoRefresh(ctx, 15*time.Minute)
  3. ✅ Should log: "core auth auto-refresh started (interval=15m)"

=== Diagnostic Complete ===
```

---

## 📚 相关文档

- **使用指南**: [TOKEN_REFRESH_METHODS.md](../TOKEN_REFRESH_METHODS.md)
- **快速参考**: [QUICK_REFERENCE.md](../QUICK_REFERENCE.md)
- **完整文档**: [docs/manual-token-refresh-ui.md](../docs/manual-token-refresh-ui.md)
- **编译修复**: [COMPILATION_FIX.md](../COMPILATION_FIX.md)

---

## ⚠️ 注意

这个目录中的工具都是独立的 `main` 包，**不能**和主程序一起编译。

正确的使用方式：
```bash
# ✅ 正确：使用 go run 运行工具
go run tools/refresh_token_via_api.go <args>
go run tools/diagnose_cpa_autorefresh.go <args>

# ❌ 错误：不要和主程序一起编译
go build ./...  # 会导致 main redeclared 错误
go build .      # 会导致 main redeclared 错误
```

编译主程序时使用：
```bash
# 只编译 main.go
go build -o newapi-gateway.exe ./main.go

# 或者指定当前目录（Go 会自动排除子目录）
go build -o newapi-gateway.exe .
```

## 📝 工具总结

| 工具 | 用途 | 使用场景 |
|------|------|----------|
| **refresh_token_via_api.go** | 刷新令牌 | 批量刷新、脚本自动化 |
| **diagnose_cpa_autorefresh.go** | 诊断自动刷新 | 调试、故障排查 |

---

**提示**: 这些工具都是为了辅助开发和运维，日常使用推荐直接使用前端界面（http://localhost:3000/admin → CPA → 刷新令牌）。

