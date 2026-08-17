#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
cd "$ROOT"

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

PACK_TMP=$(mktemp -d /tmp/devin-byok-pack.XXXXXX)
PAYLOAD_PATH="$ROOT/internal/payload/ls-wrapper"
if [ ! -f "$PAYLOAD_PATH" ]; then
  echo "missing macOS payload: $PAYLOAD_PATH" >&2
  exit 1
fi
cp "$PAYLOAD_PATH" "$PACK_TMP/ls-wrapper"
PAYLOAD_DEVIN="$ROOT/internal/payload/devin-wrapper"
if [ ! -f "$PAYLOAD_DEVIN" ]; then
  echo "missing macOS devin payload: $PAYLOAD_DEVIN" >&2
  exit 1
fi
cp "$PAYLOAD_DEVIN" "$PACK_TMP/devin-wrapper"
cleanup() {
  cp "$PACK_TMP/ls-wrapper" "$PAYLOAD_PATH"
  cp "$PACK_TMP/devin-wrapper" "$PAYLOAD_DEVIN"
  rm -rf "$PACK_TMP"
}
trap cleanup EXIT

echo "Building self-contained macOS app release $VERSION for darwin-$GOARCH..."

# Build ls-wrapper / devin-wrapper for embedding
GOOS=darwin GOARCH="$GOARCH" go build -ldflags "-s -w" -o "$PAYLOAD_PATH" "$ROOT/cmd/ls-wrapper"
GOOS=darwin GOARCH="$GOARCH" go build -ldflags "-s -w" -o "$PAYLOAD_DEVIN" "$ROOT/cmd/devin-wrapper"

LD_FLAGS="-X devin-byok/internal/version.Version=$VERSION -X devin-byok/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%S)"

# Build GUI
GOOS=darwin GOARCH="$GOARCH" CGO_ENABLED=1 go build -ldflags "-s -w $LD_FLAGS" -o "$ROOT/devin-byok-gui" "$ROOT/cmd/devin-byok-gui"

DIST="$ROOT/dist"
STAGE="$DIST/devin-byok-$VERSION-darwin-$GOARCH"
rm -rf "$STAGE"
mkdir -p "$STAGE"

APP="$STAGE/Devin BYOK.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$ROOT/devin-byok-gui" "$APP/Contents/MacOS/devin-byok-gui"
chmod +x "$APP/Contents/MacOS/devin-byok-gui"
ICON_SRC="$ROOT/internal/desktop/macos-icon.png"
if [ ! -f "$ICON_SRC" ]; then
  echo "missing macOS icon: $ICON_SRC" >&2
  exit 1
fi
ICONSET="$PACK_TMP/Devin BYOK.iconset"
mkdir -p "$ICONSET"
for spec in \
  '16 icon_16x16.png' '32 icon_16x16@2x.png' \
  '32 icon_32x32.png' '64 icon_32x32@2x.png' \
  '128 icon_128x128.png' '256 icon_128x128@2x.png' \
  '256 icon_256x256.png' '512 icon_256x256@2x.png' \
  '512 icon_512x512.png' '1024 icon_512x512@2x.png'; do
  set -- $spec
  sips -z "$1" "$1" "$ICON_SRC" --out "$ICONSET/$2" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/AppIcon.icns"
cat > "$APP/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDisplayName</key><string>Devin BYOK</string>
  <key>CFBundleExecutable</key><string>devin-byok-gui</string>
  <key>CFBundleIdentifier</key><string>com.devin-byok.gui</string>
  <key>CFBundleName</key><string>Devin BYOK</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundleIconName</key><string>AppIcon</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
  <key>CFBundleVersion</key><string>$VERSION</string>
  <key>LSMinimumSystemVersion</key><string>10.15</string>
  <key>LSUIElement</key><false/>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
EOF
DMG="$DIST/devin-byok-$VERSION-darwin-$GOARCH.dmg"
rm -f "$DMG"
DMG_TMP="$DIST/.dmg-$VERSION-darwin-$GOARCH"
rm -rf "$DMG_TMP"
mkdir -p "$DMG_TMP"
cp -R "$APP" "$DMG_TMP/"
ln -s /Applications "$DMG_TMP/Applications"
hdiutil create -volname "Devin BYOK" -srcfolder "$DMG_TMP" -ov -format UDZO "$DMG" >/dev/null
rm -rf "$DMG_TMP"

SHA=$(shasum -a 256 "$DMG" | awk '{print $1}')
echo "$SHA" > "$DMG.sha256"

echo "OK $DMG"
echo "SHA256 $SHA"
ls -lh "$DMG" "$DMG.sha256"
