package localapi

import "testing"

func TestRedactSecrets(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Authorization: Bearer abc123.def-ghi", "Authorization: [REDACTED]"},
		{"x-api-key=sk-abcdef1234567890", "x-api-key=[REDACTED]"},
		{"authorization: token-secret-value", "authorization: [REDACTED]"},
		{"token sk-abcdef1234567890 ok", "token sk-[REDACTED] ok"},
		{"no secrets here", "no secrets here"},
		{"", ""},
	}
	for _, c := range cases {
		got := redactSecrets(c.in)
		if got != c.want {
			t.Errorf("redactSecrets(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRedactStrings(t *testing.T) {
	got := redactStrings([]string{"Bearer secret", "plain"})
	if got[0] != "Bearer [REDACTED]" || got[1] != "plain" {
		t.Fatalf("redactStrings = %v", got)
	}
}
