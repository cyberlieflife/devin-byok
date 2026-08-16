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
	file     *os.File
	currSize int64
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

func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		_ = os.MkdirAll(filepath.Dir(w.path), 0o700)
		st, err := os.Stat(w.path)
		if err == nil {
			w.currSize = st.Size()
		}
		f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return 0, err
		}
		w.file = f
	}

	if w.currSize+int64(len(p)) > w.maxBytes {
		w.rotateLocked()
	}

	if w.file == nil {
		return 0, os.ErrInvalid
	}

	n, err := w.file.Write(p)
	if err == nil {
		w.currSize += int64(n)
	}
	return n, err
}

func (w *RotatingWriter) rotateLocked() {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	for i := w.keep - 1; i >= 1; i-- {
		from := w.path + "." + itoa(i)
		to := w.path + "." + itoa(i+1)
		_ = os.Remove(to)
		_ = os.Rename(from, to)
	}
	_ = os.Remove(w.path + ".1")
	_ = os.Rename(w.path, w.path+".1")
	_ = os.Remove(w.path + "." + itoa(w.keep+1))

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		w.file = f
		w.currSize = 0
	}
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
