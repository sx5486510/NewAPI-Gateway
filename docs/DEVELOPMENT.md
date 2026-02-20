# 开发文档

## 开发环境搭建

### 后端

```bash
# 安装依赖
go mod download

# 临时创建空前端 build 目录（开发模式）
mkdir -p web/build && touch web/build/index.html

# 运行（debug 模式）
GIN_MODE=debug go run main.go --port 3000
```

### 前端

```bash
cd web
npm install
npm start  # 启动开发服务器（默认 :3001）
```

前端 `package.json` 中已配置 `"proxy": "http://localhost:3000"`，开发模式下前端请求自动代理到后端。

---

## 添加新的 Relay 路由

1. 在 `router/relay-router.go` 中添加路由：
   ```go
   relay.POST("/v1/new-endpoint", controller.Relay)
   ```

2. `controller/relay.go` 中的 `Relay` 函数会自动处理：提取模型 → 路由 → 代理。

3. 如需特殊 Header 处理，在 `service/proxy.go` 的 `ProxyToUpstream` 中添加。

---

## 添加新的供应商协议

1. 在 `service/proxy.go` 中添加新的路径检测函数：
   ```go
   func isNewProtocol(path string) bool {
       return strings.Contains(path, "/new-protocol/")
   }
   ```

2. 在 `ProxyToUpstream` 的 Header 设置部分添加协议兼容代码。

3. 在 `middleware/agg_token_auth.go` 的 `extractAggToken` 中添加新的认证头支持。

---

## 数据同步流程

```
SyncProvider(provider)
├── syncPricing(client, provider)
│   └── GET /api/pricing → UpsertModelPricing
├── syncTokens(client, provider)
│   └── GET /api/token/ → UpsertProviderToken + 清理已删除 Token
├── syncBalance(client, provider)
│   └── GET /api/user/self → UpdateBalance
└── RebuildProviderRoutes(providerId)
    ├── 获取该供应商的 provider_tokens
    ├── 获取该供应商的 model_pricing
    ├── 构建 groupName → models 映射
    ├── 遍历 tokens：token.group ∈ pricing.enable_groups
    └── 批量写入 model_routes（先删后建，事务保护）
```

---

## 路由算法实现

核心函数：`model/model_route.go` → `SelectProviderToken(modelName, retry)`

```
1. 查询: model_routes WHERE model_name=? AND enabled=true
2. 提取 distinct priorities → 降序排列
3. retry=0 取最高 priority, retry=1 取次高...
4. 在同 priority 层内: weightSum = Σ(weight + 10)
5. randN(weightSum) 落入哪个区间选哪个 token
```

**注意**：`weight + 10` 的 `+10` 保底是为了确保 0 权重的路由也有被选中的概率（与上游 `ability.go` 一致）。

---

## 项目构建

### 本地构建

```bash
cd web && npm install && npm run build && cd ..
go build -ldflags "-s -w -X 'NewAPI-Gateway/common.Version=v1.0.0'" -o gateway-aggregator
```

### Docker 构建

```bash
docker build -t gateway-aggregator .
```

### 交叉编译

```bash
# Linux AMD64
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o gateway-aggregator-linux-amd64

# macOS ARM64
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -o gateway-aggregator-darwin-arm64
```

> 注意：CGO_ENABLED=1 是因为 SQLite 驱动需要 CGO。如使用 MySQL 可设为 0。
