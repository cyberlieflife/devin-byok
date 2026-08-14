#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ "$(basename "$SCRIPT_DIR")" = "scripts" ]; then
  PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
else
  PROJECT_DIR="$SCRIPT_DIR"
fi
cd "$PROJECT_DIR"

echo "[1/3] check binary..."
if [ ! -f "$PROJECT_DIR/devin-byok" ]; then
  echo "building devin-byok..."
  go build -o "$PROJECT_DIR/devin-byok" ./cmd/devin-byok
fi

echo "[2/3] ensure LS wrapper installed..."
"$PROJECT_DIR/devin-byok" install

echo "[3/3] start local-api on 127.0.0.1:8787..."
"$PROJECT_DIR/devin-byok" start
for _ in $(seq 1 20); do
  if curl -fsS http://127.0.0.1:8787/healthz; then
    echo ""
    break
  fi
  sleep 0.2
done
echo "OK. Fully restart Devin, pick BYOK model, send message."
