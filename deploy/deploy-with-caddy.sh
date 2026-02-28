#!/bin/bash
#
# Configure Caddy TLS reverse proxy for an already deployed NewAPI-Gateway service.
#
# Usage:
#   sudo bash deploy-with-caddy.sh
#   sudo bash deploy-with-caddy.sh --domain usbip.pycloud.com.cn --app-port 3030 --cert-file /etc/ssl/newapi/fullchain.pem --key-file /etc/ssl/newapi/privkey.pem
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SERVICE_NAME="newapi-gateway"
DOMAIN="usbip.pycloud.com.cn"
APP_PORT=3030
CADDY_PORT=3031
CERT_FILE=""
KEY_FILE=""
USE_CUSTOM_CERT=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain)
      DOMAIN="$2"
      shift 2
      ;;
    --app-port)
      APP_PORT="$2"
      shift 2
      ;;
    --cert-file)
      CERT_FILE="$2"
      USE_CUSTOM_CERT=true
      shift 2
      ;;
    --key-file)
      KEY_FILE="$2"
      USE_CUSTOM_CERT=true
      shift 2
      ;;
    *)
      echo -e "${RED}Unknown argument: $1${NC}"
      exit 1
      ;;
  esac
done

if [ "$EUID" -ne 0 ]; then
  echo -e "${RED}Error: please run as root.${NC}"
  exit 1
fi

if [ "$USE_CUSTOM_CERT" = true ]; then
  if [ -z "$CERT_FILE" ] || [ -z "$KEY_FILE" ]; then
    echo -e "${RED}Error: --cert-file and --key-file must be provided together.${NC}"
    exit 1
  fi
  if [ ! -f "$CERT_FILE" ] || [ ! -f "$KEY_FILE" ]; then
    echo -e "${RED}Error: certificate file or key file not found.${NC}"
    exit 1
  fi
fi

install_pkg() {
  local pkg="$1"
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y "$pkg"
  else
    yum install -y "$pkg"
  fi
}

install_caddy() {
  if command -v caddy >/dev/null 2>&1; then
    echo -e "${GREEN}Caddy installed: $(caddy version)${NC}"
    return
  fi

  echo -e "${YELLOW}Installing Caddy...${NC}"
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y 'dnf-command(copr)'
    dnf copr enable -y @caddy/caddy
    dnf install -y caddy
  else
    yum install -y yum-plugin-copr || true
    if yum copr enable -y @caddy/caddy && yum install -y caddy; then
      return
    fi

    echo -e "${YELLOW}Copr install failed, fallback to binary install...${NC}"
    yum install -y curl tar

    ARCH=$(uname -m)
    case "${ARCH}" in
      x86_64) CADDY_ARCH="amd64" ;;
      aarch64) CADDY_ARCH="arm64" ;;
      armv7l) CADDY_ARCH="armv7" ;;
      *)
        echo -e "${RED}Unsupported architecture: ${ARCH}${NC}"
        exit 1
        ;;
    esac

    CADDY_VERSION=$(curl -fsSL https://api.github.com/repos/caddyserver/caddy/releases/latest | sed -n 's/.*"tag_name": *"v\\([^"]*\\)".*/\\1/p' | head -n1)
    if [ -z "${CADDY_VERSION}" ]; then
      echo -e "${RED}Failed to detect latest Caddy version.${NC}"
      exit 1
    fi

    TMP_DIR=$(mktemp -d)
    CADDY_TGZ="caddy_${CADDY_VERSION}_linux_${CADDY_ARCH}.tar.gz"
    CADDY_URL="https://github.com/caddyserver/caddy/releases/download/v${CADDY_VERSION}/${CADDY_TGZ}"

    curl -fsSL "${CADDY_URL}" -o "${TMP_DIR}/${CADDY_TGZ}"
    tar -xzf "${TMP_DIR}/${CADDY_TGZ}" -C "${TMP_DIR}"
    install -m 755 "${TMP_DIR}/caddy" /usr/local/bin/caddy
    rm -rf "${TMP_DIR}"

    mkdir -p /etc/caddy /var/lib/caddy /var/log/caddy

    cat > /etc/systemd/system/caddy.service <<EOF
[Unit]
Description=Caddy
Documentation=https://caddyserver.com/docs/
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/caddy run --environ --config /etc/caddy/Caddyfile
ExecReload=/usr/local/bin/caddy reload --config /etc/caddy/Caddyfile --force
TimeoutStopSec=5s
LimitNOFILE=1048576
LimitNPROC=512
PrivateTmp=true
ProtectSystem=full
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
  fi
}

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Configure Caddy For NewAPI-Gateway${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "${BLUE}Domain: ${DOMAIN}${NC}"
echo -e "${BLUE}Caddy port: ${CADDY_PORT}${NC}"
echo -e "${BLUE}App port: ${APP_PORT}${NC}"
if [ "$USE_CUSTOM_CERT" = true ]; then
  echo -e "${BLUE}TLS mode: custom certificate${NC}"
else
  echo -e "${BLUE}TLS mode: automatic Let's Encrypt${NC}"
fi
echo ""

echo -e "${YELLOW}[1/5] Checking app service...${NC}"
if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
  echo -e "${RED}Error: ${SERVICE_NAME} is not running.${NC}"
  echo "Deploy app first: sudo bash deploy-on-centos.sh"
  exit 1
fi
echo -e "${GREEN}${SERVICE_NAME} is running.${NC}"

echo -e "${YELLOW}[2/5] Installing Caddy...${NC}"
install_pkg curl
install_caddy

echo -e "${YELLOW}[3/5] Writing Caddyfile...${NC}"
if [ "$USE_CUSTOM_CERT" = true ]; then
  TLS_LINE="    tls ${CERT_FILE} ${KEY_FILE}"
else
  TLS_LINE=""
fi

cat > /etc/caddy/Caddyfile <<EOF
https://${DOMAIN}:${CADDY_PORT} {
${TLS_LINE}
    reverse_proxy 127.0.0.1:${APP_PORT}
}
EOF

echo -e "${YELLOW}[4/5] Starting Caddy...${NC}"
caddy validate --config /etc/caddy/Caddyfile
systemctl enable caddy
systemctl restart caddy

echo -e "${YELLOW}[5/5] Configuring firewall...${NC}"
if command -v firewall-cmd >/dev/null 2>&1; then
  firewall-cmd --permanent --add-port=${CADDY_PORT}/tcp || true
  firewall-cmd --reload || true
else
  echo -e "${YELLOW}firewalld not found, skip firewall configuration.${NC}"
fi

echo ""
if systemctl is-active --quiet caddy; then
  echo -e "${GREEN}Caddy TLS proxy configured successfully.${NC}"
  echo ""
  echo "Access: https://${DOMAIN}:${CADDY_PORT}"
  echo "Proxy target: 127.0.0.1:${APP_PORT}"
  echo ""
  echo "Check status:"
  echo "  sudo systemctl status caddy"
  echo "  sudo systemctl status ${SERVICE_NAME}"
  echo ""
  echo "Check logs:"
  echo "  sudo journalctl -u caddy -f"
  echo "  sudo journalctl -u ${SERVICE_NAME} -f"
else
  echo -e "${RED}Failed to start Caddy.${NC}"
  echo "Check logs:"
  echo "  sudo journalctl -u caddy -n 100"
  exit 1
fi
