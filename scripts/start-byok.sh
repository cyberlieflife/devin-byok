#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "[1/3] check binary..."
if [ ! -f "$PROJECT_DIR/devin-byok" ]; then
  echo "building devin-byok..."
  go build -o "$PROJECT_DIR/devin-byok" ./cmd/devin-byok
fi

echo "[2/3] ensure LS wrapper installed..."
DATA_DIR="$HOME/Library/Application Support/devin-byok"
if [ ! -f "$DATA_DIR/ls-wrapper-install.json" ]; then
  "$PROJECT_DIR/devin-byok" install
fi

echo "[3/3] start local-api on 127.0.0.1:8787..."
nohup "$PROJECT_DIR/devin-byok" serve > /dev/null 2>&1 &
sleep 2
curl -s http://127.0.0.1:8787/healthz
echo ""
echo "OK. Fully restart Devin, pick BYOK model, send message."
