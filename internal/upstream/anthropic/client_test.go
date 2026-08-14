package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"devin-byok/internal/upstream/openai"
)

func TestAnthropicThinkingPayload(t *testing.T) {
	c := New()
	b, err := c.build("claude", []openai.ChatMessage{{Role: "user", Content: "hi"}}, false, openai.ChatOptions{MaxTokens: 4096, ThinkingType: "enabled", ThinkingBudgetTokens: 3000})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	thinking, ok := got["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(3000) {
		t.Fatalf("bad thinking payload: %s", b)
	}
}

func TestAnthropicNonThinkingOmitsThinking(t *testing.T) {
	b, err := New().build("claude", nil, false, openai.ChatOptions{MaxTokens: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"thinking"`) {
		t.Fatalf("unexpected thinking: %s", b)
	}
}

func TestAnthropicThinkingBudgetValidation(t *testing.T) {
	_, err := New().build("claude", nil, false, openai.ChatOptions{MaxTokens: 3000, ThinkingType: "enabled", ThinkingBudgetTokens: 3000})
	if err == nil || !strings.Contains(err.Error(), "must be less than max_tokens") {
		t.Fatalf("unexpected validation result: %v", err)
	}
}

func TestAnthropicStreamInterruptedWithoutMessageStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n"))
		fl.Flush()
		// 中转直接断开：无 message_stop
	}))
	defer srv.Close()
	c := New()
	var got string
	_, err := c.StreamChat(context.Background(), srv.URL, "k", "claude", []openai.ChatMessage{{Role: "user", Content: "hi"}}, openai.ChatOptions{}, func(d openai.StreamDelta) error {
		got += d.Content
		return nil
	})
	if !errors.Is(err, openai.ErrStreamInterrupted) {
		t.Fatalf("err = %v, want ErrStreamInterrupted", err)
	}
	if got != "hi" {
		t.Fatalf("partial content lost: %q", got)
	}
}

func TestAnthropicStreamMessageStopIsOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()
	c := New()
	_, err := c.StreamChat(context.Background(), srv.URL, "k", "claude", []openai.ChatMessage{{Role: "user", Content: "hi"}}, openai.ChatOptions{}, func(d openai.StreamDelta) error { return nil })
	if err != nil {
		t.Fatalf("clean message_stop stream must not error: %v", err)
	}
}
