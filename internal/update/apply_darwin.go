//go:build darwin

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func scheduleApply(extractDir, installDir, guiName, tmp string) (string, error) {
	script := filepath.Join(tmp, "apply-update.sh")
	content := fmt.Sprintf(`#!/bin/bash
set -e
SRC=%q
DST=%q
GUI=%q
echo "[devin-byok] waiting for old process to exit..."
sleep 2
echo "[devin-byok] applying update to $DST"
mkdir -p "$DST"
cp -f "$SRC/$GUI" "$DST/$GUI"
chmod +x "$DST/$GUI"
if [ -f "$SRC/START.txt" ]; then cp -f "$SRC/START.txt" "$DST/START.txt"; fi
if [ -f "$SRC/config.example.yaml" ]; then cp -f "$SRC/config.example.yaml" "$DST/config.example.yaml"; fi
echo "[devin-byok] update applied"
nohup "$DST/$GUI" > /dev/null 2>&1 &
echo "[devin-byok] done"
`, extractDir, installDir, guiName)
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
