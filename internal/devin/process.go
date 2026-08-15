package devin

import (
	"fmt"
	"strings"
	"time"
)

type ProcessResult struct {
	Running    bool   `json:"running"`
	InstallDir string `json:"install_dir"`
	Message    string `json:"message"`
}

// Restart closes every Devin window, waits for file locks to settle, and
// starts the installed desktop app again.
func Restart(installDir string) (*ProcessResult, error) {
	installDir = strings.TrimSpace(installDir)
	if err := stopDevinProcess(); err != nil {
		return nil, fmt.Errorf("关闭 Devin 失败: %w", err)
	}
	time.Sleep(900 * time.Millisecond)
	if err := startDevinProcess(installDir); err != nil {
		return nil, fmt.Errorf("启动 Devin 失败: %w", err)
	}
	return &ProcessResult{Running: true, InstallDir: installDir, Message: "Devin 已重启，本地模型配置正在加载"}, nil
}

func ProcessStatus(installDir string) ProcessResult {
	running := isDevinProcessRunning()
	message := "Devin 未运行"
	if running {
		message = "Devin 正在运行"
	}
	return ProcessResult{Running: running, InstallDir: strings.TrimSpace(installDir), Message: message}
}
