package localapi

import (
	"strings"
	"testing"
)

func TestParseGetDeepWikiRequestFields(t *testing.T) {
	// 手工构造：field2=1, field3="for", field4="file://x", field5="ctx", field7="English", field8=1
	var b []byte
	b = appendVarintTest(b, (2<<3)|0)
	b = appendVarintTest(b, 1)
	b = appendStringFieldTest(b, 3, "for")
	b = appendStringFieldTest(b, 4, "file://x")
	b = appendStringFieldTest(b, 5, "hello context")
	b = appendStringFieldTest(b, 7, "English")
	b = appendVarintTest(b, (8<<3)|0)
	b = appendVarintTest(b, 1)

	req := parseGetDeepWikiRequest(b)
	if req.RequestType != 1 || req.SymbolName != "for" || req.SymbolURI != "file://x" {
		t.Fatalf("parse basic: %+v", req)
	}
	if req.Context != "hello context" || req.Language != "English" || !req.GenerateFollowups {
		t.Fatalf("parse more: %+v", req)
	}
}

func TestBuildGetDeepWikiDeltaContainsText(t *testing.T) {
	// api_server schema: wraps GetChatMessageResponse with delta_text
	frame := buildGetDeepWikiDelta("mid-1", "HelloWiki", true, false, false)
	if !strings.Contains(string(frame), "HelloWiki") {
		t.Fatalf("missing text in frame: %q", frame)
	}
	if !strings.Contains(string(frame), "mid-1") {
		t.Fatalf("missing message id")
	}
}

func TestSplitDeepWikiFollowups(t *testing.T) {
	body, fu := splitDeepWikiFollowups("Article body\n\nFOLLOWUPS: a | b | c")
	if body != "Article body" || !strings.Contains(fu, "a | b") {
		t.Fatalf("body=%q fu=%q", body, fu)
	}
}

func TestBuildCodeMapSuccess(t *testing.T) {
	b := buildCodeMapSuccess(`{"title":"t"}`, "cid", true)
	if !strings.Contains(string(b), `{"title":"t"}`) {
		t.Fatalf("missing json")
	}
	if !strings.Contains(string(b), "cid") {
		t.Fatalf("missing cascade id")
	}
}

func appendVarintTest(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func appendStringFieldTest(b []byte, field int, s string) []byte {
	b = appendVarintTest(b, uint64((field<<3)|2))
	b = appendVarintTest(b, uint64(len(s)))
	return append(b, s...)
}
