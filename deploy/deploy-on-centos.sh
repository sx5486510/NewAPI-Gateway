#!/bin/bash
#
# NewAPI-Gateway CentOS one-click deploy script
# Clones from GitHub and deploys on CentOS server.
# Usage: sudo bash deploy-on-centos.sh [branch] [--https domain]
#
# Examples:
#   sudo bash deploy-on-centos.sh                    # Deploy with HTTP only
#   sudo bash deploy-on-centos.sh develop             # Deploy with 'develop' branch
#   sudo bash deploy-on-centos.sh --https example.com  # Deploy with HTTPS
#   sudo bash deploy-on-centos.sh develop --https example.com  # Deploy with specific branch and HTTPS
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
INSTALL_DIR="/opt/newapi-gateway"
SERVICE_NAME="newapi-gateway"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
SOURCE_DIR="${INSTALL_DIR}/source"
BINARY_NAME="gateway-aggregator"
PORT=3030
LOG_DIR="${INSTALL_DIR}/logs"
DATA_DIR="${INSTALL_DIR}/data"
REPO_URL="https://github.com/sx5486510/NewAPI-Gateway.git"
BRANCH="main"
ENABLE_HTTPS=false
DOMAIN=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --https)
            ENABLE_HTTPS=true
            if [ -n "$2" ] && [[ $2 != -* ]]; then
                DOMAIN="$2"
                shift
            fi
            shift
            ;;
        *)
            if [[ $1 != -* ]]; then
                BRANCH="$1"
            fi
            shift
            ;;
    esac
done

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  NewAPI-Gateway One-Click Deploy${NC}"
echo -e "${GREEN}  For CentOS 7/8/Stream${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Show deployment mode
if [ "$ENABLE_HTTPS" = true ]; then
    if [ -z "$DOMAIN" ]; then
        echo -e "${YELLOW}Warning: HTTPS enabled but no domain specified.${NC}"
        echo -e "${YELLOW}Usage: sudo bash deploy-on-centos.sh --https your-domain.com${NC}"
        exit 1
    fi
    echo -e "${GREEN}Deployment mode: HTTPS${NC}"
    echo -e "${GREEN}Domain: ${DOMAIN}${NC}"
else
    echo -e "${BLUE}Deployment mode: HTTP${NC}"
    echo -e "${YELLOW}Tip: Use --https your-domain.com to enable HTTPS${NC}"
fi
echo ""

# Require root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: please run as root.${NC}"
    echo "Usage: sudo bash deploy-on-centos.sh [branch] [--https domain]"
    exit 1
fi

# Show branch
echo -e "${BLUE}Using branch: ${BRANCH}${NC}"
echo ""

# Install dependencies
echo -e "${YELLOW}[1/10] Check and install dependencies...${NC}"

# Git
if ! command -v git &> /dev/null; then
    echo -e "${YELLOW}Git not found, installing...${NC}"
    yum install -y git
    echo -e "${GREEN}Git installed.${NC}"
else
    echo -e "${GREEN}Git installed: $(git --version)${NC}"
fi

# Go
if ! command -v go &> /dev/null; then
    echo -e "${YELLOW}Go not found, installing...${NC}"
    yum install -y epel-release
    yum install -y golang
    echo -e "${GREEN}Go installed.${NC}"
else
    echo -e "${GREEN}Go installed: $(go version)${NC}"
fi

# Node.js
if ! command -v node &> /dev/null; then
    echo -e "${YELLOW}Node.js not found, installing...${NC}"
    yum install -y nodejs npm
    echo -e "${GREEN}Node.js installed.${NC}"
else
    echo -e "${GREEN}Node.js installed: $(node -v)${NC}"
fi

# GCC for SQLite CGO
if ! command -v gcc &> /dev/null; then
    echo -e "${YELLOW}GCC not found, installing...${NC}"
    yum groupinstall -y "Development Tools"
    echo -e "${GREEN}GCC installed.${NC}"
else
    echo -e "${GREEN}GCC installed: $(gcc --version | head -n1)${NC}"
fi

# OpenSSL for SESSION_SECRET
if ! command -v openssl &> /dev/null; then
    echo -e "${YELLOW}OpenSSL not found, installing...${NC}"
    yum install -y openssl
    echo -e "${GREEN}OpenSSL installed.${NC}"
else
    echo -e "${GREEN}OpenSSL installed.${NC}"
fi

# Certbot for HTTPS (if enabled)
if [ "$ENABLE_HTTPS" = true ]; then
    if ! command -v certbot &> /dev/null; then
        echo -e "${YELLOW}Certbot not found, installing...${NC}"
        yum install -y epel-release
        yum install -y certbot
        echo -e "${GREEN}Certbot installed.${NC}"
    else
        echo -e "${GREEN}Certbot installed: $(certbot --version | head -n1)${NC}"
    fi
fi

echo ""

# Create directories
echo -e "${YELLOW}[2/10] Create directories...${NC}"
mkdir -p "${INSTALL_DIR}/bin"
mkdir -p "${SOURCE_DIR}"
mkdir -p "${LOG_DIR}"
mkdir -p "${DATA_DIR}"
echo -e "${GREEN}Directories created.${NC}"
echo ""

# Clone or update source
echo -e "${YELLOW}[3/10] Fetch source code...${NC}"
if [ -d "${SOURCE_DIR}/.git" ]; then
    echo "Updating existing repository..."
    cd "${SOURCE_DIR}"
    git fetch origin
    git checkout "${BRANCH}"
    git pull origin "${BRANCH}"
else
    echo "Cloning from GitHub..."
    rm -rf "${SOURCE_DIR}"
    git clone -b "${BRANCH}" "${REPO_URL}" "${SOURCE_DIR}"
fi
echo -e "${GREEN}Source code ready.${NC}"
echo ""

# Show version
if [ -f "${SOURCE_DIR}/VERSION" ]; then
    VERSION=$(cat "${SOURCE_DIR}/VERSION")
    echo -e "${BLUE}Version: ${VERSION}${NC}"
fi
echo ""

# Build frontend
echo -e "${YELLOW}[4/10] Build frontend...${NC}"
cd "${SOURCE_DIR}/web"
echo "Installing npm dependencies..."
npm install --silent
if [ $? -ne 0 ]; then
    echo -e "${RED}npm install failed, retry with mirror...${NC}"
    npm install --registry=https://registry.npmmirror.com
fi
echo "Building frontend..."
npm run build
if [ $? -ne 0 ]; then
    echo -e "${RED}Error: frontend build failed.${NC}"
    exit 1
fi
cd "${SOURCE_DIR}"
echo -e "${GREEN}Frontend build completed.${NC}"
echo ""

# Build Go program
echo -e "${YELLOW}[5/10] Build backend...${NC}"
echo "Downloading Go modules..."
go mod download
if [ $? -ne 0 ]; then
    echo -e "${RED}go mod download failed, retry with proxy...${NC}"
    GOPROXY=https://goproxy.cn,direct go mod download
fi

echo "Building..."
# Enable CGO for SQLite
CGO_ENABLED=1 go build -ldflags "-s -w" -o "${INSTALL_DIR}/bin/${BINARY_NAME}"
if [ $? -ne 0 ]; then
    echo -e "${RED}Error: go build failed.${NC}"
    exit 1
fi
echo -e "${GREEN}Build completed.${NC}"
echo ""

# Permissions
echo -e "${YELLOW}[6/10] Set permissions...${NC}"
chmod +x "${INSTALL_DIR}/bin/${BINARY_NAME}"
chown -R root:root "${INSTALL_DIR}"
echo -e "${GREEN}Permissions set.${NC}"
echo ""

# Generate session secret
echo -e "${YELLOW}[7/10] Generate SESSION_SECRET...${NC}"
SESSION_SECRET=$(openssl rand -hex 32)
echo -e "${GREEN}SESSION_SECRET generated.${NC}"
echo ""

# Stop old service if exists
if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    echo -e "${YELLOW}Stopping existing service...${NC}"
    systemctl stop "${SERVICE_NAME}"
fi

# Create systemd service
echo -e "${YELLOW}[8/10] Create systemd service...${NC}"
cat > "${SERVICE_FILE}" <<EOF
[Unit]
Description=NewAPI Gateway Service
Documentation=https://github.com/sx5486510/NewAPI-Gateway
After=network.target
Wants=network.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=${INSTALL_DIR}

ExecStart=${INSTALL_DIR}/bin/${BINARY_NAME} --port ${PORT} --log-dir ${LOG_DIR}

Restart=always
RestartSec=5s
StartLimitInterval=0

Environment="GIN_MODE=release"
Environment="PORT=${PORT}"
Environment="LOG_DIR=${LOG_DIR}"
Environment="SESSION_SECRET=${SESSION_SECRET}"
# Environment="SQL_DSN=sqlite://${DATA_DIR}/newapi.db"

NoNewPrivileges=true
PrivateTmp=true

LimitNOFILE=65536
MemoryLimit=1G
CPUQuota=100%

StandardOutput=journal
StandardError=journal
SyslogIdentifier=newapi-gateway

[Install]
WantedBy=multi-user.target
EOF

echo -e "${GREEN}Service file created: ${SERVICE_FILE}${NC}"
echo ""

# Create .env
echo -e "${YELLOW}[9/10] Create .env file...${NC}"
cat > "${INSTALL_DIR}/.env" <<EOF
# NewAPI-Gateway .env
# Generated: $(date)
# Repo: ${REPO_URL}
# Branch: ${BRANCH}

# Server
PORT=${PORT}
LOG_DIR=${LOG_DIR}
GIN_MODE=release

# Security
SESSION_SECRET=${SESSION_SECRET}

# Database (SQLite by default)
SQL_DSN=sqlite://${DATA_DIR}/newapi.db

# MySQL example:
# SQL_DSN=username:password@tcp(localhost:3306)/newapi?charset=utf8mb4&parseTime=True&loc=Local

# PostgreSQL example:
# SQL_DSN=host=localhost user=username password=password dbname=newapi port=5432 sslmode=disable TimeZone=Asia/Shanghai

# Redis (optional)
# REDIS_CONN_STRING=redis://localhost:6379
# REDIS_PASSWORD=

# Other
# REDIS_ENABLED=false
EOF

chmod 600 "${INSTALL_DIR}/.env"
echo -e "${GREEN}.env file created: ${INSTALL_DIR}/.env${NC}"
echo ""

# Start service
echo -e "${YELLOW}[10/10] Starting service...${NC}"
systemctl daemon-reload
systemctl enable "${SERVICE_NAME}"
systemctl start "${SERVICE_NAME}"
echo -e "${GREEN}Service started.${NC}"
echo ""

# Configure HTTPS if enabled
if [ "$ENABLE_HTTPS" = true ]; then
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}Configuring HTTPS...${NC}"
    echo -e "${YELLOW}========================================${NC}"
    echo ""
    
    # Open firewall ports
    echo -e "${YELLOW}[11/12] Configure firewall...${NC}"
    if command -v firewall-cmd &> /dev/null; then
        echo "Opening ports 80 and 443..."
        firewall-cmd --permanent --add-port=80/tcp 2>/dev/null || true
        firewall-cmd --permanent --add-port=443/tcp 2>/dev/null || true
        firewall-cmd --reload 2>/dev/null || true
        echo -e "${GREEN}Firewall configured.${NC}"
    else
        echo -e "${YELLOW}firewalld not found, skipping firewall configuration.${NC}"
    fi
    echo ""
    
    # Wait for DNS to propagate (optional)
    echo -e "${YELLOW}[12/12] Obtaining SSL certificate...${NC}"
    echo "This may take a few minutes..."
    echo ""
    
    # Obtain certificate
    certbot certonly --standalone -d "${DOMAIN}" --non-interactive --agree-tos --register-unsafely-without-email 2>/dev/null || {
        echo -e "${RED}Failed to obtain SSL certificate.${NC}"
        echo -e "${YELLOW}Please ensure:${NC}"
        echo "  1. Domain ${DOMAIN} resolves to this server's IP"
        echo "  2. Ports 80 and 443 are accessible from the internet"
        echo "  3. No other service is using port 80"
        echo ""
        echo "You can obtain the certificate later manually:"
        echo "  certbot certonly --standalone -d ${DOMAIN}"
        echo ""
        ENABLE_HTTPS=false
    }
    
    if [ "$ENABLE_HTTPS" = true ]; then
        echo -e "${GREEN}SSL certificate obtained successfully!${NC}"
        echo ""
        
        # Setup auto-renewal
        echo "Setting up auto-renewal..."
        systemctl enable --now certbot-renew.timer 2>/dev/null || true
        echo -e "${GREEN}Auto-renewal configured.${NC}"
        echo ""
    fi
fi

# Verify status
sleep 3
if systemctl is-active --quiet "${SERVICE_NAME}"; then
    LOCAL_IP=$(hostname -I | awk '{print $1}')
    PUBLIC_IP=$(curl -s ifconfig.me 2>/dev/null || echo "unknown")

    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  Deployment completed${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "Service info:"
    echo "  Name: ${SERVICE_NAME}"
    echo "  Install dir: ${INSTALL_DIR}"
    echo "  Source dir: ${SOURCE_DIR}"
    echo "  Binary: ${INSTALL_DIR}/bin/${BINARY_NAME}"
    echo "  Logs: ${LOG_DIR}"
    echo "  Data: ${DATA_DIR}"
    echo "  Port: ${PORT}"
    echo "  .env: ${INSTALL_DIR}/.env"
    echo ""
    echo "Service management:"
    echo "  status:  sudo systemctl status ${SERVICE_NAME}"
    echo "  start:   sudo systemctl start ${SERVICE_NAME}"
    echo "  stop:    sudo systemctl stop ${SERVICE_NAME}"
    echo "  restart: sudo systemctl restart ${SERVICE_NAME}"
    echo "  logs:    sudo journalctl -u ${SERVICE_NAME} -f"
    echo "  logs:    tail -f ${LOG_DIR}/*.log"
    echo ""
    echo "Update code:"
    echo "  cd ${SOURCE_DIR}"
    echo "  git pull"
    echo "  sudo bash ${SOURCE_DIR}/deploy/deploy-on-centos.sh"
    echo ""
    echo "Or re-run deploy script:"
    echo "  sudo bash ${SOURCE_DIR}/deploy/deploy-on-centos.sh"
    echo ""
    echo "Access:"
    if [ "$ENABLE_HTTPS" = true ]; then
        echo "  HTTPS: https://${DOMAIN}"
        echo "  HTTP:  http://${DOMAIN} (will redirect to HTTPS)"
    else
        echo "  Local:  http://${LOCAL_IP}:${PORT}"
        echo "  Public: http://${PUBLIC_IP}:${PORT}"
    fi
    echo ""
    echo "Default credentials:"
    echo "  user: root"
    echo "  pass: 123456"
    echo "  Please change the password after login."
    echo ""
    echo "Security tips:"
    echo "  1. Open only required ports in firewall"
    if [ "$ENABLE_HTTPS" = false ]; then
        echo "  2. Consider enabling HTTPS: sudo bash ${SOURCE_DIR}/deploy/deploy-on-centos.sh --https your-domain.com"
    else
        echo "  2. HTTPS is enabled with Let's Encrypt"
    fi
    echo "  3. Keep system and app updated"
    echo "  4. Protect secrets in .env"
    echo "  5. Change default password"
    echo ""
else
    echo -e "${RED}Service failed to start.${NC}"
    echo "Check logs:"
    echo "  sudo journalctl -u ${SERVICE_NAME} -n 50"
    echo ""
    exit 1
fi
