# 构建说明

## 本地构建（Makefile）

```bash
# 构建后端二进制（自动注入 VERSION + git hash）
make build-be

# 完整构建（前端 + 后端）
make build
```

构建后的版本号格式：`v1.0.0+6fd4977`（VERSION 文件内容 + git 短 hash）

编译时间自动注入为 UTC 时间戳。

## Docker 构建

### 推荐方式（传入 git 版本）

```bash
# 在宿主机执行，传入 git 版本和构建时间
docker build \
  --build-arg GIT_VERSION="$(cat VERSION)+$(git rev-parse --short HEAD)" \
  --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t newapi-gateway:latest .
```

### 简化方式（使用 VERSION 文件）

```bash
# 不传 --build-arg，自动使用 VERSION 文件内容
docker build -t newapi-gateway:latest .
```

此时版本号为 `v1.0.0`（无 git hash），编译时间为容器内构建时间。

## 版本信息查看

### 命令行

```bash
./bin/gateway-aggregator --version
```

### Web UI

访问 **设置 → 其他设置**，顶部显示：

```
版本号：v1.0.0+6fd4977
编译时间：2026-07-06T12:34:56Z
```

### API 接口

```bash
curl http://localhost:3030/api/status | jq '.data | {version, build_time}'
```

返回示例：

```json
{
  "version": "v1.0.0+6fd4977",
  "build_time": "2026-07-06T12:34:56Z"
}
```

## 版本号规则

- **本地构建（make）**：`VERSION 文件内容 + git 短 hash`（如 `v1.0.0+6fd4977`）
- **Docker 构建（带参数）**：与本地构建一致
- **Docker 构建（无参数）**：仅 VERSION 文件内容（如 `v1.0.0`）
- **非 git 环境**：`VERSION 文件内容 + nogit`（如 `v1.0.0+nogit`）

## 注意事项

1. **VERSION 文件**：位于项目根目录，存储语义化版本号（如 `v1.0.0`）
2. **git hash**：取自 `git rev-parse --short HEAD`，短 hash 7 位
3. **编译时间**：UTC 格式 ISO8601（`2026-07-06T12:34:56Z`）
4. **构建参数优先级**：Docker `--build-arg` > VERSION 文件 > 默认值
