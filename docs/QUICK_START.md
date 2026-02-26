# 快速开始

> 返回文档入口：[README.md](./README.md)

## 文档导航

- 上一篇：[文档中心](./README.md)
- 下一篇：[ARCHITECTURE.md](./ARCHITECTURE.md)
- 架构规范：[DOCS_ARCHITECTURE.md](./DOCS_ARCHITECTURE.md)

## 环境要求

- Go >= 1.18
- Node.js >= 16
- 数据库：SQLite（默认）/ MySQL / PostgreSQL
- 可选：Redis（用于 Session 与限流）

## 方式一：本地快速启动（推荐开发/试用）

### 1. 安装依赖

```bash
go mod download
npm --prefix web install
```

### 2. 构建前端与后端

```bash
npm --prefix web run build
go build -ldflags "-s -w -X 'NewAPI-Gateway/common.Version=$(cat VERSION)'" -o ./bin/gateway-aggregator
```

### 3. 启动服务

```bash
./bin/gateway-aggregator --port 3030 --log-dir ./logs
```

### 4. 首次登录

- 地址：`http://localhost:3030/`
- 默认账号：`root`
- 默认密码：`123456`

## 方式二：使用 Makefile（推荐日常开发）

```bash
make deps
make dev      # 同时启动后端(:3030)和前端(:3001)
```

可选命令：

```bash
make dev-be   # 仅后端
make dev-fe   # 仅前端
make test     # 运行后端测试
make build    # 构建前后端
```

## 方式三：Docker 部署

```bash
docker build -t gateway-aggregator .
docker run -d --name gateway-aggregator \
  --restart always \
  -p 3030:3030 \
  -v /data/gateway-aggregator:/data \
  gateway-aggregator
```

## 最小可用流程

1. 登录后台后在「供应商」页面添加上游。
2. 点击「同步」，系统会抓取上游 `pricing`、`token`、`balance` 并重建路由；若为 `key_only` 模式，仅拉取模型列表并构建路由。
3. 在「令牌」页面创建聚合令牌，得到 `ag-xxxx`。
4. 使用该令牌调用网关接口。

示例：

```bash
curl http://localhost:3030/v1/chat/completions \
  -H "Authorization: Bearer ag-your-token" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": false
  }'
```

## 相关文档

- 架构细节：[ARCHITECTURE.md](./ARCHITECTURE.md)
- 配置与环境变量：[CONFIGURATION.md](./CONFIGURATION.md)
- API 详情：[API_REFERENCE.md](./API_REFERENCE.md)
