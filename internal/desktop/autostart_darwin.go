//go:build darwin

package desktop

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const launchAgentLabel = "com.devin-byok.serve"

func AutostartPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
}

func AutostartEnabled() bool {
	_, err := os.Stat(AutostartPath())
	return err == nil
}

func SetAutostart(on bool, projectDir, cliExe string) error {
	plistPath := AutostartPath()
	if !on {
		_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d", os.Getuid()), plistPath).Run()
		_ = os.Remove(plistPath)
		return nil
	}
	if strings.TrimSpace(cliExe) == "" {
		return fmt.Errorf("cli exe empty")
	}
	if strings.TrimSpace(projectDir) == "" {
		projectDir = filepath.Dir(cliExe)
	}
	dir := filepath.Dir(plistPath)
	logDir := filepath.Join(filepath.Dir(dir), "Logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>start</string>
	</array>
	<key>WorkingDirectory</key>
	<string>%s</string>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<false/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, html.EscapeString(launchAgentLabel), html.EscapeString(cliExe), html.EscapeString(projectDir),
		html.EscapeString(filepath.Join(logDir, launchAgentLabel+".out.log")),
		html.EscapeString(filepath.Join(logDir, launchAgentLabel+".err.log")))
	if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
		return err
	}
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d", os.Getuid()), plistPath).Run()
	if err := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), plistPath).Run(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w", err)
	}
	return nil
}
