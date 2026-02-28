#!/bin/bash
#
# Configure Caddy TLS reverse proxy for an already deployed NewAPI-Gateway service.
#
# Usage:
#   sudo bash deploy-with-caddy.sh --domain gateway.example.com --app-port 3030 --cert-file /etc/ssl/newapi/fullchain.pem --key-file /etc/ssl/newapi/privkey.pem
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SERVICE_NAME="newapi-gateway"
DOMAIN="gateway.example.com"
APP_PORT=3030
CADDY_PORT=3031
CERT_FILE=""
KEY_FILE=""
CADDY_GITHUB_URL="https://github.com/caddyserver/caddy/releases/download/v2.11.1/caddy_2.11.1_linux_amd64.tar.gz"

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
      shift 2
      ;;
    --key-file)
      KEY_FILE="$2"
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

if [ -z "$CERT_FILE" ] || [ -z "$KEY_FILE" ]; then
  echo -e "${RED}Error: certificate is required. Please provide both --cert-file and --key-file.${NC}"
  echo ""
  echo "Usage:"
  echo "  sudo bash deploy-with-caddy.sh --domain ${DOMAIN} --app-port ${APP_PORT} --cert-file /etc/ssl/newapi/fullchain.pem --key-file /etc/ssl/newapi/privkey.pem"
  echo ""
  echo "Example:"
  echo "sudo mkdir -p /etc/ssl/newapi && sudo unzip -o gateway.example.com_nginx.zip -d /etc/ssl/newapi && sudo chmod 600 /etc/ssl/newapi/*"
  echo "  sudo bash deploy-with-caddy.sh --domain gateway.example.com --app-port 3030 --cert-file /etc/ssl/newapi/gateway.example.com_bundle.crt --key-file /etc/ssl/newapi/gateway.example.com.key"
  exit 1
fi
if [ ! -f "$CERT_FILE" ] || [ ! -f "$KEY_FILE" ]; then
  echo -e "${RED}Error: certificate file or key file not found.${NC}"
  exit 1
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

  echo -e "${YELLOW}Installing Caddy from GitHub release...${NC}"
  install_pkg curl
  install_pkg tar

  ARCH=$(uname -m)
  if [ "${ARCH}" != "x86_64" ]; then
    echo -e "${RED}This script uses fixed amd64 package, unsupported architecture: ${ARCH}${NC}"
    exit 1
  fi

  TMP_DIR=$(mktemp -d)
  CADDY_TGZ="${TMP_DIR}/caddy.tar.gz"
  if ! curl -fsSL "${CADDY_GITHUB_URL}" -o "${CADDY_TGZ}"; then
    echo -e "${RED}Failed to download Caddy from GitHub: ${CADDY_GITHUB_URL}${NC}"
    rm -rf "${TMP_DIR}"
    exit 1
  fi
  tar -xzf "${CADDY_TGZ}" -C "${TMP_DIR}"
  install -m 755 "${TMP_DIR}/caddy" /usr/local/bin/caddy
  rm -rf "${TMP_DIR}"

  if ! command -v caddy >/dev/null 2>&1; then
    echo -e "${RED}Failed to install Caddy binary.${NC}"
    exit 1
  fi

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
}

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Configure Caddy For NewAPI-Gateway${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "${BLUE}Domain: ${DOMAIN}${NC}"
echo -e "${BLUE}Caddy port: ${CADDY_PORT}${NC}"
echo -e "${BLUE}App port: ${APP_PORT}${NC}"
echo -e "${BLUE}TLS mode: custom certificate (required)${NC}"
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
TLS_LINE="    tls ${CERT_FILE} ${KEY_FILE}"

cat > /etc/caddy/Caddyfile <<EOF
${DOMAIN}:${CADDY_PORT} {
${TLS_LINE}
    reverse_proxy http://127.0.0.1:${APP_PORT} {
        header_up Host 127.0.0.1:${APP_PORT}
    }
}
EOF

echo -e "${YELLOW}[4/5] Starting Caddy...${NC}"
/usr/local/bin/caddy validate --config /etc/caddy/Caddyfile
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
