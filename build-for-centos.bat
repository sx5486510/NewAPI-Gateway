@echo off
REM NewAPI-Gateway 打包脚本 - 为 CentOS (Linux amd64) 构建可执行文件
REM 切换到 UTF-8 编码以正确显示中文
chcp 65001 >nul

echo ========================================
echo  NewAPI-Gateway 打包脚本
echo  目标平台: CentOS Linux (amd64)
echo ========================================

echo.
echo [1/4] 检查环境...
where go >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo 错误: 未找到 Go，请先安装 Go
    exit /b 1
)

where npm >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo 错误: 未找到 npm，请先安装 Node.js
    exit /b 1
)

echo.
echo [2/4] 构建前端静态资源...
cd web
call npm install
if %ERRORLEVEL% neq 0 (
    echo 错误: npm install 失败
    exit /b 1
)
call npm run build
if %ERRORLEVEL% neq 0 (
    echo 错误: 前端构建失败
    exit /b 1
)
cd ..

echo.
echo [3/4] 设置交叉编译环境变量...
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=0

echo.
echo [4/4] 编译 Linux 可执行文件...
echo.
echo 注意: 此脚本使用 CGO_ENABLED=0 生成纯静态可执行文件，
echo       但 SQLite3 需要启用 CGO。因此有两种部署方式：
echo.
echo 方式一: 一键部署（推荐，支持 SQLite）
echo   1. 直接在服务器执行
echo   2. 在服务器上运行: bash deploy/deploy-on-centos.sh
echo.
echo 方式二: 交叉编译（需要使用 MySQL/PostgreSQL）
echo   使用本脚本编译，然后在 CentOS 上配置 SQL_DSN 环境变量
echo.
if not exist bin mkdir bin
set /p VERSION=<VERSION
go build -ldflags "-s -w -X 'NewAPI-Gateway/common.Version=%VERSION%'" -o ./bin/gateway-aggregator
if %ERRORLEVEL% neq 0 (
    echo 错误: Go 编译失败
    exit /b 1
)

echo.
echo ========================================
echo  构建成功!
echo  可执行文件位置: ./bin/gateway-aggregator
echo ========================================
echo.
echo 文件大小:
dir .\bin\gateway-aggregator | find "gateway-aggregator"
echo.
echo 下一步 - 最简单的部署方式:
echo.
echo ========================================
echo 一键部署（推荐）
echo   直接在 CentOS 上从 GitHub 拉取并编译
echo ========================================
echo.
echo 1. SSH 登录到 CentOS 服务器:
echo    ssh user@server
echo.
echo 2. 下载并运行部署脚本:
echo    curl -fsSL https://raw.githubusercontent.com/sx5486510/NewAPI-Gateway/main/deploy/deploy-on-centos.sh -o deploy-on-centos.sh
echo    sudo bash deploy-on-centos.sh
echo.
echo 部署脚本会自动完成:
echo   - 安装依赖（Git、Go、Node.js、GCC）
echo   - 从 GitHub 克隆最新代码
echo   - 构建前端和编译后端（支持 SQLite）
echo   - 配置 systemd 服务（开机自启）
echo   - 启动服务
echo.
echo 部署完成后访问:
echo   http://your-server-ip:3030
echo.
echo 默认登录信息:
echo   用户名: root
echo   密码: 123456
echo.
echo ========================================
echo 其他部署方式
echo ========================================
echo.
echo 方式二: 使用本地的预编译二进制
echo   限制: 不支持 SQLite，必须配置 MySQL/PostgreSQL
echo.
echo 1. 传输可执行文件到 CentOS:
echo    scp .\bin\gateway-aggregator user@server:/opt/newapi-gateway/
echo.
echo 2. 在服务器上运行（需要先配置数据库）:
echo    ssh user@server
echo    cd /opt/newapi-gateway
echo    # 修改数据库配置为 MySQL/PostgreSQL
echo    # 然后运行: sudo ./gateway-aggregator --port 3030
echo.

pause
