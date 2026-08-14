//go:build darwin

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func scheduleApply(extractDir, installDir, guiName, tmp string) (string, error) {
	script := filepath.Join(tmp, "apply-update.sh")
	content := fmt.Sprintf(`#!/bin/bash
set -e
SRC=%s
DST=%s
GUI=%s
APP="Devin BYOK.app"
CLI=devin-byok
echo "[devin-byok] waiting for old process to exit..."
sleep 2
if [ ! -e "$SRC/$APP" ] && [ ! -e "$SRC/devin-byok-gui.app" ] && [ ! -e "$SRC/$GUI" ]; then
  for candidate in "$SRC"/*; do
    if [ -e "$candidate/$APP" ] || [ -e "$candidate/devin-byok-gui.app" ] || [ -e "$candidate/$GUI" ]; then
      SRC="$candidate"
      break
    fi
  done
fi
echo "[devin-byok] applying update to $DST"
mkdir -p "$DST"
if [ -d "$SRC/$APP" ]; then
  rm -rf "$DST/$APP.new"
  ditto "$SRC/$APP" "$DST/$APP.new"
  rm -rf "$DST/$APP"
  mv "$DST/$APP.new" "$DST/$APP"
  chmod +x "$DST/$APP/Contents/MacOS/$GUI"
elif [ -d "$SRC/devin-byok-gui.app" ]; then
  rm -rf "$DST/$APP.new"
  ditto "$SRC/devin-byok-gui.app" "$DST/$APP.new"
  rm -rf "$DST/$APP"
  mv "$DST/$APP.new" "$DST/$APP"
  chmod +x "$DST/$APP/Contents/MacOS/$GUI"
elif [ -f "$SRC/$GUI" ]; then
  cp -f "$SRC/$GUI" "$DST/$GUI"
  chmod +x "$DST/$GUI"
else
  echo "[devin-byok] ERROR: missing GUI binary"
  exit 1
fi
if [ -f "$SRC/$CLI" ]; then cp -f "$SRC/$CLI" "$DST/$CLI"; chmod +x "$DST/$CLI"; fi
for name in START.txt config.example.yaml start-byok.sh stop-byok.sh uninstall-all.sh; do
  if [ -f "$SRC/$name" ]; then cp -f "$SRC/$name" "$DST/$name"; fi
done
echo "[devin-byok] update applied"
if [ -d "$DST/$APP" ]; then
  nohup open "$DST/$APP" > /dev/null 2>&1 &
else
  nohup "$DST/$GUI" > /dev/null 2>&1 &
fi
echo "[devin-byok] done"
`, shellQuote(extractDir), shellQuote(installDir), shellQuote(guiName))
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		return "", err
	}
	cmd := exec.Command("/bin/bash", script)
	cmd.Dir = tmp
	if err := cmd.Start(); err != nil {
		return "", err
	}
	return script, nil
}

func scheduleApplyArtifact(artifactPath, installDir, guiName, tmp string) (string, error) {
	script := filepath.Join(tmp, "apply-update.sh")
	content := fmt.Sprintf(`#!/bin/bash
set -euo pipefail
DMG=%s
DST=%s
GUI=%s
APP="Devin BYOK.app"
MOUNT=$(mktemp -d "${TMPDIR:-/tmp}/devin-byok-mount.XXXXXX")
cleanup() {
  hdiutil detach "$MOUNT" -quiet >/dev/null 2>&1 || true
  rmdir "$MOUNT" >/dev/null 2>&1 || true
}
trap cleanup EXIT
echo "[devin-byok] waiting for old process to exit..."
sleep 2
hdiutil attach "$DMG" -nobrowse -readonly -mountpoint "$MOUNT" >/dev/null
SRC="$MOUNT/$APP"
if [ ! -d "$SRC" ]; then
  SRC=$(find "$MOUNT" -maxdepth 1 -type d -name "*.app" -print -quit)
fi
if [ -z "$SRC" ] || [ ! -d "$SRC" ]; then
  echo "[devin-byok] ERROR: DMG does not contain an app"
  exit 1
fi
echo "[devin-byok] applying update to $DST"
mkdir -p "$DST"
rm -rf "$DST/$APP.new"
ditto "$SRC" "$DST/$APP.new"
rm -rf "$DST/$APP"
mv "$DST/$APP.new" "$DST/$APP"
chmod +x "$DST/$APP/Contents/MacOS/$GUI"
echo "[devin-byok] update applied"
nohup open "$DST/$APP" >/dev/null 2>&1 &
echo "[devin-byok] done"
`, shellQuote(artifactPath), shellQuote(installDir), shellQuote(guiName))
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		return "", err
	}
	cmd := exec.Command("/bin/bash", script)
	cmd.Dir = tmp
	if err := cmd.Start(); err != nil {
		return "", err
	}
	return script, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
