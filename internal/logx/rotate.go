package logx

import (
	"os"
	"path/filepath"
	"sync"
)

// RotatingWriter 按大小轮转的追加写（A3）。
type RotatingWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	keep     int
}

func NewRotatingWriter(path string, maxBytes int64, keep int) *RotatingWriter {
	if maxBytes <= 0 {
		maxBytes = 32 << 20
	}
	if keep <= 0 {
		keep = 3
	}
	return &RotatingWriter{path: path, maxBytes: maxBytes, keep: keep}
}

func (w *RotatingWriter) WriteLine(s string) error {
	_, err := w.Write([]byte(s))
	return err
}

func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = os.MkdirAll(filepath.Dir(w.path), 0o755)
	if st, err := os.Stat(w.path); err == nil && st.Size()+int64(len(p)) > w.maxBytes {
		w.rotateLocked()
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return f.Write(p)
}

func (w *RotatingWriter) rotateLocked() {
	for i := w.keep - 1; i >= 1; i-- {
		from := w.path + "." + itoa(i)
		to := w.path + "." + itoa(i+1)
		_ = os.Remove(to)
		_ = os.Rename(from, to)
	}
	_ = os.Remove(w.path + ".1")
	_ = os.Rename(w.path, w.path+".1")
	_ = os.Remove(w.path + "." + itoa(w.keep+1))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
