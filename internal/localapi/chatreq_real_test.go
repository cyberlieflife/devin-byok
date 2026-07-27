package localapi

import (
	"os"
	"testing"
)

func TestParseRealCapturedGetChatMessage(t *testing.T) {
	path := `D:\Devin-byok\work\capture\sample-getchatmessage.plain.bin`
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skip(err)
	}
	p := parseGetChatMessageRequest(b)
	if p.ModelUID == "" {
		t.Fatalf("model empty")
	}
	if len(p.Tools) < 5 {
		t.Fatalf("tools=%d model=%s sys=%d user=%q", len(p.Tools), p.ModelUID, len(p.SystemPrompt), p.UserText)
	}
	if p.UserText == "" {
		t.Fatalf("user text empty; sys head=%q", truncate(p.SystemPrompt, 80))
	}
	t.Logf("model=%s tools=%d hist=%d user=%q sys=%d", p.ModelUID, len(p.Tools), len(p.Messages), p.UserText, len(p.SystemPrompt))
}
