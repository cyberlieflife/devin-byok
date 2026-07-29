package localapi

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"devin-byok/internal/pbwire"
	"devin-byok/internal/upstream/openai"
)

func TestBuildDevstralResponseValidUTF8(t *testing.T) {
	bad := string([]byte{0xff, 0xfe, 'h', 'i'})
	b := buildDevstralResponse(bad, []openaiToolCallView{{
		ID: "call_1", Name: "code_search", Arguments: `{"query":"main"}`,
	}})
	fields := pbwire.ParseFields(b)
	var out string
	var tools int
	for _, f := range fields {
		if f.Number == 2 && f.Wire == 2 {
			out = string(f.Bytes)
			if !utf8.ValidString(out) {
				t.Fatal("output not valid utf8")
			}
		}
		if f.Number == 3 {
			tools++
		}
	}
	if !strings.Contains(out, "hi") {
		t.Fatalf("output=%q", out)
	}
	if !strings.Contains(out, "[TOOL_CALLS]code_search[ARGS]") {
		t.Fatalf("mistral tool call missing in output=%q", out)
	}
	if tools != 1 {
		t.Fatalf("tools=%d", tools)
	}
}

func TestIsDevstralRPC(t *testing.T) {
	if !isDevstralRPC("exa.api_server_pb.ApiServerService/GetDevstralStream") {
		t.Fatal("expected devstral")
	}
	if isChatRPC("exa.api_server_pb.ApiServerService/GetDevstralStream") {
		t.Fatal("devstral must not be classified as GetChatMessage chat RPC")
	}
}

func TestBuildDevstralNotChatMessageShape(t *testing.T) {
	b := buildDevstralResponse("hello-fc", nil)
	_ = bytes.Contains(pbwire.ConnectFrame(0, b), []byte{})
	fields := pbwire.ParseFields(b)
	has2 := false
	for _, f := range fields {
		if f.Number == 2 {
			has2 = true
		}
	}
	if !has2 {
		t.Fatal("missing field 2 output")
	}
}

func TestBuildDevstralMistralOutputAndToolFields(t *testing.T) {
	b := buildDevstralResponse("", []openaiToolCallView{{
		ID: "call_1", Name: "restricted_exec", Arguments: `{"command1":{"type":"rg","pattern":"main","path":"."}}`,
	}})
	fields := pbwire.ParseFields(b)
	var tools int
	var names []string
	var output string
	for _, f := range fields {
		if f.Number == 2 && f.Wire == 2 {
			output = string(f.Bytes)
		}
		if f.Number == 3 && f.Wire == 2 {
			tools++
			sub := pbwire.ParseFields(f.Bytes)
			for _, sf := range sub {
				if sf.Number == 2 && sf.Wire == 2 {
					names = append(names, string(sf.Bytes))
				}
			}
		}
	}
	if tools != 1 || len(names) != 1 || names[0] != "restricted_exec" {
		t.Fatalf("tools=%d names=%v", tools, names)
	}
	if !strings.HasPrefix(output, "[TOOL_CALLS]restricted_exec[ARGS]{") {
		t.Fatalf("output mistral format bad: %q", output)
	}
}

func TestSynthesizeRestrictedExecSchema(t *testing.T) {
	calls := synthesizeRestrictedExec("Find README entry", "")
	if len(calls) != 1 {
		t.Fatalf("%+v", calls)
	}
	args := calls[0].Function.Arguments
	if strings.Contains(args, `"depth"`) {
		t.Fatalf("tree must use levels not depth: %s", args)
	}
	if !strings.Contains(args, `"levels"`) {
		t.Fatalf("missing levels: %s", args)
	}
	if !strings.Contains(args, `"file":"README.md"`) {
		t.Fatalf("missing readfile file: %s", args)
	}
	if !strings.Contains(args, `"type":"rg"`) {
		t.Fatalf("missing rg: %s", args)
	}
}


func TestBuildFastContextAnswerXML(t *testing.T) {
	xml := buildFastContextAnswerXML("README.md\npackage.json\nsrc\n", "overview")
	if !strings.Contains(xml, "<ANSWER>") || !strings.Contains(xml, "</ANSWER>") {
		t.Fatalf("%s", xml)
	}
	if !strings.Contains(xml, `path="/codebase/README.md"`) {
		t.Fatalf("path missing: %s", xml)
	}
	if !strings.Contains(xml, "<range>") {
		t.Fatalf("range missing: %s", xml)
	}
	calls := synthesizeAnswerTool("README.md\nsrc\n", "q")
	if len(calls) != 1 || calls[0].Function.Name != "answer" {
		t.Fatalf("%+v", calls)
	}
	if !strings.Contains(calls[0].Function.Arguments, "<ANSWER>") && !strings.Contains(calls[0].Function.Arguments, "\\u003cANSWER") {
		t.Fatalf("args=%s", calls[0].Function.Arguments)
	}
}

func TestEnsureAnswerToolXML(t *testing.T) {
	bad := []openai.ToolCall{{ID: "1", Type: "function"}}
	bad[0].Function.Name = "answer"
	bad[0].Function.Arguments = `{"answer":"just plain text"}`
	out := ensureAnswerToolXML(bad, "README.md\npackage.json", "q")
	if len(out) != 1 || (!strings.Contains(out[0].Function.Arguments, "<ANSWER>") && !strings.Contains(out[0].Function.Arguments, "\\u003cANSWER")) {
		t.Fatalf("%+v", out)
	}
}
