# API 详细参考文档

## 认证方式

### Relay API（ag-Token）

使用聚合 Token 认证，支持以下三种格式：

```http
# OpenAI 标准格式
Authorization: Bearer ag-xxxxxxxxxxxx

# Anthropic 格式
x-api-key: ag-xxxxxxxxxxxx

# Gemini 格式
x-goog-api-key: ag-xxxxxxxxxxxx
# 或 URL 参数
?key=ag-xxxxxxxxxxxx
```

### 管理 API（Session/Token）

通过 `/api/user/login` 登录获取 Session，或使用用户管理 Token：

```http
Authorization: Bearer <user-token>
```

---

## Relay API 详解

### POST /v1/chat/completions

Chat 对话补全，完全兼容 OpenAI 格式。

**请求示例：**

```json
{
  "model": "gpt-4o",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello!"}
  ],
  "stream": true,
  "temperature": 0.7
}
```

**响应**：与上游 NewAPI 响应完全一致，网关零改动透传。

**流式响应**：当 `stream: true` 时，响应为 SSE 格式：

```
data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","choices":[...]}

data: [DONE]
```

### POST /v1/embeddings

向量化接口。

```json
{
  "model": "text-embedding-3-small",
  "input": "Hello world"
}
```

### POST /v1/images/generations

文生图接口。

```json
{
  "model": "dall-e-3",
  "prompt": "A cat sitting on a mat",
  "n": 1,
  "size": "1024x1024"
}
```

### POST /v1/messages

Anthropic Claude 兼容接口。需要额外添加：

```http
anthropic-version: 2023-06-01
```

### POST /v1beta/models/*

Gemini 兼容接口，路径格式：

```
POST /v1beta/models/gemini-pro:generateContent
```

### GET /v1/models

返回所有已接入供应商中**去重后**的可用模型列表。

**响应：**

```json
{
  "object": "list",
  "data": [
    {"id": "gpt-4o", "object": "model", "owned_by": "aggregated-gateway"},
    {"id": "claude-3-5-sonnet-20240620", "object": "model", "owned_by": "aggregated-gateway"}
  ]
}
```

---

## 管理 API 详解

所有管理 API 响应格式统一为：

```json
{
  "success": true,
  "message": "",
  "data": {}
}
```

### 供应商管理

#### POST /api/provider/ — 创建供应商

```json
{
  "name": "Provider A",
  "base_url": "https://api.provider-a.com",
  "access_token": "your-access-token",
  "user_id": 1,
  "weight": 10,
  "priority": 0,
  "checkin_enabled": true,
  "remark": "主力供应商"
}
```

#### POST /api/provider/:id/sync — 同步数据

异步触发该供应商的数据同步任务。同步内容：

1. 模型定价（调用上游 `GET /api/pricing`）
2. Token 列表（调用上游 `GET /api/token/?p=1&page_size=100`）
3. 用户余额（调用上游 `GET /api/user/self`）
4. 自动重建该供应商的模型路由

#### POST /api/provider/:id/checkin — 手动签到

调用上游 `POST /api/user/checkin` 完成签到。

### 聚合 Token 管理

#### POST /api/agg-token/ — 创建 Token

```json
{
  "name": "my-token",
  "expired_time": -1,
  "model_limits_enabled": true,
  "model_limits": "gpt-4o,claude-3-5-sonnet-20240620",
  "allow_ips": "192.168.1.1\n10.0.0.1"
}
```

**响应 data**：返回完整 Token `"ag-xxxxxxxxxxxx"`

| 字段                   | 说明                             |
| ---------------------- | -------------------------------- |
| `name`                 | Token 名称                       |
| `expired_time`         | 过期时间戳，`-1` 表示永不过期    |
| `model_limits_enabled` | 是否启用模型白名单               |
| `model_limits`         | 逗号分隔的模型名列表             |
| `allow_ips`            | 换行分隔的 IP 白名单，留空不限制 |

### 模型路由

#### GET /api/route/?model=gpt-4o — 查看路由

支持按模型名称过滤。每条路由包含：

| 字段                | 说明                  |
| ------------------- | --------------------- |
| `model_name`        | 模型名                |
| `provider_token_id` | 对应的供应商 Token ID |
| `provider_id`       | 供应商 ID             |
| `enabled`           | 是否启用              |
| `priority`          | 优先级                |
| `weight`            | 权重                  |

#### POST /api/route/rebuild — 重建路由

重建所有供应商的模型路由表。逻辑：

```
对每个启用的供应商:
  获取其 provider_tokens 和 model_pricing
  对每个 token:
    token.group_name → pricing.enable_groups 包含此 group 的所有模型
    → 生成 (model, token_id, provider_id, weight, priority) 路由条目
```

### Dashboard 统计

#### GET /api/dashboard

返回聚合统计数据：

```json
{
  "total_requests": 1234,
  "success_requests": 1200,
  "failed_requests": 34,
  "total_providers": 3,
  "total_models": 42,
  "total_routes": 156,
  "by_provider": [
    {"provider_name": "Provider A", "request_count": 800}
  ],
  "by_model": [
    {"model_name": "gpt-4o", "request_count": 500}
  ]
}
```

---

## 错误码

Relay API 返回 OpenAI 兼容格式的错误：

```json
{
  "error": {
    "message": "错误描述",
    "type": "error_type",
    "code": "error_code"
  }
}
```

| HTTP Code | type                   | code                  | 说明                   |
| --------- | ---------------------- | --------------------- | ---------------------- |
| 401       | `authentication_error` | `invalid_api_key`     | Token 无效/过期/已禁用 |
| 403       | `permission_error`     | `ip_not_allowed`      | IP 不在白名单          |
| 403       | `permission_error`     | `model_not_allowed`   | 模型不在白名单         |
| 503       | `server_error`         | `service_unavailable` | 所有供应商不可用       |
| 502       | `server_error`         | -                     | 上游请求失败           |

---

## 定时任务

| 任务     | 频率       | 说明                                        |
| -------- | ---------- | ------------------------------------------- |
| 数据同步 | 每 5 分钟  | 同步所有启用供应商的 pricing/tokens/balance |
| 路由重建 | 同步后自动 | 根据最新数据重建 model_routes               |
| 签到     | 每 24 小时 | 为启用签到的供应商执行签到                  |
