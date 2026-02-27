# CentOS 部署指南

本指南介绍在 CentOS 7/8/Stream 上部署 NewAPI-Gateway 的方法。

## 🚀 一键部署（推荐）

### 快速开始

直接在 CentOS 服务器上执行以下命令：

```bash
# 下载并运行部署脚本（默认启用 HTTPS）
curl -fsSL https://raw.githubusercontent.com/sx5486510/NewAPI-Gateway/main/deploy/deploy-on-centos.sh -o deploy-on-centos.sh
sudo bash deploy-on-centos.sh
```

**默认使用 HTTPS，程序启动时自动生成自签名 SSL 证书。**

### 脚本自动完成的操作

部署脚本会自动执行以下 11 个步骤：

| 步骤 | 操作 |
|------|------|
| 1/11 | 检查并安装依赖（Git、Go、Node.js、GCC、OpenSSL） |
| 2/11 | 创建目录结构 |
| 3/11 | 从 GitHub 克隆最新代码 |
| 4/11 | 构建前端静态资源 |
| 5/11 | 编译后端（启用 CGO，支持 SQLite） |
| 6/11 | 设置文件权限 |
| 7/11 | 生成安全的 SESSION_SECRET |
| 8/11 | 创建 systemd 服务文件 |
| 9/11 | 创建 .env 配置文件（添加 HTTPS_ENABLED=true） |
| 10/11 | 启动服务并自动生成自签名证书 |
| 11/11 | 配置防火墙开放端口 |

### 部署完成后的信息

```bash
# 查看服务状态
sudo systemctl status newapi-gateway

# 访问 Web 界面（HTTPS）
https://your-server-ip:3030
```

**⚠️ 浏览器提示：**
- 会显示"不安全"警告（自签名证书）
- 点击"高级" → "继续访问"即可正常使用

**默认登录信息：**
- 用户名：`root`
- 密码：`123456`
- ⚠️ **请登录后立即修改密码！**

### 使用 HTTP 模式

如需使用 HTTP（不推荐）：

```bash
# 添加 --http 参数禁用 HTTPS
sudo bash deploy-on-centos.sh --http
```

### 使用特定分支

如果需要使用特定的分支：

```bash
# HTTPS 模式（默认）
sudo bash deploy-on-centos.sh branch-name

# HTTP 模式
sudo bash deploy-on-centos.sh branch-name --http
```

### 更新代码

当有新版本时：

```bash
cd /opt/newapi-gateway/source
git pull
sudo bash deploy/deploy-on-centos.sh
```

---

## 📁 目录结构

```
/opt/newapi-gateway/
├── bin/                    # 可执行文件
│   └── gateway-aggregator
├── source/                 # 源代码
│   ├── deploy/            # 部署脚本
│   │   ├── deploy-on-centos.sh
│   │   └── build-on-centos.sh
│   ├── web/               # 前端源码
│   └── ...
├── logs/                   # 日志目录
├── data/                   # 数据库文件（SQLite）
│   └── newapi.db
└── .env                    # 环境配置文件
```

---

## ⚙️ 配置文件

### .env 文件

位置：`/opt/newapi-gateway/.env`

```bash
# 服务配置
PORT=3030
LOG_DIR=/opt/newapi-gateway/logs
GIN_MODE=release

# 安全配置
SESSION_SECRET=your-random-secret-key

# HTTPS 配置（默认启用）
HTTPS_ENABLED=true

# 数据库配置（默认使用 SQLite）
SQL_DSN=sqlite:///opt/newapi-gateway/data/newapi.db

# MySQL 示例：
# SQL_DSN=username:password@tcp(localhost:3306)/newapi?charset=utf8mb4&parseTime=True&loc=Local

# PostgreSQL 示例：
# SQL_DSN=host=localhost user=username password=password dbname=newapi port=5432 sslmode=disable TimeZone=Asia/Shanghai

# Redis 配置（可选，用于会话管理）
# REDIS_CONN_STRING=redis://localhost:6379
# REDIS_PASSWORD=

# 其他配置
# REDIS_ENABLED=false
```

### HTTPS 自签名证书

程序首次启动时会在运行目录自动生成：

- **证书文件：** `server.crt`
- **私钥文件：** `server.key`
- **有效期：** 1 年
- **无需域名：** 使用 IP 地址即可

### 修改配置后重启

```bash
sudo systemctl restart newapi-gateway
```

---

## 🛠️ 服务管理

### systemd 服务

服务文件位置：`/etc/systemd/system/newapi-gateway.service`

### 常用命令

```bash
# 查看服务状态
sudo systemctl status newapi-gateway

# 启动服务
sudo systemctl start newapi-gateway

# 停止服务
sudo systemctl stop newapi-gateway

# 重启服务
sudo systemctl restart newapi-gateway

# 设置开机自启
sudo systemctl enable newapi-gateway

# 禁用开机自启
sudo systemctl disable newapi-gateway

# 查看实时日志
sudo journalctl -u newapi-gateway -f

# 查看最近的日志
sudo journalctl -u newapi-gateway -n 50

# 查看日志文件
tail -f /opt/newapi-gateway/logs/*.log
```

---

## 🔒 防火墙配置

### 开放端口（默认 3030）

```bash
# 使用 firewalld（CentOS 7+）
sudo firewall-cmd --permanent --add-port=3030/tcp
sudo firewall-cmd --reload

# 查看已开放的端口
sudo firewall-cmd --list-ports

# 使用 iptables（旧版本）
sudo iptables -A INPUT -p tcp --dport 3030 -j ACCEPT
sudo service iptables save
```

---

## 🌐 配置 Nginx 反向代理（可选）

使用 Nginx 作为反向代理可以提供更好的性能和安全性。

### 安装 Nginx

```bash
sudo yum install -y nginx
```

### HTTPS 模式配置文件

当应用启用 HTTPS 时，Nginx 配置示例：

创建配置文件：`/etc/nginx/conf.d/newapi-gateway.conf`

```nginx
server {
    listen 80;
    server_name your-domain.com;

    # 重定向到应用（应用使用 HTTPS）
    location / {
        proxy_pass https://localhost:3030;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # HTTPS 代理配置
        proxy_ssl_verify off;

        # 增加超时时间
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
```

### HTTP 模式配置文件

当应用使用 HTTP 时：

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:3030;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 增加超时时间
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
```

### 启动 Nginx

```bash
# 测试配置
sudo nginx -t

# 启动 Nginx
sudo systemctl start nginx

# 设置开机自启
sudo systemctl enable nginx
```

---

## 🔐 HTTPS 说明

### 自签名证书

应用默认使用自动生成的自签名 SSL 证书：

- **生成时机：** 程序首次启动时
- **证书文件：** `server.crt`（在程序运行目录）
- **私钥文件：** `server.key`（在程序运行目录）
- **有效期：** 1 年
- **无需域名：** 使用 IP 地址即可访问

### 浏览器提示

由于使用自签名证书，浏览器会显示安全警告：

```
⚠️ 您的连接不是私密连接
   此站点使用了不受支持的协议。

高级 → 继续访问 your-server-ip (不安全)
```

这是正常现象，点击"继续访问"即可正常使用。

### 切换 HTTP/HTTPS

在 `.env` 文件中修改：

```bash
# 启用 HTTPS（默认）
HTTPS_ENABLED=true

# 禁用 HTTPS，使用 HTTP
HTTPS_ENABLED=false
```

修改后重启服务：

```bash
sudo systemctl restart newapi-gateway
```

---

## 🔧 故障排除

### 服务无法启动

```bash
# 查看详细错误日志
sudo journalctl -u newapi-gateway -n 50

# 检查端口占用
sudo netstat -tlnp | grep 3030
# 或
sudo ss -tlnp | grep 3030
```

### 数据库连接失败

- 检查数据库服务是否运行
- 验证 `.env` 中的 `SQL_DSN` 连接字符串是否正确
- 确认数据库用户权限

### 前端构建失败

```bash
# 清除 npm 缓存并重新构建
cd /opt/newapi-gateway/source/web
rm -rf node_modules package-lock.json
npm install
npm run build
```

### Go 依赖下载失败

```bash
# 使用 Go 代理
cd /opt/newapi-gateway/source
export GOPROXY=https://goproxy.cn,direct
go mod download
```

### npm 安装失败

```bash
# 使用国内镜像
cd /opt/newapi-gateway/source/web
npm install --registry=https://registry.npmmirror.com
```

---

## 🛡️ 安全建议

1. **修改默认密码**：登录后立即修改 root 密码
2. **配置防火墙**：只开放必要的端口（默认 3030）
3. **使用 HTTPS**：默认已启用自签名 SSL 证书保护数据传输
4. **定期更新**：保持系统和应用更新
5. **保护配置文件**：`.env` 包含敏感信息，确保权限正确（600）
6. **保护证书文件**：`server.key` 私钥文件应妥善保管
7. **日志监控**：定期检查访问日志和错误日志
8. **备份数据**：定期备份数据库和配置文件

---

## ⚡ 性能优化

### 使用 Redis

启用 Redis 可以提高会话管理和缓存性能：

```bash
# 安装 Redis
sudo yum install -y redis
sudo systemctl start redis
sudo systemctl enable redis

# 修改 .env 配置
sudo vi /opt/newapi-gateway/.env
# 添加: REDIS_CONN_STRING=redis://localhost:6379

# 重启服务
sudo systemctl restart newapi-gateway
```

### 资源限制

systemd 服务已配置资源限制：
- 内存限制：1G
- CPU 配额：100%
- 文件描述符：65536

如需修改，编辑服务文件：

```bash
sudo vi /etc/systemd/system/newapi-gateway.service
# 修改 [Service] 部分的限制参数
sudo systemctl daemon-reload
sudo systemctl restart newapi-gateway
```

---

## 📦 备份和恢复

### 备份数据

```bash
# 备份数据库
sudo cp /opt/newapi-gateway/data/newapi.db /opt/newapi-gateway/data/newapi.db.backup.$(date +%Y%m%d)

# 备份配置文件
sudo cp /opt/newapi-gateway/.env /opt/newapi-gateway/.env.backup.$(date +%Y%m%d)

# 创建完整备份
tar -czf newapi-gateway-backup-$(date +%Y%m%d).tar.gz /opt/newapi-gateway
```

### 恢复数据

```bash
# 停止服务
sudo systemctl stop newapi-gateway

# 恢复数据库
sudo cp /path/to/newapi.db.backup /opt/newapi-gateway/data/newapi.db

# 恢复配置
sudo cp /path/to/.env.backup /opt/newapi-gateway/.env

# 启动服务
sudo systemctl start newapi-gateway
```

---

## 📋 部署脚本参数

### deploy-on-centos.sh

**用法：** `sudo bash deploy-on-centos.sh [branch] [--http]`

**参数：**
- `branch`（可选）：指定要使用的 Git 分支，默认为 `main`
- `--http`（可选）：禁用 HTTPS，使用 HTTP 模式

**示例：**
```bash
# 使用默认分支（main），启用 HTTPS（默认）
sudo bash deploy-on-centos.sh

# 使用默认分支，禁用 HTTPS
sudo bash deploy-on-centos.sh --http

# 使用特定分支，启用 HTTPS
sudo bash deploy-on-centos.sh develop

# 使用特定分支，禁用 HTTPS
sudo bash deploy-on-centos.sh develop --http
```

---

## 🌍 相关信息

### 默认配置

| 配置项 | 值 | 说明 |
|--------|-----|------|
| 服务端口 | 3030 | 可通过 PORT 环境变量修改 |
| HTTPS | 启用（默认） | 自动生成自签名证书 |
| 安装目录 | /opt/newapi-gateway | 程序安装位置 |
| 日志目录 | /opt/newapi-gateway/logs | 日志文件位置 |
| 数据目录 | /opt/newapi-gateway/data | SQLite 数据库位置 |
| 数据库 | SQLite | 默认使用，支持 MySQL/PostgreSQL |
| 会话存储 | Cookie | 可选使用 Redis |

### 相关链接

- 项目仓库：https://github.com/sx5486510/NewAPI-Gateway
- 部署脚本：`deploy/deploy-on-centos.sh`
- 问题反馈：https://github.com/sx5486510/NewAPI-Gateway/issues

---

## 🔄 更新和维护

### 更新到最新版本

```bash
cd /opt/newapi-gateway/source
git pull origin main
sudo bash deploy/deploy-on-centos.sh
```

### 重新部署

如果需要完全重新部署：

```bash
# 停止服务
sudo systemctl stop newapi-gateway

# 备份数据（重要！）
sudo cp /opt/newapi-gateway/data/newapi.db /opt/newapi-gateway/data/newapi.db.backup

# 重新运行部署脚本
cd /opt/newapi-gateway/source
sudo bash deploy/deploy-on-centos.sh
```

---

## 📞 获取帮助

如果遇到问题：

1. 查看日志：`sudo journalctl -u newapi-gateway -n 50`
2. 检查配置：`cat /opt/newapi-gateway/.env`
3. 测试连接：`curl https://localhost:3030 -k`（HTTPS）
4. 或测试连接：`curl http://localhost:3030`（HTTP）
5. 提交问题：https://github.com/sx5486510/NewAPI-Gateway/issues
