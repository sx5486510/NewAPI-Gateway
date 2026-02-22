# 项目结构说明

> 返回文档入口：[README.md](./README.md)

## 顶层目录

```text
.
├── main.go                 # 程序入口
├── common/                 # 全局配置、常量、日志、Redis、工具
├── controller/             # HTTP 控制器（参数解析、调用 service/model）
├── middleware/             # 鉴权、限流、跨域、缓存
├── model/                  # GORM 模型与核心查询逻辑
├── router/                 # 路由注册（api/relay/web）
├── service/                # 业务服务（代理、同步、签到、上游客户端）
├── web/                    # React 管理台
├── docs/                   # 项目文档
├── Dockerfile              # 容器构建
└── Makefile                # 常用开发命令
```

## 后端分层职责

- `router`：定义 URL 与中间件组合。
- `middleware`：统一处理鉴权、限流与请求前置逻辑。
- `controller`：接收请求并调用 `service` / `model`。
- `service`：封装跨模型业务流程与外部调用。
- `model`：数据库读写、路由选择、统计查询。
- `common`：系统共享配置与工具函数。

## 关键文件映射

| 文件 | 作用 |
| --- | --- |
| `main.go` | 初始化 DB、Redis、定时任务与 HTTP 服务 |
| `router/relay-router.go` | OpenAI/Anthropic/Gemini 兼容 Relay 路由 |
| `router/api-router.go` | 管理 API 分组与权限控制 |
| `service/proxy.go` | 透明代理、SSE 转发、Usage 日志 |
| `service/sync.go` | 上游 pricing/token/balance 同步与路由重建 |
| `model/model_route.go` | 模型路由优先级 + 权重算法 |
| `middleware/agg_token_auth.go` | 聚合 Token 校验（ag-token） |

## 前端结构（`web/`）

- `web/src/pages`：页面级入口（仪表盘、供应商、令牌、日志等）。
- `web/src/components`：可复用 UI 组件。
- `web/build`：前端构建产物，由后端静态托管。

## 扩展点建议

1. 新增 Relay 协议：优先改 `router/relay-router.go` + `service/proxy.go`。
2. 新增管理模块：按 `router` -> `controller` -> `model/service` 顺序添加。
3. 调整路由策略：集中修改 `model/model_route.go`。
4. 调整同步逻辑：集中修改 `service/sync.go` 与 `service/upstream_client.go`。

## 相关文档

- 开发指南：[DEVELOPMENT.md](./DEVELOPMENT.md)
- 架构说明：[ARCHITECTURE.md](./ARCHITECTURE.md)
