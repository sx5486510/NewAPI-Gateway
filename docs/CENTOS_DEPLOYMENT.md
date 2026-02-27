# CentOS 部署指南

本指南介绍在 CentOS 7/8/Stream 上部署 NewAPI-Gateway 的方法。

## 🚀 一键部署（推荐）

### 快速开始

直接在 CentOS 服务器上执行以下命令：

```bash
# 下载并运行部署脚本
curl -fsSL https://raw.githubusercontent.com/sx5486510/NewAPI-Gateway/main/deploy/deploy-on-centos.sh -o deploy-on-centos.sh
sudo bash deploy-on-centos.sh
```

### 脚本自动完成的操作

部署脚本会自动执行以下 10 个步骤：

| 步骤 | 操作 |
|------|------|
| 1/10 | 检查并安装依赖（Git、Go、Node.js、GCC、OpenSSL） |
| 2/10 | 创建目录结构 |
| 3/10 | 从 GitHub 克隆最新代码 |
| 4/10 | 构建前端静态资源 |
| 5/10 | 编译后端（启用 CGO，支持 SQLite） |
| 6/10 | 设置文件权限 |
| 7/10 | 生成安全的 SESSION_SECRET |
| 8/10 | 创建 systemd 服务文件 |
| 9/10 | 创建 .env 配置文件 |
| 10/10 | 启动服务并设置开机自启 |

### 部署完成后的信息

```bash
# 查看服务状态
sudo systemctl status newapi-gateway

# 访问 Web 界面
http://your-server-ip:3030
```

**默认登录信息：**
- 用户名：`root`
- 密码：`123456`
- ⚠️ **请登录后立即修改密码！**

### 使用特定分支

如果需要使用特定的分支：

```bash
sudo bash deploy-on-centos.sh branch-name
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

### 配置文件

创建配置文件：`/etc/nginx/conf.d/newapi-gateway.conf`

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

## 🔐 配置 HTTPS（推荐）

使用 Let's Encrypt 免费证书：

```bash
# 安装 Certbot
sudo yum install -y epel-release
sudo yum install -y certbot python2-certbot-nginx

# 获取证书
sudo certbot --nginx -d your-domain.com

# 自动续期
sudo systemctl enable --now certbot-renew.timer
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
3. **使用 HTTPS**：配置 SSL 证书保护数据传输
4. **定期更新**：保持系统和应用更新
5. **保护配置文件**：`.env` 包含敏感信息，确保权限正确（600）
6. **日志监控**：定期检查访问日志和错误日志
7. **备份数据**：定期备份数据库和配置文件

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

**用法：** `sudo bash deploy-on-centos.sh [branch]`

**参数：**
- `branch`（可选）：指定要使用的 Git 分支，默认为 `main`

**示例：**
```bash
# 使用默认分支（main）
sudo bash deploy-on-centos.sh

# 使用特定分支
sudo bash deploy-on-centos.sh develop
```

---

## 🌍 相关信息

### 默认配置

| 配置项 | 值 | 说明 |
|--------|-----|------|
| 服务端口 | 3030 | 可通过 PORT 环境变量修改 |
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
3. 测试连接：`curl http://localhost:3030`
4. 提交问题：https://github.com/sx5486510/NewAPI-Gateway/issues
