#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

"$SCRIPT_DIR/stop-byok.sh"
"$PROJECT_DIR/devin-byok" uninstall

echo "Uninstall complete. Fully restart Devin."
