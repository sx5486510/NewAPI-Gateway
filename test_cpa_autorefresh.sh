#!/bin/bash

echo "=========================================="
echo "CPA Auto-Refresh Test Script"
echo "=========================================="
echo ""

echo "Step 1: Start Gateway and wait for CPA to initialize..."
echo "Expected log messages:"
echo "  - [SYS] embedded CPA starting on 127.0.0.1:xxxx"
echo "  - file watcher started for config and auth directory changes"
echo "  - core auth auto-refresh started (interval=15m0s)"
echo ""

echo "Step 2: Check logs after 10 seconds..."
echo ""

echo "Run this command to start Gateway:"
echo "  ./newapi-gateway"
echo ""

echo "In another terminal, run:"
echo "  tail -f logs/common.log | grep -E 'auto-refresh|watcher'"
echo ""

echo "You should see:"
echo "  [timestamp] level=info msg=\"core auth auto-refresh started (interval=15m0s)\""
echo "  [timestamp] level=info msg=\"file watcher started for config and auth directory changes\""
echo ""
