package devin

import (
	"os"
	"path/filepath"
)

// DetectDataDirs 返回可能的 Devin/Windsurf 用户数据目录。
func DetectDataDirs() []string {
	var out []string
	appdata := os.Getenv("APPDATA")
	home, _ := os.UserHomeDir()
	cands := []string{
		filepath.Join(appdata, "Devin"),
		filepath.Join(home, ".devin"),
		filepath.Join(appdata, "Windsurf"),
		filepath.Join(home, ".windsurf"),
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			out = append(out, c)
		}
	}
	return out
}
