//go:build darwin

package update

import "testing"

func TestShellQuote(t *testing.T) {
	got := shellQuote("/tmp/a user's folder")
	want := "'/tmp/a user'\"'\"'s folder'"
	if got != want {
		t.Fatalf("shellQuote() = %q, want %q", got, want)
	}
}
