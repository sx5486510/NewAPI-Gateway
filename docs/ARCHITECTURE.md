# 系统架构

> 返回文档入口：[README.md](./README.md)

## 架构目标

- 对客户端提供单一入口与单一令牌（`ag-`）。
- 对上游保持透明代理，避免暴露网关存在。
- 支持多供应商聚合路由与失败降级。
- 支持运营侧可观测（日志、统计、成本估算）。

## 逻辑组件

- `router/`：注册管理 API、Relay API、Web 路由。
- `middleware/`：鉴权、限流、CORS、缓存等横切能力。
- `controller/`：请求编排与参数校验。
- `service/`：代理转发、上游同步、签到、路由重建。
- `model/`：数据库实体与核心查询逻辑。
- `web/`：管理后台（React）。

## 核心请求链路（Relay）

```text
客户端
  -> /v1/*
  -> middleware.AggTokenAuth
      - 解析 ag-token（Authorization / x-api-key / x-goog-api-key / key）
      - 校验状态、过期时间、IP 白名单
  -> controller.Relay
      - 提取请求 model
      - 校验聚合 token 的模型白名单
      - SelectProviderToken(model, retry)
  -> service.ProxyToUpstream
      - 替换认证为上游 sk-token
      - 清理代理特征 Header
      - 透传请求体与关键 Header
      - 流式/非流式响应转发
      - 记录 usage_logs
```

## 管理链路（/api）

```text
后台用户
  -> /api/user/login 建立 Session
  -> /api/provider/* /api/route/* /api/log/* 等管理接口
  -> 控制器调用 model/service
  -> 落库并返回统一响应 {success,message,data}
```

说明：绝大多数管理接口使用 `NoTokenAuth`，要求 Session 访问，不接受用户 Token。

## 数据同步与路由重建

### 定时任务

- 同步任务：每 5 分钟执行一次 `syncAllProviders()`。
- 签到任务：每 24 小时执行一次 `CheckinAllProviders()`。

### 单供应商同步流程

1. 拉取上游 `GET /api/pricing`，写入 `model_pricings`。
2. 拉取上游 `GET /api/token/`（分页），写入 `provider_tokens`。
3. 拉取上游 `GET /api/user/self`，更新供应商余额。
4. 按 token 分组与模型可用组关系重建该供应商 `model_routes`。

## 路由算法

实现位置：`model/model_route.go` 的 `SelectProviderToken`。

1. 查询启用路由：`model_name = ? AND enabled = true`。
2. 对优先级去重并降序。
3. 根据 `retry` 选择目标优先级层。
4. 在该层执行加权随机：`weightSum = Σ(weight + 10)`。
5. 命中后返回 `provider_token` 与 `provider`。

`+10` 用于保证低权重或 0 权重路由仍有基础概率。

## 透明代理策略

- 认证替换：客户端 `ag-` -> 上游 `sk-`。
- Header 清理：删除 `X-Forwarded-*`、`Via`、`Forwarded`、`X-Real-IP`。
- 请求体透传：不改写业务 body。
- 支持 SSE：实时转发流式响应并记录首 token 延迟。

## 关键数据表

- `providers`：上游实例信息与同步元数据。
- `provider_tokens`：上游 sk token 与分组。
- `model_pricings`：模型定价与可用分组缓存。
- `model_routes`：模型到 token 的路由映射。
- `aggregated_tokens`：用户 ag token。
- `usage_logs`：调用日志、token/cost/延迟统计。

## 相关文档

- 配置项：[CONFIGURATION.md](./CONFIGURATION.md)
- API 细节：[API_REFERENCE.md](./API_REFERENCE.md)
- 数据表说明：[DATABASE_SCHEMA.md](./DATABASE_SCHEMA.md)
