package localapi

import (
	"testing"

	"devin-byok/internal/pbwire"
	"devin-byok/internal/upstream/openai"
)

func TestParseGetChatMessageToolsAndHistory(t *testing.T) {
	// 构造最小化 GetChatMessageRequest 字段：
	// 2=system, 3=user w/ user_request, 3=assistant w/ tool_calls, 3=tool result, 10=tool def, 21=model
	var body []byte
	body = pbwire.AppendString(body, 2, "You are Cascade. Use tools.")
	// user message source=1
	var userMsg []byte
	userMsg = pbwire.AppendEnum(userMsg, 2, 1)
	userMsg = pbwire.AppendString(userMsg, 3, "<user_request>\nlist dir\n</user_request>")
	body = pbwire.AppendMessage(body, 3, userMsg)
	// assistant with tool_calls source=2
	var tc []byte
	tc = pbwire.AppendString(tc, 1, "call_1")
	tc = pbwire.AppendString(tc, 2, "list_dir")
	tc = pbwire.AppendString(tc, 3, `{"DirectoryPath":"D:\\\\Devin-byok"}`)
	var asst []byte
	asst = pbwire.AppendEnum(asst, 2, 2)
	asst = pbwire.AppendString(asst, 3, "")
	asst = pbwire.AppendMessage(asst, 6, tc)
	body = pbwire.AppendMessage(body, 3, asst)
	// tool result source=4
	var toolRes []byte
	toolRes = pbwire.AppendEnum(toolRes, 2, 4)
	toolRes = pbwire.AppendString(toolRes, 3, "README.md\nconfig.yaml")
	toolRes = pbwire.AppendString(toolRes, 7, "call_1")
	body = pbwire.AppendMessage(body, 3, toolRes)
	// tool definition
	var toolDef []byte
	toolDef = pbwire.AppendString(toolDef, 1, "list_dir")
	toolDef = pbwire.AppendString(toolDef, 2, "List a directory")
	toolDef = pbwire.AppendString(toolDef, 3, `{"type":"object","properties":{"DirectoryPath":{"type":"string"}}}`)
	body = pbwire.AppendMessage(body, 10, toolDef)
	// do_not_call should be ignored
	var skip []byte
	skip = pbwire.AppendString(skip, 1, "do_not_call")
	body = pbwire.AppendMessage(body, 10, skip)
	body = pbwire.AppendString(body, 16, "conv-test-1")
	body = pbwire.AppendString(body, 21, "grok-4.5-high")

	parsed := parseGetChatMessageRequest(body)
	if parsed.UserText != "list dir" {
		t.Fatalf("UserText=%q", parsed.UserText)
	}
	if parsed.ModelUID != "grok-4.5-high" {
		t.Fatalf("ModelUID=%q", parsed.ModelUID)
	}
	if len(parsed.Tools) != 1 || parsed.Tools[0].Function.Name != "list_dir" {
		t.Fatalf("tools=%+v", parsed.Tools)
	}
	if len(parsed.Messages) != 3 {
		t.Fatalf("messages len=%d %+v", len(parsed.Messages), parsed.Messages)
	}
	if parsed.Messages[1].Role != "assistant" || len(parsed.Messages[1].ToolCalls) != 1 {
		t.Fatalf("assistant tool_calls=%+v", parsed.Messages[1])
	}
	if parsed.Messages[2].Role != "tool" || parsed.Messages[2].ToolCallID != "call_1" {
		t.Fatalf("tool msg=%+v", parsed.Messages[2])
	}

	msgs := buildOpenAIMessages(parsed)
	if msgs[0].Role != "system" {
		t.Fatalf("first role=%s", msgs[0].Role)
	}
	if msgs[len(msgs)-1].Role != "tool" {
		t.Fatalf("last role=%s", msgs[len(msgs)-1].Role)
	}
}

func TestSessionCacheKeepsToolsOnShortRequest(t *testing.T) {
	// 先完整 planner
	var full []byte
	full = pbwire.AppendString(full, 2, "You are Cascade full prompt")
	var toolDef []byte
	toolDef = pbwire.AppendString(toolDef, 1, "read_file")
	toolDef = pbwire.AppendString(toolDef, 3, `{"type":"object"}`)
	full = pbwire.AppendMessage(full, 10, toolDef)
	full = pbwire.AppendString(full, 16, "conv-cache-1")
	p1 := parseGetChatMessageRequest(full)
	if len(p1.Tools) != 1 {
		t.Fatalf("p1 tools=%d", len(p1.Tools))
	}
	// 短请求只有 do_not_call
	var short []byte
	short = pbwire.AppendString(short, 2, "short")
	var skip []byte
	skip = pbwire.AppendString(skip, 1, "do_not_call")
	short = pbwire.AppendMessage(short, 10, skip)
	short = pbwire.AppendString(short, 16, "conv-cache-1")
	p2 := parseGetChatMessageRequest(short)
	if len(p2.Tools) != 1 || p2.Tools[0].Function.Name != "read_file" {
		t.Fatalf("cached tools lost: %+v", p2.Tools)
	}
	if p2.SystemPrompt == "" {
		// short has its own system "short"; that is fine. Tools must remain.
	}
}

func TestMergeToolCallDeltas(t *testing.T) {
	i0 := 0
	d1 := openai.ToolCall{Index: &i0, ID: "c1", Type: "function"}
	d1.Function.Name = "read_file"
	d1.Function.Arguments = `{"file`
	d2 := openai.ToolCall{Index: &i0}
	d2.Function.Arguments = `_path":"a.go"}`
	out := openai.MergeToolCallDeltas(nil, []openai.ToolCall{d1})
	out = openai.MergeToolCallDeltas(out, []openai.ToolCall{d2})
	if len(out) != 1 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].Function.Name != "read_file" {
		t.Fatalf("name=%s", out[0].Function.Name)
	}
	if out[0].Function.Arguments != `{"file_path":"a.go"}` {
		t.Fatalf("args=%s", out[0].Function.Arguments)
	}
}

func TestBuildGetChatMessageToolFinalStopReason(t *testing.T) {
	b := buildGetChatMessageToolFinal("mid", []openaiToolCallView{{
		ID: "c1", Name: "list_dir", Arguments: `{"DirectoryPath":"D:\\x"}`,
	}})
	fields := pbwire.ParseFields(b)
	foundStop := false
	foundTool := false
	for _, f := range fields {
		if f.Number == 5 && f.Wire == 0 && int(f.Varint) == stopReasonFunctionCall {
			foundStop = true
		}
		if f.Number == 6 && f.Wire == 2 {
			foundTool = true
			sub := pbwire.ParseFields(f.Bytes)
			name := ""
			for _, sf := range sub {
				if sf.Number == 2 && sf.Wire == 2 {
					name = string(sf.Bytes)
				}
			}
			if name != "list_dir" {
				t.Fatalf("tool name=%q", name)
			}
		}
	}
	if !foundStop || !foundTool {
		t.Fatalf("stop=%v tool=%v", foundStop, foundTool)
	}
}

func TestEmitToolCallsSmartRunCommandSkipsIncompleteDelta(t *testing.T) {
	views := []openaiToolCallView{
		{ID: "call_rc_1", Name: "run_command", Arguments: `{"CommandLine":"dir"}`},
	}
	var frames [][]byte
	writeFrame := func(b []byte) bool {
		frames = append(frames, b)
		return true
	}
	writeDelta := func(text, thinking string, inProgress bool) bool {
		return true
	}

	emitToolCallsSmart(writeFrame, writeDelta, "mid-test", views)

	if len(frames) == 0 {
		t.Fatalf("expected frames written")
	}

	// 最终一帧必须为带 stopReasonFunctionCall 的 final 帧
	lastFrame := frames[len(frames)-1]
	fields := pbwire.ParseFields(lastFrame)
	foundStopFunc := false
	for _, f := range fields {
		if f.Number == 5 && f.Wire == 0 && int(f.Varint) == stopReasonFunctionCall {
			foundStopFunc = true
		}
	}
	if !foundStopFunc {
		t.Fatalf("last frame must be stopReasonFunctionCall")
	}

	// 检查是否有带 stopReasonIncomplete 且带 tool_calls (field 6) 的预览帧
	for _, frame := range frames[:len(frames)-1] {
		fFields := pbwire.ParseFields(frame)
		hasToolField := false
		hasIncomplete := false
		for _, f := range fFields {
			if f.Number == 6 && f.Wire == 2 {
				hasToolField = true
			}
			if f.Number == 5 && f.Wire == 0 && int(f.Varint) == stopReasonIncomplete {
				hasIncomplete = true
			}
		}
		if hasToolField && hasIncomplete {
			t.Fatalf("run_command should not emit incomplete tool preview frame")
		}
	}
}

func TestTitleGenerationDetection(t *testing.T) {
	plain := []byte("Generate a title for this conversation concisely")
	parsed := parseGetChatMessageRequest(plain)
	if !isTitleGenerationChat(parsed, "generate title", plain) {
		t.Fatal("expected title generation detection")
	}
}


