package devinwrap_test

import (
	"os"
	"runtime"
	"testing"

	"devin-byok/internal/devinwrap"
	"devin-byok/internal/payload"
)

func TestMaterializeDevinWrapper(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("devin wrapper is only embedded for Windows/macOS releases")
	}
	if len(payload.DevinWrapper) < 1000 {
		t.Fatal("devin wrapper empty")
	}
	p, err := devinwrap.MaterializeWrapper()
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil || st.Size() == 0 {
		t.Fatal("missing", p, err)
	}
	t.Log("ok", p, st.Size())
}
