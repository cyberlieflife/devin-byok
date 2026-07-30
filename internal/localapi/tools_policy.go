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

// 工具分类：按 tools.mode / allow / deny 过滤。
var (
	toolReadOnly = map[string]bool{
		"ask_user_question":  true,
		"code_search":        true,
		"find_code_context": true,
		"fast_context":      true,
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
	// 语言服务侧对工作区外搜索更敏感的工具
	toolWorkspaceSearch = map[string]bool{
		"code_search":  true,
		"find_by_name": true,
		"grep_search":  true,
	}
)

// filterToolsByPolicy 按 tools.mode / allow / deny 过滤上游可见 tools。
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

// writeEditPathKeys Cascade/上游常见的写入路径字段名。
var writeEditPathKeys = []string{
	"TargetFile", "target_file", "targetFile",
	"FilePath", "file_path", "filePath", "filepath",
	"Path", "path", "file", "filename", "FileName",
}

func mapHasWritePath(m map[string]any) (string, bool) {
	for _, k := range writeEditPathKeys {
		if v, ok := m[k]; ok {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" {
				return k, true
			}
		}
	}
	// 大小写不敏感兜底
	for k, v := range m {
		lk := strings.ToLower(strings.TrimSpace(k))
		if lk == "targetfile" || lk == "target_file" || lk == "filepath" || lk == "file_path" || lk == "path" || lk == "file" || lk == "filename" {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" {
				return k, true
			}
		}
	}
	return "", false
}

func isWriteEditToolName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	keys := []string{"write_to_file", "write_file", "edit_file", "create_file", "search_replace", "apply_patch", "modify_file", "append_file"}
	for _, k := range keys {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

// validateToolCalls 校验 FUNCTION_CALL 参数是否为合法 JSON 等。
func validateToolCalls(calls []openai.ToolCall) (ok []openai.ToolCall, errText string) {
	return validateToolCallsEx(calls, nil)
}

// validateToolCallsEx 在 validateToolCalls 基础上增加工作区路径检查。
func validateToolCallsEx(calls []openai.ToolCall, workspaceRoots []string) (ok []openai.ToolCall, errText string) {
	if len(calls) == 0 {
		return nil, ""
	}
	var problems []string
	for _, tc := range calls {
		name := strings.TrimSpace(tc.Function.Name)
		if name == "" {
			problems = append(problems, "工具名称缺失")
			continue
		}
		args := strings.TrimSpace(tc.Function.Arguments)
		if args == "" {
			tc.Function.Arguments = "{}"
			args = "{}"
		}
		if !json.Valid([]byte(args)) {
			problems = append(problems, fmt.Sprintf("%s 参数不是合法 JSON，已跳过", name))
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(args), &v); err != nil {
			problems = append(problems, fmt.Sprintf("%s 参数解析失败: %v", name, err))
			continue
		}
		// write/edit 必须带目标路径，避免空 {} 或残缺参数导致「TargetFile 未传入」截停
		if isWriteEditToolName(name) {
			m, ok := v.(map[string]any)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s 参数必须是 JSON 对象且包含 TargetFile/path", name))
				continue
			}
			if _, ok := mapHasWritePath(m); !ok {
				problems = append(problems, fmt.Sprintf("%s 缺少 TargetFile/path，已跳过（防止残缺写入截停）", name))
				continue
			}
		}
		switch v.(type) {
		case map[string]any, []any:
		default:
			problems = append(problems, fmt.Sprintf("%s 参数须为 JSON 对象/数组", name))
			continue
		}

		// 搜索类工具：路径落在工作区外时拦截，避免语言服务报错
		if toolWorkspaceSearch[name] && len(workspaceRoots) > 0 {
			if p := extractToolPath(name, v); p != "" && pathOutsideWorkspace(p, workspaceRoots) {
				problems = append(problems, fmt.Sprintf(
					"%s 路径 %s 不在当前工作区（%s）内；请改用工作区内路径，或对区外目录使用 run_command",
					name, p, strings.Join(workspaceRoots, ", ")))
				continue
			}
		}
		ok = append(ok, tc)
	}
	if len(ok) == 0 && len(problems) > 0 {
		return nil, "工具调用全部无效: " + strings.Join(problems, "；") + "。"
	}
	if len(problems) > 0 {
		return ok, "部分工具调用已跳过: " + strings.Join(problems, "；")
	}
	return ok, ""
}

// isWorkspacePathError 判断报错是否属于“工具调用全部无效: ...不在当前工作区...内”
func isWorkspacePathError(errText string) bool {
	if !strings.HasPrefix(errText, "工具调用全部无效:") {
		return false
	}
	return strings.Contains(errText, "不在当前工作区") || strings.Contains(errText, "不在当前工作区（")
}

// buildWorkspacePathRetryPrompt 构建工作区外路径错误的重试反思提示词
func buildWorkspacePathRetryPrompt(errText string, retryCount int) string {
	return fmt.Sprintf(`【系统自动纠错通知 - 第 %d 次重试】
您刚刚发起的工具调用遭遇系统拦截并失败。

1. 犯错内容：%s
2. 犯错原因：检索类工具（如 grep_search、code_search、find_by_name 等）受 IDE 工作区限制，无法跨工作区检索外层或其它目录。
3. 修正方案（请严格遵照以下两条之一纠正）：
   - 方案 A（推荐）：如果需要检索工作区内代码，请将路径修改为当前工作区内的有效路径；
   - 方案 B：如果确实需要搜索/读取工作区外目录，请禁止使用搜索工具，改为使用 run_command 工具通过命令行（如 rg、dir、findstr 等）进行操作。

请根据以上提示，重新修正并给出有效的工具调用或回答。`, retryCount, errText)
}

func extractToolPath(toolName string, args any) string {
	m, ok := args.(map[string]any)
	if !ok {
		return ""
	}
	// 常见路径字段
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

// extractWorkspaceRoots 从请求正文猜测当前工作区根路径。
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
		// 过滤系统目录噪声
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
	// 靠近 workspace 字样的路径
	for _, m := range regexp.MustCompile(`(?is)workspace[^\x00]{0,80}?([a-zA-Z]:[\\/][^\x00\s"']{2,160})`).FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}
	// 正文中的 Windows 路径
	for _, m := range winPathRe.FindAllString(text, -1) {
		add(m)
	}

	// 限制数量，避免噪声根过多
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
	// 相对路径不判定
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

// humanizeChatError 将上游/网络错误转成可读中文。
func humanizeChatError(err error) string {
	if err == nil {
		return "未知错误"
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "not within any current workspace") || strings.Contains(low, "outside a currently open workspace"):
		return "搜索路径不在当前工作区内。请在 Devin 中打开对应文件夹，或将 tools.mode 设为 full 后用 run_command 处理区外路径。"
	case strings.Contains(low, "timeout") || strings.Contains(low, "deadline"):
		return "上游或工具调用超时。请在配置中增大 tools.timeout_sec（有工具时）或 upstream.timeout_sec（无工具时）。"
	case strings.Contains(low, "connection refused"):
		return "无法连接上游，请检查 base_url 与本地模型服务是否已启动。"
	case strings.Contains(low, "401") || strings.Contains(low, "403") || strings.Contains(low, "auth"):
		return "上游鉴权失败，请检查 api_key。"
	case strings.Contains(low, "tool") && strings.Contains(low, "support"):
		return "上游可能不支持 function calling/tools。可尝试将 tools.mode 设为 off。"
	default:
		return "BYOK 上游错误: " + msg
	}
}

// needsWorkspaceHint 检测是否需要注入“请打开工作区”提示。
func needsWorkspaceHint(plain []byte, userText string, msgs []openai.ChatMessage) bool {
	blob := string(plain)
	low := strings.ToLower(blob + "\n" + userText)
	markers := []string{
		"no folder is open",
		"no workspace",
		"没有打开文件夹",
		"未打开工作区",
		"no open workspace",
		"open a folder",
		"打开文件夹",
	}
	for _, m := range markers {
		if strings.Contains(low, strings.ToLower(m)) {
			return true
		}
	}
	for _, m := range msgs {
		if m.Role == "assistant" {
			c := openai.TextContent(m.Content)
			if strings.Contains(c, "没有打开文件夹") || strings.Contains(c, "未打开工作区") {
				return true
			}
		}
	}
	return false
}

const workspaceHintText = "当前可能没有打开工作区文件夹。请先在 Devin 中打开项目目录；若只需问答可不调用工作区工具，或使用 ask_user_question 向用户确认路径。"

// cascadeToolsHintText 提示模型优先用检索/读写工具，命令工具谨慎使用。
const cascadeToolsHintText = "优先使用 grep_search / code_search / find_by_name 检索，再用 read_file/list_dir 阅读；需要终端时再 run_command（如 rg/findstr）。避免在未确认路径时对工作区外盲目搜索。"

// injectWorkspaceHint 向 messages 注入工作区提示 system 消息。
func injectWorkspaceHint(msgs []openai.ChatMessage) []openai.ChatMessage {
	return injectSystemNote(msgs, workspaceHintText, "没有打开工作区")
}

func injectCascadeToolsHint(msgs []openai.ChatMessage) []openai.ChatMessage {
	return injectSystemNote(msgs, cascadeToolsHintText, "优先使用 grep_search")
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


const fastContextAgentHintText = `You are the Fast Context (Instant Context) retrieval agent for Cascade/Devin.
Your job is to quickly gather the most relevant code locations for the user's query.

Rules:
1. Prefer tools: code_search, grep_search, find_by_name, read_file, list_dir. Do not use shell to create/edit files.
2. Search broadly then read only the most relevant files/sections.
3. In your final answer, list concrete file paths and line ranges that matter, with a one-line reason each.
4. Keep the answer compact and structured. Example:
   - path/to/file.go:12-48 — handles X
   - path/to/other.ts:80-120 — defines Y
5. If nothing relevant is found, say so clearly and suggest 1-2 alternate search terms.
6. Do not invent file paths. Only cite paths you actually observed via tools.`

func injectFastContextAgentHint(msgs []openai.ChatMessage) []openai.ChatMessage {
	return injectSystemNote(msgs, fastContextAgentHintText, "Fast Context (Instant Context) retrieval agent")
}
