# 部署指南

> 返回文档入口：[README.md](./README.md)

## 部署前检查

- 已准备至少一个可访问的 NewAPI 上游。
- 已确定数据存储方案（SQLite / MySQL / PostgreSQL）。
- 已规划日志目录与持久化目录。

## 方式一：二进制部署

### 1. 构建

```bash
npm --prefix web install
npm --prefix web run build
go mod download
go build -ldflags "-s -w -X 'NewAPI-Gateway/common.Version=$(cat VERSION)'" -o gateway-aggregator
```

### 2. 启动

```bash
PORT=3000 \
GIN_MODE=release \
SESSION_SECRET='replace-with-strong-secret' \
./gateway-aggregator --port 3000 --log-dir ./logs
```

## 方式二：Docker 部署

### 单容器（SQLite）

```bash
docker build -t gateway-aggregator .
docker run -d --name gateway-aggregator \
  --restart always \
  -p 3000:3000 \
  -e GIN_MODE=release \
  -e SESSION_SECRET='replace-with-strong-secret' \
  -v /data/gateway-aggregator:/data \
  gateway-aggregator
```

### Docker Compose（SQLite + Redis）

```yaml
version: '3.9'
services:
  gateway:
    image: gateway-aggregator:latest
    container_name: gateway-aggregator
    restart: always
    ports:
      - "3000:3000"
    environment:
      PORT: "3000"
      GIN_MODE: "release"
      SESSION_SECRET: "replace-with-strong-secret"
      SQLITE_PATH: "/data/gateway-aggregator.db"
      REDIS_CONN_STRING: "redis://redis:6379/0"
    volumes:
      - /data/gateway-aggregator:/data
    depends_on:
      - redis

  redis:
    image: redis:7-alpine
    container_name: gateway-redis
    restart: always
```

## 方式三：systemd（Linux）

创建 `/etc/systemd/system/gateway-aggregator.service`：

```ini
[Unit]
Description=NewAPI Gateway
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/gateway
ExecStart=/opt/gateway/gateway-aggregator --port 3000 --log-dir /opt/gateway/logs
Environment=GIN_MODE=release
Environment=SESSION_SECRET=replace-with-strong-secret
Environment=SQLITE_PATH=/opt/gateway/data/gateway-aggregator.db
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

启用服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now gateway-aggregator
sudo systemctl status gateway-aggregator
```

## 反向代理（Nginx）

```nginx
server {
    listen 80;
    server_name gateway.example.com;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE 需要关闭缓冲
        proxy_buffering off;
        proxy_read_timeout 3600s;
    }
}
```

## 升级步骤（建议）

1. 备份数据库与日志。
2. 拉取新代码并重新构建镜像/二进制。
3. 先在测试环境验证 `Sync`、`Relay` 与日志写入。
4. 生产滚动重启。
5. 登录后台检查 `Dashboard`、`Providers`、`Routes` 是否正常。

## 相关文档

- 配置说明：[CONFIGURATION.md](./CONFIGURATION.md)
- 运维手册：[OPERATIONS.md](./OPERATIONS.md)
