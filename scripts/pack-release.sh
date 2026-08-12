#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"

if [ -z "$VERSION" ]; then
  VERSION=$(grep -o 'Version = "[^"]*"' "$ROOT/internal/version/version.go" | cut -d'"' -f2)
fi
if [ -z "$VERSION" ]; then
  VERSION="0.0.0"
fi

ARCH="${GOARCH:-$(uname -m)}"
case "$ARCH" in
  arm64|aarch64) GOARCH="arm64" ;;
  x86_64|amd64)  GOARCH="amd64" ;;
esac
export GOARCH

echo "Building self-contained GUI release $VERSION for darwin-$GOARCH..."

# Build ls-wrapper for embedding
GOOS=darwin go build -ldflags "-s -w" -o "$ROOT/internal/payload/ls-wrapper" ./cmd/ls-wrapper

# Copy config template
cp "$ROOT/internal/payload/config.example.yaml" /tmp/config.example.yaml 2>/dev/null || true

LD_FLAGS="-X devin-byok/internal/version.Version=$VERSION -X devin-byok/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%S)"

# Build CLI
GOOS=darwin go build -ldflags "$LD_FLAGS" -o devin-byok ./cmd/devin-byok

# Build GUI
GOOS=darwin CGO_ENABLED=1 go build -ldflags "-s -w $LD_FLAGS" -o devin-byok-gui ./cmd/devin-byok-gui

DIST="$ROOT/dist"
STAGE="$DIST/devin-byok-$VERSION-darwin-$GOARCH"
mkdir -p "$STAGE"

cp devin-byok-gui "$STAGE/"
cp devin-byok "$STAGE/"

cat > "$STAGE/START.txt" <<EOF
Devin BYOK v$VERSION (macOS GUI)

1. Run ./devin-byok-gui (may need: xattr -d com.apple.quarantine devin-byok-gui)
2. Configure models/providers in GUI
3. Start service in GUI (auto apply + LS wrapper)
4. Fully quit and reopen Devin, pick a BYOK model

Quit GUI: service stops automatically and Devin settings restore.

Repo: https://github.com/cyberlieflife/devin-byok
License: AGPL-3.0
EOF

# Also create convenience scripts
cp "$ROOT/scripts/start-byok.sh" "$STAGE/"
cp "$ROOT/scripts/stop-byok.sh" "$STAGE/"
cp "$ROOT/scripts/uninstall-all.sh" "$STAGE/"
cp "$ROOT/internal/payload/config.example.yaml" "$STAGE/"

ZIP="$DIST/devin-byok-$VERSION-darwin-$GOARCH.zip"
rm -f "$ZIP"
(cd "$DIST" && zip -r "$(basename "$ZIP")" "$(basename "$STAGE")" -x "*.DS_Store")

SHA=$(shasum -a 256 "$ZIP" | awk '{print $1}')
echo "$SHA" > "$ZIP.sha256"

echo "OK $ZIP"
echo "SHA256 $SHA"
ls -la "$STAGE/"
