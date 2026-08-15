//go:build windows

package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func AutostartPath() string {
	return filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "devin-byok-serve.cmd")
}

func AutostartEnabled() bool {
	_, err := os.Stat(AutostartPath())
	return err == nil
}

func SetAutostart(on bool, projectDir, cliExe string) error {
	lnk := AutostartPath()
	if !on {
		_ = os.Remove(lnk)
		return nil
	}
	if strings.TrimSpace(cliExe) == "" {
		return fmt.Errorf("cli exe empty")
	}
	if strings.TrimSpace(projectDir) == "" {
		projectDir = filepath.Dir(cliExe)
	}
	content := fmt.Sprintf("@echo off\r\ncd /d \"%s\"\r\n\"%s\" start\r\n", projectDir, cliExe)
	return os.WriteFile(lnk, []byte(content), 0o644)
}
