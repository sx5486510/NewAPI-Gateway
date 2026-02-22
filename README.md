<p align="right">
   <strong>中文</strong> | <a href="./README.en.md">English</a>
</p>

<div align="center">

# API Gateway Aggregator

_✨ 多供应商 NewAPI 聚合网关 — 统一接入、透明代理、使用统计 ✨_

</div>

## 项目简介

API Gateway Aggregator 是一个聚合多个 [NewAPI](https://github.com/QuantumNous/new-api) 供应商的透明网关。用户使用单一聚合 Token（`ag-xxx`）即可调用所有已接入供应商的 AI 模型服务，系统自动进行**权重轮询和优先级路由**，上游供应商无法感知网关的存在。

### 核心特性

- ✅ **透明代理**：Header 清洗、body 零改动、UA 透传，上游仅看到"真实客户端"
- ✅ **多供应商管理**：统一管理多个 NewAPI 实例的 Token、定价、余额
- ✅ **智能路由**：Priority 分层 + Weight 加权随机选择，与上游 `ability.go` 同算法
- ✅ **自动同步**：每 5 分钟从上游同步 pricing/tokens/balance，自动重建路由表
- ✅ **签到服务**：自动为启用签到的供应商执行每日签到
- ✅ **SSE 流式支持**：完整支持 Server-Sent Events 流式代理
- ✅ **使用统计**：详细记录每次调用的模型/供应商/耗时/状态
- ✅ **OpenAI 兼容**：支持 OpenAI / Anthropic / Gemini 等多种 API 格式
- ✅ **Web 管理面板**：React 前端，含仪表盘、供应商管理、Token管理、日志查看

> **本网关不涉及充值/计费**，仅做透明代理和使用情况统计。

---

## 系统架构

```
用户客户端
  │ Authorization: Bearer ag-xxx
  ▼
┌─────────────────────────┐
│    API Gateway Aggregator │
│  ┌─────┐  ┌──────┐      │
│  │ Auth │→│Router│      │
│  └─────┘  └──┬───┘      │
│              ▼           │
│         ┌────────┐       │
│         │ Proxy  │       │
│         └────┬───┘       │
│              │ Bearer sk-xxx
└──────────────┼───────────┘
        ┌──────┼──────┐
        ▼      ▼      ▼
    NewAPI-A NewAPI-B NewAPI-C
```

---

## 快速开始

### 环境要求

- Go 1.18+
- Node.js 16+
- SQLite（默认）/ MySQL / PostgreSQL

### 手动部署

```bash
# 1. 克隆项目
git clone <repo-url>
cd API-Gateway-Aggregator-main

# 2. 构建前端
cd web
npm install
npm run build
cd ..

# 3. 构建后端
go mod download
go build -ldflags "-s -w -X 'NewAPI-Gateway/common.Version=$(cat VERSION)'" -o gateway-aggregator

# 4. 运行
./gateway-aggregator --port 3000 --log-dir ./logs
```

### Docker 部署

```bash
docker build -t gateway-aggregator .
docker run -d --restart always \
  -p 3000:3000 \
  -v /data/gateway-aggregator:/data \
  gateway-aggregator
```

### 首次登录

访问 `http://localhost:3000/` — 初始账号：`root` / `123456`

---

## 使用指南

### 1. 添加供应商

登录后进入 **供应商** 页面，点击"添加供应商"：

| 字段         | 说明                                | 示例                         |
| ------------ | ----------------------------------- | ---------------------------- |
| 名称         | 供应商标识名                        | `Provider-A`                 |
| Base URL     | 上游 NewAPI 地址                    | `https://api.provider-a.com` |
| Access Token | 上游 access_token                   | `eyJhbGci...`                |
| 上游 User ID | 上游用户 ID（用于 New-Api-User 头） | `1`                          |
| 权重         | 路由权重（越高越优先）              | `10`                         |
| 优先级       | 路由层级（越高越优先）              | `0`                          |
| 启用签到     | 是否自动签到                        | ☑️                            |

### 2. 同步数据

点击供应商列表中的 **同步** 按钮，系统会：

1. 从上游获取模型定价（`GET /api/pricing`）
2. 获取 sk-Token 列表（`GET /api/token/`）
3. 获取用户余额（`GET /api/user/self`）
4. 自动重建模型路由表

### 3. 创建聚合 Token

进入 **令牌** 页面，创建聚合 Token：

- 支持设置过期时间
- 支持模型白名单
- 支持 IP 白名单

创建成功后获得 `ag-xxx` 格式的 Token。

### 4. 调用 API

使用聚合 Token 调用 OpenAI 兼容接口：

```bash
curl https://your-gateway.com/v1/chat/completions \
  -H "Authorization: Bearer ag-xxxxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

系统自动从所有供应商中选择可用的 `gpt-4o` 路由进行调用。

---

## API 文档

### Relay API（OpenAI 兼容，使用 ag-Token 认证）

| Method | Path                              | 说明                  |
| ------ | --------------------------------- | --------------------- |
| POST   | `/v1/chat/completions`            | 对话补全              |
| POST   | `/v1/completions`                 | 文本补全              |
| POST   | `/v1/embeddings`                  | 向量化                |
| POST   | `/v1/images/generations`          | 文生图                |
| POST   | `/v1/audio/speech`                | 文本转语音            |
| POST   | `/v1/audio/transcriptions`        | 语音转文本            |
| POST   | `/v1/messages`                    | Anthropic Claude 兼容 |
| POST   | `/v1/responses`                   | OpenAI Responses API  |
| POST   | `/v1/rerank`                      | 重排序                |
| POST   | `/v1/moderations`                 | 内容审核              |
| POST   | `/v1/video/generations`           | 视频生成              |
| POST   | `/v1beta/models/*`                | Gemini 兼容           |
| GET    | `/v1/models`                      | 查看所有可用模型      |
| GET    | `/v1/models/:model`               | 查看特定模型信息      |
| GET    | `/dashboard/billing/subscription` | 余额查询（兼容）      |
| GET    | `/dashboard/billing/usage`        | 用量查询（兼容）      |

### 管理 API（Session/Token 认证）

#### 供应商管理（Admin）

| Method | Path                        | 说明             |
| ------ | --------------------------- | ---------------- |
| GET    | `/api/provider/`            | 列出所有供应商   |
| POST   | `/api/provider/`            | 创建供应商       |
| PUT    | `/api/provider/`            | 更新供应商       |
| DELETE | `/api/provider/:id`         | 删除供应商       |
| POST   | `/api/provider/:id/sync`    | 触发同步         |
| POST   | `/api/provider/:id/checkin` | 手动签到         |
| GET    | `/api/provider/:id/tokens`  | 查看供应商 Token |

#### 聚合 Token 管理（User）

| Method | Path                 | 说明            |
| ------ | -------------------- | --------------- |
| GET    | `/api/agg-token/`    | 我的 Token 列表 |
| POST   | `/api/agg-token/`    | 创建 Token      |
| PUT    | `/api/agg-token/`    | 更新 Token      |
| DELETE | `/api/agg-token/:id` | 删除 Token      |

#### 模型路由管理（Admin）

| Method | Path                 | 说明              |
| ------ | -------------------- | ----------------- |
| GET    | `/api/route/`        | 查看路由表        |
| GET    | `/api/route/models`  | 所有可用模型      |
| PUT    | `/api/route/:id`     | 更新路由权重/状态 |
| POST   | `/api/route/rebuild` | 重建路由表        |

#### 日志与统计

| Method | Path             | 说明                 |
| ------ | ---------------- | -------------------- |
| GET    | `/api/log/self`  | 我的调用日志（User） |
| GET    | `/api/log/`      | 全部日志（Admin）    |
| GET    | `/api/dashboard` | 仪表盘统计（Admin）  |

#### 用户管理 & 系统设置

沿用原始 NewAPI-Gateway 框架的用户管理和系统设置接口，详见 `/api/user/*` 和 `/api/option/*`。

---

## 配置

### 环境变量

| 变量                | 说明                             | 示例                                   |
| ------------------- | -------------------------------- | -------------------------------------- |
| `PORT`              | 监听端口                         | `3000`                                 |
| `SQL_DRIVER`        | SQL 驱动（可选）                 | `sqlite` / `mysql` / `postgres`        |
| `SQL_DSN`           | 数据库连接串（MySQL/PostgreSQL 必填） | `root:pwd@tcp(localhost:3306)/gateway` |
| `REDIS_CONN_STRING` | Redis 连接（用于限流和 Session） | `redis://default:pw@localhost:6379`    |
| `SESSION_SECRET`    | 固定 Session 密钥                | `random_string`                        |
| `GIN_MODE`          | 运行模式                         | `release` / `debug`                    |

未设置 `SQL_DRIVER` 时，程序会保持兼容旧行为：
- 未设置 `SQL_DSN`：使用 SQLite（`SQLITE_PATH`，默认 `gateway-aggregator.db`）
- 已设置 `SQL_DSN`：自动识别 PostgreSQL（`postgres://`、`postgresql://`、或 `dbname=... user=...`），否则按 MySQL 处理

### 命令行参数

| 参数        | 说明     | 默认值 |
| ----------- | -------- | ------ |
| `--port`    | 服务端口 | `3000` |
| `--log-dir` | 日志目录 | 不保存 |
| `--version` | 打印版本 | -      |

---

## 路由算法

模型路由查询与上游 `ability.go` 的 `GetChannel` 完全一致：

1. **查询路由表**：`model_routes WHERE model_name = ? AND enabled = true`
2. **Priority 分层**：按优先级降序排列，默认取最高优先级层
3. **Weight 加权随机**：`weight_sum = Σ(weight + 10)`，随机数落入哪个区间选哪个
4. **Retry 降级**：失败后 retry 取下一优先级层

---

## 隐匿策略

确保上游供应商**感知不到网关的存在**：

| 策略               | 实现方式                                 |
| ------------------ | ---------------------------------------- |
| Authorization 替换 | `ag-xxx` → 上游 `sk-xxx`                 |
| 代理头清除         | 删除 `X-Forwarded-*`、`Via`、`Forwarded` |
| User-Agent 透传    | 保留客户端原始 UA                        |
| Body 零改动        | 请求体原样转发                           |
| 无自定义头         | 不添加任何网关标识 Header                |

---

## 数据模型

| 表名                | 说明                      |
| ------------------- | ------------------------- |
| `users`             | 网关用户                  |
| `options`           | 系统配置（KV）            |
| `providers`         | 上游供应商                |
| `provider_tokens`   | 供应商 sk-Token           |
| `aggregated_tokens` | 用户聚合 Token (ag-xxx)   |
| `model_routes`      | 模型 → 供应商Token 路由表 |
| `model_pricings`    | 上游模型定价缓存          |
| `usage_logs`        | 调用日志                  |

---

## 项目结构

```
API-Gateway-Aggregator-main/
├── main.go                        # 入口
├── common/                        # 公共常量、工具
├── model/
│   ├── user.go                    # 用户
│   ├── option.go                  # 配置 KV
│   ├── provider.go                # 供应商
│   ├── provider_token.go          # 供应商 Token
│   ├── aggregated_token.go        # 聚合 Token
│   ├── model_route.go             # 路由引擎
│   ├── model_pricing.go           # 定价缓存
│   └── usage_log.go               # 使用日志
├── service/
│   ├── upstream_client.go         # 上游 API 客户端
│   ├── sync.go                    # 数据同步 + 路由重建
│   ├── checkin.go                 # 签到
│   ├── proxy.go                   # 透传代理（SSE）
│   └── cron.go                    # 定时任务
├── middleware/
│   ├── auth.go                    # 管理后台认证
│   ├── agg_token_auth.go          # Relay Token 认证
│   ├── cors.go                    # CORS
│   └── rate-limit.go              # 限流
├── controller/
│   ├── relay.go                   # 核心代理
│   ├── provider.go                # 供应商管理
│   ├── aggregated_token.go        # Token 管理
│   ├── route.go                   # 路由管理
│   ├── log.go                     # 日志/统计
│   └── user.go                    # 用户管理
├── router/
│   ├── relay-router.go            # /v1/* 透传路由
│   ├── api-router.go              # /api/* 管理路由
│   └── web-router.go              # React SPA 路由
└── web/                           # React 前端
    └── src/
        ├── components/            # UI 组件
        └── pages/                 # 页面
```

---

## License

MIT License

本项目包含第三方 MIT 许可代码，详见 `THIRD_PARTY_NOTICES.md`。
