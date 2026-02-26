# 开发指南

> 返回文档入口：[README.md](./README.md)

## 文档导航

- 上一篇：[PROJECT_STRUCTURE.md](./PROJECT_STRUCTURE.md)
- 下一篇：[DATABASE_SCHEMA.md](./DATABASE_SCHEMA.md)
- 接口参考：[API_REFERENCE.md](./API_REFERENCE.md)

## 开发环境

- Go >= 1.18
- Node.js >= 16
- 推荐使用 `make` 管理本地任务

## 快速启动开发模式

### 安装依赖

```bash
make deps
```

### 启动前后端

```bash
make dev
```

默认端口：

- 后端：`http://localhost:3030`
- 前端：`http://localhost:3001`

前端 `package.json` 已配置代理到后端。

## 常用命令

```bash
make dev-be   # 后端调试
make dev-fe   # 前端调试
make build    # 构建前后端
make test     # 后端测试
make clean    # 清理构建产物
```

## 后端调试建议

- 设置 `GIN_MODE=debug` 查看更详细日志。
- 临时开启 `DEBUG_PROXY_AUTH=1` 可输出代理认证替换摘要（排障后关闭）。
- 若未配置 Redis，限流与 Session 会回退为内存/Cookie 模式。

## 核心开发入口

- Relay 主流程：`controller/relay.go` -> `service/proxy.go`
- 路由算法：`model/model_route.go`
- 同步与重建：`service/sync.go`
- 聚合 Token 鉴权：`middleware/agg_token_auth.go`
- 管理路由：`router/api-router.go`

## 常见开发任务

### 1. 新增 Relay 端点

1. 在 `router/relay-router.go` 注册新路径。
2. 复用 `controller.Relay` 时确认请求体中 `model` 字段可被解析。
3. 如需特殊协议 Header，在 `service/proxy.go` 增加路径判断与头处理。
4. 更新 `docs/API_REFERENCE.md`。

### 2. 调整路由策略

1. 优先修改 `model/model_route.go` 的 `BuildRouteAttemptsByPriority` / `computeRouteContribution` / `finalizeRouteHealthStat`。
2. 补充对应单元测试（建议覆盖优先级分层、价值评分、健康倍率边界）。
3. 更新 `docs/ARCHITECTURE.md` 的算法说明。

### 3. 扩展供应商同步字段

1. 更新 `service/upstream_client.go` 的上游结构映射。
2. 更新 `service/sync.go` 写库逻辑。
3. 注意：上游可能返回 `HTTP 200` 但 `success=false` 的错误体，`UpstreamClient` 会直接返回错误。
4. 若上游被 Cloudflare 挑战页拦截，`UpstreamClient` 会在错误中提示可能的拦截原因。
5. 上游错误响应体在日志中会截断（默认 200 bytes），需要完整响应请查看上游日志或抓包。
6. 如新增字段入库，更新 `model/*` 结构并确认迁移兼容性。
7. 更新 `docs/DATABASE_SCHEMA.md`。

## 提交前检查清单

1. `go test ./...` 通过。
2. 前端构建 `npm --prefix web run build` 通过。
3. 关键流程手测：登录 -> 供应商同步 -> 创建聚合 token -> Relay 请求。
4. 文档同步更新：
   - 接口改动 -> `API_REFERENCE.md`
   - 配置改动 -> `CONFIGURATION.md`
   - 架构流程改动 -> `ARCHITECTURE.md`

## 相关文档

- 项目结构：[PROJECT_STRUCTURE.md](./PROJECT_STRUCTURE.md)
- API 参考：[API_REFERENCE.md](./API_REFERENCE.md)
- 架构说明：[ARCHITECTURE.md](./ARCHITECTURE.md)
- 数据模型：[DATABASE_SCHEMA.md](./DATABASE_SCHEMA.md)
