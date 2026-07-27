package localapi

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"devin-byok/internal/config"
	"devin-byok/internal/upstream/openai"
)

// ?????mode ???????deny ?????
var (
	toolReadOnly = map[string]bool{
		"ask_user_question":  true,
		"code_search":        true,
		"find_by_name":       true,
		"grep_search":        true,
		"list_dir":           true,
		"read_file":          true,
		"read_notebook":      true,
		"read_terminal":      true,
		"read_url_content":   true,
		"search_web":         true,
		"todo_list":          true,
		"trajectory_search":  true,
		"view_content_chunk": true,
	}
	toolWrite = map[string]bool{
		"browser_preview": true,
		"create_memory":   true,
		"edit":            true,
		"edit_notebook":   true,
		"multi_edit":      true,
		"write_to_file":   true,
	}
	toolCommand = map[string]bool{
		"command_status": true,
		"run_command":    true,
	}
	// ?? LS ????????????????????
	toolWorkspaceSearch = map[string]bool{
		"code_search":   true,
		"find_by_name":  true,
		"grep_search":   true,
	}
)

// filterToolsByPolicy ? tools.mode / allow / deny ???? tools?
func filterToolsByPolicy(tools []openai.Tool, cfg *config.File) ([]openai.Tool, string) {
	mode := cfg.ToolsMode()
	if !cfg.Features.EnableCascadeTools || mode == "off" {
		return nil, mode
	}
	allowExtra := map[string]bool{}
	for _, n := range cfg.Tools.Allow {
		allowExtra[strings.TrimSpace(n)] = true
	}
	deny := map[string]bool{}
	for _, n := range cfg.Tools.Deny {
		deny[strings.TrimSpace(n)] = true
	}

	out := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		name := strings.TrimSpace(t.Function.Name)
		if name == "" || name == "do_not_call" {
			continue
		}
		if deny[name] {
			continue
		}
		if allowExtra[name] || toolAllowedInMode(name, mode) {
			out = append(out, t)
		}
	}
	return out, mode
}

func toolAllowedInMode(name, mode string) bool {
	switch mode {
	case "readonly", "read_only", "read":
		return toolReadOnly[name]
	case "standard", "std", "default":
		return toolReadOnly[name] || toolWrite[name]
	case "full", "all":
		return true
	default:
		return toolReadOnly[name] || toolWrite[name]
	}
}

// validateToolCalls ??? FUNCTION_CALL ????? JSON / ??????
func validateToolCalls(calls []openai.ToolCall) (ok []openai.ToolCall, errText string) {
	return validateToolCallsEx(calls, nil)
}

// validateToolCallsEx ????????????????
func validateToolCallsEx(calls []openai.ToolCall, workspaceRoots []string) (ok []openai.ToolCall, errText string) {
	if len(calls) == 0 {
		return nil, ""
	}
	var problems []string
	for _, tc := range calls {
		name := strings.TrimSpace(tc.Function.Name)
		if name == "" {
			problems = append(problems, "????????")
			continue
		}
		args := strings.TrimSpace(tc.Function.Arguments)
		if args == "" {
			tc.Function.Arguments = "{}"
			args = "{}"
		}
		if !json.Valid([]byte(args)) {
			problems = append(problems, fmt.Sprintf("%s ?????? JSON???????", name))
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(args), &v); err != nil {
			problems = append(problems, fmt.Sprintf("%s ??????: %v", name, err))
			continue
		}
		switch v.(type) {
		case map[string]any, []any:
		default:
			problems = append(problems, fmt.Sprintf("%s ???? JSON ??", name))
			continue
		}

		// ???????? grep/code_search ?????????
		if toolWorkspaceSearch[name] && len(workspaceRoots) > 0 {
			if p := extractToolPath(name, v); p != "" && pathOutsideWorkspace(p, workspaceRoots) {
				problems = append(problems, fmt.Sprintf(
					"%s ?? %s ????????%s???????????????????????????????????????? run_command ??",
					name, p, strings.Join(workspaceRoots, ", ")))
				continue
			}
		}
		ok = append(ok, tc)
	}
	if len(ok) == 0 && len(problems) > 0 {
		return nil, "??????: " + strings.Join(problems, "?") + "?"
	}
	if len(problems) > 0 {
		return ok, "???????: " + strings.Join(problems, "?")
	}
	return ok, ""
}

func extractToolPath(toolName string, args any) string {
	m, ok := args.(map[string]any)
	if !ok {
		return ""
	}
	// ??????????
	keys := []string{"SearchPath", "search_path", "search_folder_absolute_uri", "SearchDirectory", "DirectoryPath", "directory_path", "file_path", "AbsolutePath", "path"}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	_ = toolName
	return ""
}

var winPathRe = regexp.MustCompile(`(?i)[a-z]:[\\/][^\x00-\x1f"<>|*?]{1,240}`)

// extractWorkspaceRoots ?????????????????
func extractWorkspaceRoots(plain []byte) []string {
	if len(plain) == 0 {
		return nil
	}
	text := string(plain)
	found := map[string]bool{}
	var roots []string
	add := func(p string) {
		p = normalizePath(p)
		if p == "" || found[p] {
			return
		}
		// ????
		low := strings.ToLower(p)
		if strings.Contains(low, "\\windows\\") || strings.Contains(low, "/windows/") {
			return
		}
		found[p] = true
		roots = append(roots, p)
	}

	// file:/// URI
	for _, m := range regexp.MustCompile(`(?i)file:///[^\s\x00"']+`).FindAllString(text, -1) {
		u, err := url.Parse(m)
		if err == nil {
			add(filepath.FromSlash(u.Path))
		}
	}
	// ????? workspace ?????
	for _, m := range regexp.MustCompile(`(?is)workspace[^\\x00]{0,80}?([a-zA-Z]:[\\\\/][^\\x00\\s"']{2,160})`).FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}
	// ??????
	for _, m := range winPathRe.FindAllString(text, -1) {
		// ?????/?????????????????????
		add(m)
	}

	// ???????????????????? 2~4 ??
	if len(roots) > 6 {
		roots = roots[:6]
	}
	return roots
}

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.TrimPrefix(p, "file:///")
	p = strings.TrimPrefix(p, "file://")
	p = strings.ReplaceAll(p, "/", "\\")
	// URL decode light
	if strings.Contains(p, "%") {
		if u, err := url.PathUnescape(p); err == nil {
			p = u
		}
	}
	p = filepath.Clean(p)
	return strings.ToLower(p)
}

func pathOutsideWorkspace(path string, roots []string) bool {
	if path == "" || len(roots) == 0 {
		return false
	}
	p := normalizePath(path)
	// ????????????
	if !regexp.MustCompile(`(?i)^[a-z]:\\`).MatchString(p) && !strings.HasPrefix(p, `\\`) {
		return false
	}
	for _, r := range roots {
		rr := normalizePath(r)
		if rr == "" {
			continue
		}
		if p == rr || strings.HasPrefix(p, strings.TrimRight(rr, `\`)+`\`) {
			return false
		}
	}
	return true
}

// humanizeChatError ???/?????????????
func humanizeChatError(err error) string {
	if err == nil {
		return "????"
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "not within any current workspace") || strings.Contains(low, "outside a currently open workspace"):
		return "???????????????????????? Devin ??????????????????tools.mode=full ???? run_command ???"
	case strings.Contains(low, "timeout") || strings.Contains(low, "deadline"):
		return "?????????????????????? config ? tools.timeout_sec / upstream.timeout_sec ????"
	case strings.Contains(low, "connection refused"):
		return "?????????????? base_url ????????"
	case strings.Contains(low, "401") || strings.Contains(low, "403") || strings.Contains(low, "auth"):
		return "?????????? api_key?"
	case strings.Contains(low, "tool") && strings.Contains(low, "support"):
		return "??????????? function calling/tools????????????? tools.mode ?? off?"
	default:
		return "BYOK ????: " + msg
	}
}

// needsWorkspaceHint ?????????????????
func needsWorkspaceHint(plain []byte, userText string, msgs []openai.ChatMessage) bool {
	blob := string(plain)
	low := strings.ToLower(blob + "\n" + userText)
	markers := []string{
		"no folder is open",
		"no workspace",
		"???????",
		"??????",
		"no open workspace",
		"open a folder",
		"?????",
	}
	for _, m := range markers {
		if strings.Contains(low, strings.ToLower(m)) {
			return true
		}
	}
	for _, m := range msgs {
		if m.Role == "assistant" && (strings.Contains(openai.TextContent(m.Content), "???????") || strings.Contains(openai.TextContent(m.Content), "??????")) {
			return true
		}
	}
	return false
}

const workspaceHintText = "???????????????????????????????????? ask_user_question ?????????????????"

// cascadeToolsHintText ???????????????????
const cascadeToolsHintText = "???????grep_search / code_search / find_by_name ??????????????????????????????????????? run_command?? rg/findstr???????? Devin ???????read_file/list_dir ????????????"

// injectWorkspaceHint ? messages ????? system ???????????
func injectWorkspaceHint(msgs []openai.ChatMessage) []openai.ChatMessage {
	return injectSystemNote(msgs, workspaceHintText, "?????????")
}

func injectCascadeToolsHint(msgs []openai.ChatMessage) []openai.ChatMessage {
	return injectSystemNote(msgs, cascadeToolsHintText, "??????????????")
}

func injectSystemNote(msgs []openai.ChatMessage, note, dedupeKey string) []openai.ChatMessage {
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(openai.TextContent(m.Content), dedupeKey) {
			return msgs
		}
	}
	out := make([]openai.ChatMessage, 0, len(msgs)+1)
	if len(msgs) > 0 && msgs[0].Role == "system" {
		out = append(out, msgs[0])
		out = append(out, openai.ChatMessage{Role: "system", Content: note})
		out = append(out, msgs[1:]...)
		return out
	}
	out = append(out, openai.ChatMessage{Role: "system", Content: note})
	out = append(out, msgs...)
	return out
}
