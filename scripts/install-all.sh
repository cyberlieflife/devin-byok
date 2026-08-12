#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "Building..."
go build -o "$PROJECT_DIR/devin-byok" ./cmd/devin-byok
go build -o "$PROJECT_DIR/devin-byok-gui" ./cmd/devin-byok-gui
go build -o "$PROJECT_DIR/devin-byok-ls-wrapper" ./cmd/ls-wrapper

echo ""
echo "Installing LS wrapper..."
"$PROJECT_DIR/devin-byok" install

echo ""
echo "Install complete. Run: $SCRIPT_DIR/start-byok.sh"
