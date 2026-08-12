//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func scheduleApply(extractDir, installDir, guiName, tmp string) (string, error) {
	script := filepath.Join(tmp, "apply-update.ps1")
	srcPS := powershellSingleQuote(extractDir)
	dstPS := powershellSingleQuote(installDir)
	ps := strings.Join([]string{
		"$ErrorActionPreference = 'Continue'",
		"Write-Host '[devin-byok] waiting for old process to exit...'",
		"$src = " + srcPS,
		"$dst = " + dstPS,
		"$gui = Join-Path $dst '" + guiName + "'",
		"$srcGui = Join-Path $src '" + guiName + "'",
		"if (-not (Test-Path -LiteralPath $srcGui)) { Write-Host '[devin-byok] ERROR: missing' $srcGui; exit 1 }",
		"# 最多等 40s，直到目标 exe 可写（旧 GUI 已退出）",
		"for ($i = 0; $i -lt 80; $i++) {",
		"  try {",
		"    if (Test-Path -LiteralPath $gui) {",
		"      $fs = [System.IO.File]::Open($gui, 'Open', 'ReadWrite', 'None')",
		"      $fs.Close()",
		"    }",
		"    break",
		"  } catch {",
		"    Start-Sleep -Milliseconds 500",
		"  }",
		"}",
		"Write-Host '[devin-byok] applying update to' $dst",
		"New-Item -ItemType Directory -Force -Path $dst | Out-Null",
		"Copy-Item -LiteralPath $srcGui -Destination $gui -Force",
		"if (Test-Path -LiteralPath (Join-Path $src 'START.txt')) { Copy-Item -LiteralPath (Join-Path $src 'START.txt') -Destination (Join-Path $dst 'START.txt') -Force }",
		"if (Test-Path -LiteralPath (Join-Path $src 'config.example.yaml')) { Copy-Item -LiteralPath (Join-Path $src 'config.example.yaml') -Destination (Join-Path $dst 'config.example.yaml') -Force }",
		"Write-Host '[devin-byok] update applied'",
		"Start-Process -FilePath $gui -WorkingDirectory $dst",
		"Write-Host '[devin-byok] done'",
		"exit 0",
	}, "\r\n") + "\r\n"
	if err := os.WriteFile(script, []byte(ps), 0o755); err != nil {
		return "", err
	}
	cmd := exec.Command("powershell",
		"-NoProfile", "-ExecutionPolicy", "Bypass",
		"-WindowStyle", "Hidden",
		"-File", script,
	)
	cmd.Dir = tmp
	cmd.SysProcAttr = hiddenSysProcAttr()
	if err := cmd.Start(); err != nil {
		return "", err
	}
	return script, nil
}

func powershellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
