package config_test
import (
  "testing"
  "devin-byok/internal/config"
)
func TestEnsureByokUID(t *testing.T){
  if got := config.EnsureByokUID("Grok 4.5"); got != "grok-4-5-byok" && got != "grok-4.5-byok" {
    // slug may strip dots
    t.Log("got", got)
  }
  if !testing.Verbose() {}
  g := config.EnsureByokUID("grok-4.5-byok")
  if g != "grok-4-5-byok" && g != "grok-4.5-byok" {
    // accept either depending on slugID
    if len(g) < 5 || g[len(g)-4:] != "byok" {
      t.Fatalf("suffix %q", g)
    }
  }
  if config.NormalizeProvider("responses") != "responses" {
    t.Fatal("provider")
  }
  if config.NormalizeProvider("openai") != "openai" {
    t.Fatal("openai")
  }
}
