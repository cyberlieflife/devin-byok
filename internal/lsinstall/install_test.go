package lsinstall_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"devin-byok/internal/lsinstall"
	"devin-byok/internal/payload"
	"devin-byok/internal/platform"
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

func TestEnsureRealCopy(t *testing.T) {
	// 构造假 bundle：installDir 下按平台 bin 子路径放置原版语言服务器。
	installDir := t.TempDir()
	bin := lsinstall.BinDir(installDir)
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	lsName := platform.LanguageServerName()
	realName := platform.RealLanguageServerName()
	fake := []byte("fake language server binary for test")
	if err := os.WriteFile(filepath.Join(bin, lsName), fake, 0o755); err != nil {
		t.Fatal(err)
	}

	// 注入目标目录，避免写入真实数据目录。
	dstDir := t.TempDir()
	orig := lsinstall.RealCopyDirForTest()
	lsinstall.SetRealCopyDirForTest(func() string { return dstDir })
	defer lsinstall.SetRealCopyDirForTest(orig)

	p, err := lsinstall.EnsureRealCopy(installDir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dstDir, realName)
	if p != want {
		t.Fatalf("path=%q want %q", p, want)
	}
	got, err := os.ReadFile(want)
	if err != nil || !bytes.Equal(got, fake) {
		t.Fatalf("real copy mismatch: err=%v len=%d", err, len(got))
	}

	// 幂等：内容一致时第二次调用不重写（mtime 不变）。
	st1, _ := os.Stat(want)
	if _, err := lsinstall.EnsureRealCopy(installDir); err != nil {
		t.Fatal(err)
	}
	st2, _ := os.Stat(want)
	if st1.ModTime() != st2.ModTime() {
		t.Fatal("second call should skip rewrite")
	}

	// 源缺失时报错。
	empty := t.TempDir()
	if _, err := lsinstall.EnsureRealCopy(empty); err == nil {
		t.Fatal("expected error for missing bundle")
	}

	// RemoveRealCopy 删除副本。
	if err := lsinstall.RemoveRealCopy(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatal("real copy should be removed after RemoveRealCopy")
	}
}
