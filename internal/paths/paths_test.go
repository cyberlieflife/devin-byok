package paths_test
import (
  "os"
  "path/filepath"
  "testing"
  "devin-byok/internal/paths"
)
func TestEnsureConfig(t *testing.T){
  p, err := paths.EnsureConfig()
  if err != nil { t.Fatal(err) }
  if filepath.Base(filepath.Dir(p)) != ".devin-byok" && filepath.Base(filepath.Dir(p)) != "devin-byok" {
    t.Log("dir", filepath.Dir(p))
  }
  if _, err := os.Stat(p); err != nil { t.Fatal(err) }
  t.Log(p)
}
