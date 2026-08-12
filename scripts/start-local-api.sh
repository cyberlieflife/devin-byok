#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

nohup "$PROJECT_DIR/devin-byok" serve > /dev/null 2>&1 &
echo "local-api started on http://127.0.0.1:8787"
