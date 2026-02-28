# CentOS 部署指南

本文档对应当前仓库脚本的实际行为：
- `deploy/deploy-on-centos.sh`：部署服务，默认 HTTP。
- `deploy/deploy-with-caddy.sh`：在服务已部署基础上配置 Caddy + TLS。

## 1. 部署应用服务（默认 HTTP）

```bash
curl -fsSL https://raw.githubusercontent.com/sx5486510/NewAPI-Gateway/main/deploy/deploy-on-centos.sh -o deploy-on-centos.sh
sudo bash deploy-on-centos.sh
```

默认行为：
- 服务名：`newapi-gateway`
- 应用监听：`3030`
- 不启用应用内置 HTTPS（默认 HTTP）

可选：若你确实需要应用自签名 HTTPS（不推荐），可加 `--https`。

```bash
sudo bash deploy-on-centos.sh --https
```

## 2. 配置 Caddy + 证书（推荐）

在应用已运行后执行：

```bash
sudo bash /opt/newapi-gateway/source/deploy/deploy-with-caddy.sh \
  --domain usbip.pycloud.com.cn \
  --app-port 3030 \
  --cert-file /etc/ssl/newapi/usbip.pycloud.com.cn_bundle.crt \
  --key-file /etc/ssl/newapi/usbip.pycloud.com.cn.key
```

脚本行为：
- 检查 `newapi-gateway` 服务已运行。
- 安装并启动 Caddy。
- 写入 `/etc/caddy/Caddyfile`。
- Caddy 对外监听 `3031`（HTTPS），反代到 `127.0.0.1:3030`。
- 开放防火墙 `3031/tcp`。

访问地址：

```text
https://usbip.pycloud.com.cn:3031
```

## 3. Caddy 脚本参数

```bash
sudo bash deploy-with-caddy.sh [--domain <域名>] [--app-port <端口>] [--cert-file <证书>] [--key-file <私钥>]
```

参数说明：
- `--domain`：公网域名，默认 `usbip.pycloud.com.cn`
- `--app-port`：应用本地端口，默认 `3030`
- `--cert-file` / `--key-file`：自定义证书和私钥，需同时提供

如果不传证书参数，脚本会使用 Caddy 自动签发（Let's Encrypt）。

## 4. 常用检查命令

```bash
# 应用服务
sudo systemctl status newapi-gateway
sudo journalctl -u newapi-gateway -f

# Caddy
sudo systemctl status caddy
sudo journalctl -u caddy -f

# 本地联通检查
curl -I http://127.0.0.1:3030
curl -I https://usbip.pycloud.com.cn:3031
```

## 5. 证书文件建议

- 将证书放在固定目录，如：`/etc/ssl/newapi/`
- 权限收紧：

```bash
sudo chmod 600 /etc/ssl/newapi/*
```

## 6. 更新流程

```bash
cd /opt/newapi-gateway/source
git pull
sudo bash deploy/deploy-on-centos.sh
# 如需重新应用证书/域名配置，再执行：
# sudo bash deploy/deploy-with-caddy.sh ...
```
