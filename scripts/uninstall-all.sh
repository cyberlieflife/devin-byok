#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ "$(basename "$SCRIPT_DIR")" = "scripts" ]; then
  PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
else
  PROJECT_DIR="$SCRIPT_DIR"
fi

"$SCRIPT_DIR/stop-byok.sh"
"$PROJECT_DIR/devin-byok" uninstall

echo "Uninstall complete. Fully restart Devin."
