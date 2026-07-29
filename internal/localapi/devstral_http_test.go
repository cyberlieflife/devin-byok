package localapi

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"devin-byok/internal/config"
	"devin-byok/internal/pbwire"
)

func TestDevstralHTTPHandlerReturnsToolCall(t *testing.T) {
	home, _ := os.UserHomeDir()
	matches, _ := filepath.Glob(filepath.Join(home, ".devin-byok", "work", "capture", "bodies", "*GetDevstralStream*"))
	if len(matches) == 0 {
		t.Skip("no capture")
	}
	// newest
	var newest string
	var newestMod int64
	for _, m := range matches {
		st, err := os.Stat(m)
		if err != nil {
			continue
		}
		if st.ModTime().UnixNano() > newestMod {
			newestMod = st.ModTime().UnixNano()
			newest = m
		}
	}
	raw, err := os.ReadFile(newest)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.File{}
	cfg.Features.EnableFastContext = true
	cfg.Features.PureLocal = true
	cfg.Features.EnableStream = true
	s := New(cfg, t.TempDir())

	req := httptest.NewRequest(http.MethodPost, "/_route/api_server/exa.api_server_pb.ApiServerService/GetDevstralStream", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/connect+proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	t.Logf("status=%d ctype=%s bodyLen=%d", res.StatusCode, res.Header.Get("Content-Type"), len(body))
	if res.StatusCode != 200 {
		t.Fatalf("status %d body=%q", res.StatusCode, body)
	}
	// parse connect frames
	var toolNames []string
	var outputs []string
	i := 0
	for i+5 <= len(body) {
		flags := body[i]
		n := int(binary.BigEndian.Uint32(body[i+1 : i+5]))
		i += 5
		if i+n > len(body) {
			t.Fatalf("truncated frame flags=%d n=%d remain=%d", flags, n, len(body)-i)
		}
		payload := body[i : i+n]
		i += n
		t.Logf("frame flags=%d len=%d", flags, len(payload))
		if flags&0x02 != 0 {
			t.Logf("end stream: %s", payload)
			continue
		}
		fields := pbwire.ParseFields(payload)
		for _, f := range fields {
			if f.Number == 2 && f.Wire == 2 {
				outputs = append(outputs, string(f.Bytes))
			}
			if f.Number == 3 && f.Wire == 2 {
				sub := pbwire.ParseFields(f.Bytes)
				var name string
				for _, sf := range sub {
					if sf.Number == 2 {
						name = string(sf.Bytes)
					}
				}
				toolNames = append(toolNames, name)
				t.Logf("tool call subfields=%d name=%q", len(sub), name)
			}
		}
	}
	t.Logf("outputs=%v tools=%v", outputs, toolNames)
	if len(toolNames) == 0 {
		t.Fatalf("expected tool_calls in response; hex head=%x", body[:min(64, len(body))])
	}
	if toolNames[0] != "restricted_exec" && toolNames[0] != "answer" {
		t.Fatalf("unexpected tool %q", toolNames[0])
	}
}
