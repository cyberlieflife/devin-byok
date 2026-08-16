package extinstall

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"
)

// TestEmbeddedExtensionMatchesSource 保证内嵌的扩展文件与 extension/ 源副本一致，
// 避免改一处漏一处导致安装包与仓库源漂移。
func TestEmbeddedExtensionMatchesSource(t *testing.T) {
	const srcRoot = "../../extension/devin-byok-prompt"
	files := []string{"extension.js", "package.json", "media/icon.svg"}
	for _, rel := range files {
		emb, err := fs.ReadFile(ExtFS, path.Join(ExtRoot, rel))
		if err != nil {
			t.Fatalf("embedded %s: %v", rel, err)
		}
		disk, err := os.ReadFile(filepath.Join(srcRoot, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("source %s: %v", rel, err)
		}
		if string(emb) != string(disk) {
			t.Errorf("%s: embedded (%d bytes) != source (%d bytes)", rel, len(emb), len(disk))
		}
	}
}
