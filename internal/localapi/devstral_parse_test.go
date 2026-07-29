package localapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDevstralFromCapturedFrame(t *testing.T) {
	// 使用用户 capture 中的真实 Connect+gzip 帧
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".devin-byok", "work", "capture", "bodies")
	matches, _ := filepath.Glob(filepath.Join(dir, "*GetDevstralStream*"))
	if len(matches) == 0 {
		t.Skip("no captured GetDevstralStream bodies")
	}
	// newest
	newest := matches[0]
	fi, _ := os.Stat(newest)
	for _, m := range matches[1:] {
		if s, err := os.Stat(m); err == nil && s.ModTime().After(fi.ModTime()) {
			newest, fi = m, s
		}
	}
	raw, err := os.ReadFile(newest)
	if err != nil {
		t.Fatal(err)
	}
	plain := decodeConnectPayloads(raw)
	if len(plain) < 100 {
		t.Fatalf("decode failed len=%d file=%s", len(plain), newest)
	}
	req := parseDevstralRequest(plain)
	if len(req.Messages) == 0 {
		t.Fatalf("no messages parsed from %s plain=%d", newest, len(plain))
	}
	if len(req.Tools) == 0 {
		// fallback extractor
		req.Tools = extractToolsFromPlain(plain)
	}
	if len(req.Tools) == 0 {
		t.Fatalf("no tools parsed from %s", newest)
	}
	names := map[string]bool{}
	for _, tl := range req.Tools {
		names[tl.Function.Name] = true
	}
	if !names["restricted_exec"] {
		t.Fatalf("missing restricted_exec in %v", names)
	}
	t.Logf("file=%s msgs=%d tools=%d names=%v", filepath.Base(newest), len(req.Messages), len(req.Tools), names)
}
