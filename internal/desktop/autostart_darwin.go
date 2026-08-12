//go:build darwin

package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const launchAgentLabel = "com.devin-byok.serve"

func AutostartPath() string {
	return filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", launchAgentLabel+".plist")
}

func AutostartEnabled() bool {
	_, err := os.Stat(AutostartPath())
	return err == nil
}

func SetAutostart(on bool, projectDir, cliExe string) error {
	plistPath := AutostartPath()
	if !on {
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
	_ = os.MkdirAll(dir, 0o755)
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
`, launchAgentLabel, cliExe, projectDir,
		filepath.Join(filepath.Dir(plistPath), "..", "Logs", launchAgentLabel+".out.log"),
		filepath.Join(filepath.Dir(plistPath), "..", "Logs", launchAgentLabel+".err.log"))
	return os.WriteFile(plistPath, []byte(content), 0o644)
}
