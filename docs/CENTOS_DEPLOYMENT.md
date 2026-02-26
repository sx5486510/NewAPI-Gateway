# CentOS 部署指南

本指南介绍在 CentOS 7/8/Stream 上部署 NewAPI-Gateway 的两种方式：
- 方式一：一键部署（推荐，支持 SQLite）
- 方式二：Windows 交叉编译（需要 MySQL/PostgreSQL）

## 方式一：一键部署（推荐）

1. 打包源码（排除构建产物）

```bash
tar -czf newapi-gateway-source.tar.gz --exclude=node_modules --exclude=bin --exclude=web/build --exclude=web/node_modules .
```

2. 上传到服务器

```bash
scp newapi-gateway-source.tar.gz user@server:/opt/newapi-gateway/source/
```

3. 解压源码

```bash
sudo mkdir -p /opt/newapi-gateway/source
cd /opt/newapi-gateway/source
# 解压
tar -xzf newapi-gateway-source.tar.gz
```

4. 运行脚本（会拉取代码、安装依赖、构建前端、编译后端、生成 systemd 服务并启动）

```bash
sudo bash deploy/deploy-on-centos.sh
```

## 方式二：Windows 交叉编译（不支持 SQLite）

1. 在 Windows 运行脚本生成二进制

```bat
build-for-centos.bat
```

2. 上传二进制到服务器（示例路径）

```bash
scp .\bin\gateway-aggregator user@server:/opt/newapi-gateway/gateway-aggregator
```

3. 配置数据库（必须使用 MySQL/PostgreSQL）

设置环境变量 `SQL_DSN`，例如：

```bash
# MySQL
export SQL_DSN='user:password@tcp(localhost:3306)/newapi?charset=utf8mb4&parseTime=True&loc=Local'

# PostgreSQL
export SQL_DSN='host=localhost user=username password=password dbname=newapi port=5432 sslmode=disable TimeZone=Asia/Shanghai'
```

4. 使用 systemd（推荐）或直接运行

- 或直接运行：

```bash
/opt/newapi-gateway/gateway-aggregator --port 3030 --log-dir /opt/newapi-gateway/logs
```

## 默认端口与修改

- 默认端口：`3030`
- 可通过环境变量 `PORT` 或启动参数 `--port` 修改

## systemd 常用命令

```bash
sudo systemctl status newapi-gateway
sudo systemctl start newapi-gateway
sudo systemctl stop newapi-gateway
sudo systemctl restart newapi-gateway
sudo journalctl -u newapi-gateway -f
```

## 安全建议

- 登录后尽快修改默认密码
- 仅开放必要端口（默认 3030）
- 建议配置 HTTPS（例如 Let's Encrypt）
- 妥善保护 `.env` 中的敏感信息
