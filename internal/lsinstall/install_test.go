package lsinstall_test

import (
	"os"
	"runtime"
	"testing"

	"devin-byok/internal/lsinstall"
	"devin-byok/internal/payload"
)

func TestMaterialize(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("wrapper binary is only embedded for Windows/macOS releases")
	}
	if len(payload.LSWrapper) < 1000 {
		t.Fatal("wrapper empty")
	}
	if len(payload.ConfigExample) < 100 {
		t.Fatal("config empty")
	}
	p, err := lsinstall.MaterializeWrapper()
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil || st.Size() == 0 {
		t.Fatal("missing", p, err)
	}
	t.Log("ok", p, st.Size())
}
