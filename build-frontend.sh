#!/usr/bin/env bash
set -euo pipefail

skip_install=0
if [[ "${1:-}" == "--skip-install" ]]; then
    skip_install=1
elif [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    echo "Usage: $0 [--skip-install]"
    exit 0
elif [[ -n "${1:-}" ]]; then
    echo "ERROR: unknown option: $1" >&2
    echo "Usage: $0 [--skip-install]" >&2
    exit 1
fi

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
web_dir="$root_dir/web"
build_dir="$web_dir/build"

echo "========================================"
echo "  NewAPI-Gateway frontend web build"
echo "========================================"
echo

echo "[1/4] Checking Node.js and npm..."
if ! command -v node >/dev/null 2>&1; then
    echo "ERROR: Node.js was not found. Install Node.js first." >&2
    exit 1
fi
if ! command -v npm >/dev/null 2>&1; then
    echo "ERROR: npm was not found. Install npm first." >&2
    exit 1
fi
echo "OK: Node.js $(node --version)"
echo "OK: npm $(npm --version)"
echo

echo "[2/4] Checking web directory..."
if [[ ! -d "$web_dir" ]]; then
    echo "ERROR: web directory does not exist: $web_dir" >&2
    exit 1
fi
echo "OK: $web_dir"
echo

cd "$web_dir"

echo "[3/4] Checking dependencies..."
if [[ "$skip_install" -eq 1 ]]; then
    echo "SKIP: dependency install was skipped."
elif [[ ! -d "node_modules" ]]; then
    echo "node_modules was not found. Running npm install..."
    npm install
else
    echo "OK: node_modules exists."
fi
echo

echo "[4/4] Building frontend web..."
npm run build

if [[ ! -d "$build_dir" ]]; then
    echo "ERROR: build output was not created: $build_dir" >&2
    exit 1
fi

if command -v du >/dev/null 2>&1; then
    build_size="$(du -sh "$build_dir" 2>/dev/null | awk '{print $1}')"
else
    build_size="unknown"
fi

echo
echo "========================================"
echo "  Frontend web build completed"
echo "========================================"
echo "Output: web/build/"
echo "Size: $build_size"
echo
