package localapi

import (
	"bytes"
	"testing"
)

func TestGetChatMessageDeltaSchema(t *testing.T) {
	b := buildGetChatMessageDelta("mid-1", "hello", "thinking-here", false)
	if !bytes.Contains(b, []byte("hello")) {
		t.Fatalf("missing delta text: %x", b)
	}
	if !bytes.Contains(b, []byte("thinking-here")) {
		t.Fatalf("missing delta thinking: %x", b)
	}
	b2 := buildGetChatMessageResponse("mid-1", "cid", "hello", false)
	if !bytes.Contains(b2, []byte("hello")) {
		t.Fatalf("wrapper missing text")
	}
	t.Logf("withThinking=%d hex=%x", len(b), b)
}
