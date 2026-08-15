package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"devin-byok/internal/config"
)

func TestOpenAIReasoningEffortPayload(t *testing.T) {
	c := &Client{cfg: config.UpstreamConfig{Thinking: config.ThinkingConfig{Param: "reasoning_effort"}}}
	body := c.buildChatRequest("grok", []ChatMessage{{Role: "user", Content: "hi"}}, false, ChatOptions{Thinking: "high"})
	b, _ := json.Marshal(body)
	if !strings.Contains(string(b), `"reasoning_effort":"high"`) {
		t.Fatalf("missing effort: %s", b)
	}
	if strings.Contains(string(b), `"thinking"`) {
		t.Fatalf("unexpected anthropic thinking: %s", b)
	}
}

func TestOpenAIReasoningNestedPayload(t *testing.T) {
	c := &Client{cfg: config.UpstreamConfig{Thinking: config.ThinkingConfig{Param: "reasoning.effort"}}}
	body := c.buildChatRequest("grok", nil, false, ChatOptions{Thinking: "medium"})
	if body.Reasoning == nil || body.Reasoning.Effort != "medium" {
		t.Fatalf("nested reasoning missing: %+v", body)
	}
}

func TestOpenAIStreamInterruptedWithoutDone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		fl.Flush()
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n"))
		fl.Flush()
		// 中转直接断开：无 [DONE]、无 finish_reason
	}))
	defer srv.Close()
	c := New(config.UpstreamConfig{})
	var got string
	_, err := c.StreamChat(context.Background(), "m", []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{BaseURL: srv.URL, APIKey: "k"}, func(d StreamDelta) error {
		got += d.Content
		return nil
	})
	if !errors.Is(err, ErrStreamInterrupted) {
		t.Fatalf("err = %v, want ErrStreamInterrupted", err)
	}
	if got != "hello world" {
		t.Fatalf("partial content lost: %q", got)
	}
}

func TestOpenAIStreamDoneWithoutFinishReasonIsOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()
	c := New(config.UpstreamConfig{})
	_, err := c.StreamChat(context.Background(), "m", []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{BaseURL: srv.URL, APIKey: "k"}, func(d StreamDelta) error { return nil })
	if err != nil {
		t.Fatalf("clean [DONE] stream must not error: %v", err)
	}
}

func TestOpenAIStreamFinishReasonWithoutDoneIsOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer srv.Close()
	c := New(config.UpstreamConfig{})
	_, err := c.StreamChat(context.Background(), "m", []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{BaseURL: srv.URL, APIKey: "k"}, func(d StreamDelta) error { return nil })
	if err != nil {
		t.Fatalf("finish_reason without [DONE] must be accepted: %v", err)
	}
}
