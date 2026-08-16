package localapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// dumpBody 把完整请求体写入抓包目录（仅在显式开启时调用）。
func (s *Server) dumpBody(method string, raw []byte) {
	if !s.captureOn || s.bodyDir == "" || len(raw) == 0 {
		return
	}
	name := fmt.Sprintf("%s_%d.bin", sanitize(method), time.Now().UnixNano())
	_ = os.WriteFile(filepath.Join(s.bodyDir, name), raw, 0o600)
}

// appendRPC 追加一行 RPC 元数据（仅在显式开启时调用）。
func (s *Server) appendRPC(rec map[string]any) {
	if !s.captureOn {
		return
	}
	s.rpcMu.Lock()
	defer s.rpcMu.Unlock()
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	b = append(b, '\n')
	if s.rpcRotator != nil {
		_, _ = s.rpcRotator.Write(b)
		return
	}
	// fallback
	f, err := os.OpenFile(s.rpcLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(b)
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func truncateB64(b []byte, maxRaw int) string {
	if len(b) > maxRaw {
		b = b[:maxRaw]
	}
	return base64.StdEncoding.EncodeToString(b)
}
