package update

import (
	"sync"
	"time"
)

// Progress 下载/更新进度（供 GUI 轮询）。
type Progress struct {
	Phase     string  `json:"phase"` // idle|checking|downloading|verifying|scheduling|done|error
	Percent   float64 `json:"percent"`
	Bytes     int64   `json:"bytes"`
	Total     int64   `json:"total"`
	Message   string  `json:"message"`
	UpdatedAt string  `json:"updated_at"`
}

var (
	progMu sync.RWMutex
	prog   = Progress{Phase: "idle", Message: "空闲", UpdatedAt: time.Now().Format(time.RFC3339)}
)

func GetProgress() Progress {
	progMu.RLock()
	defer progMu.RUnlock()
	return prog
}

func setProgress(phase string, percent float64, bytes, total int64, msg string) {
	progMu.Lock()
	defer progMu.Unlock()
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	prog = Progress{
		Phase: phase, Percent: percent, Bytes: bytes, Total: total,
		Message: msg, UpdatedAt: time.Now().Format(time.RFC3339),
	}
}

// ResetProgress 重置为空闲。
func ResetProgress() {
	setProgress("idle", 0, 0, 0, "空闲")
}
