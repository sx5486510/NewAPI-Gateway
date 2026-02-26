#!/bin/bash
#
# NewAPI-Gateway CentOS local build script
# Builds and deploys directly on the CentOS server (avoids cross-compile CGO issues).
# Usage: sudo bash build-on-centos.sh
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
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

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  NewAPI-Gateway Local Build Script${NC}"
echo -e "${GREEN}  For CentOS 7/8/Stream${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Require root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: please run as root.${NC}"
    echo "Usage: sudo bash build-on-centos.sh"
    exit 1
fi

# Install dependencies
echo -e "${YELLOW}[1/9] Check and install dependencies...${NC}"

# Go
if ! command -v go &> /dev/null; then
    echo -e "${YELLOW}Go not found, installing...${NC}"
    if [ -f /etc/centos-release ]; then
        CENTOS_VERSION=$(rpm -q --queryformat '%{VERSION}' centos-release)
    else
        CENTOS_VERSION=$(rpm -q --queryformat '%{VERSION}' redhat-release)
    fi
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

echo ""

# Create directories
echo -e "${YELLOW}[2/9] Create directories...${NC}"
mkdir -p "${SOURCE_DIR}"
mkdir -p "${INSTALL_DIR}/bin"
mkdir -p "${LOG_DIR}"
mkdir -p "${DATA_DIR}"
echo -e "${GREEN}Directories created.${NC}"
echo ""

# Check source code
echo -e "${YELLOW}[3/9] Check source code...${NC}"
if [ ! -f "${SOURCE_DIR}/main.go" ]; then
    echo -e "${RED}Error: source code not found in ${SOURCE_DIR}.${NC}"
    echo "Please upload the source code first, for example:"
    echo "1. Create a tarball (exclude build artifacts):"
    echo "   tar -czf newapi-gateway-source.tar.gz --exclude=node_modules --exclude=bin --exclude=web/build --exclude=web/node_modules ."
    echo "2. Upload to server:"
    echo "   scp newapi-gateway-source.tar.gz user@server:${SOURCE_DIR}/"
    echo "3. Extract on server:"
    echo "   cd ${SOURCE_DIR}"
    echo "   tar -xzf newapi-gateway-source.tar.gz"
    echo ""
    exit 1
fi
echo -e "${GREEN}Source code found.${NC}"
echo ""

# Build frontend
echo -e "${YELLOW}[4/9] Build frontend...${NC}"
cd "${SOURCE_DIR}/web"
echo "Installing npm dependencies..."
npm install --silent
if [ $? -ne 0 ]; then
    echo -e "${RED}Error: npm install failed.${NC}"
    exit 1
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
echo -e "${YELLOW}[5/9] Build backend...${NC}"
echo "Downloading Go modules..."
go mod download
if [ $? -ne 0 ]; then
    echo -e "${RED}Error: go mod download failed.${NC}"
    exit 1
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
echo -e "${YELLOW}[6/9] Set permissions...${NC}"
chmod +x "${INSTALL_DIR}/bin/${BINARY_NAME}"
chown -R root:root "${INSTALL_DIR}"
echo -e "${GREEN}Permissions set.${NC}"
echo ""

# Generate session secret
echo -e "${YELLOW}[7/9] Generate SESSION_SECRET...${NC}"
SESSION_SECRET=$(openssl rand -hex 32)
echo -e "${GREEN}SESSION_SECRET generated.${NC}"
echo ""

# Create systemd service
echo -e "${YELLOW}[8/9] Create systemd service...${NC}"
cat > "${SERVICE_FILE}" <<EOF
[Unit]
Description=NewAPI Gateway Service
Documentation=https://github.com/Calcium-Ion/new-api
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
Environment="SQL_DSN=sqlite://${DATA_DIR}/newapi.db"

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
echo -e "${YELLOW}[9/9] Create .env file...${NC}"
cat > "${INSTALL_DIR}/.env" <<EOF
# NewAPI-Gateway .env
# Generated: $(date)

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
echo -e "${YELLOW}Starting service...${NC}"
systemctl daemon-reload
systemctl enable "${SERVICE_NAME}"
systemctl restart "${SERVICE_NAME}"
echo -e "${GREEN}Service started.${NC}"
echo ""

# Verify status
sleep 2
if systemctl is-active --quiet "${SERVICE_NAME}"; then
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
    echo "Rebuild after code changes:"
    echo "  cd ${SOURCE_DIR}"
    echo "  sudo bash build-on-centos.sh"
    echo ""
    echo "Access:"
    echo "  http://$(hostname -I | awk '{print $1}'):${PORT}"
    echo "  http://$(curl -s ifconfig.me):${PORT}"
    echo ""
    echo "Security tips:"
    echo "  1. Open only required ports in firewall"
    echo "  2. Consider enabling HTTPS (Let's Encrypt)"
    echo "  3. Keep system and app updated"
    echo "  4. Protect secrets in .env"
    echo ""
else
    echo -e "${RED}Service failed to start.${NC}"
    echo "Check logs:"
    echo "  sudo journalctl -u ${SERVICE_NAME} -n 50"
    echo ""
    exit 1
fi
