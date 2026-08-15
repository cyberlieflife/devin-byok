#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

if [ ! -x "$PROJECT_DIR/devin-byok" ]; then
  echo "ERROR: missing binary $PROJECT_DIR/devin-byok (build with: go build -o devin-byok ./cmd/devin-byok)" >&2
  exit 1
fi

nohup "$PROJECT_DIR/devin-byok" serve > /dev/null 2>&1 &
echo "local-api started on http://127.0.0.1:8787"
