package promptstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"devin-byok/internal/paths"
)

// Mode 提示词注入方式。
type Mode string

const (
	ModeAppend  Mode = "append"
	ModePrepend Mode = "prepend"
	ModeReplace Mode = "replace"
)

// BuiltinFileToolsPrompt 固定注入：优先 write/edit 工具，而不是 shell 改文件。
const BuiltinFileToolsPrompt = `## File tools (required)
When creating or changing project files, use Cascade file tools — not the shell.

- **Create** new files with the write/create tool (write_to_file, write_file, create_file, or equivalent). Do not create files via shell redirection (echo/cat/printf > file, Out-File, Set-Content) when a write tool is available.
- **Edit** existing files with the edit/patch tool (edit_file, search_replace, apply_patch, or equivalent). Prefer minimal, targeted edits over rewriting the whole file when a small change is enough.
- **Prefer tools over commands** for reading, listing, searching, creating, and editing workspace files. Use run_command / terminal only for builds, tests, git, package managers, system info, or when no suitable file tool exists.
- Resolve paths against the workspace roots you were given. Do not overwrite unrelated files. After a write/edit, briefly confirm the path and what changed.
`

// Prompt 一条可开关的系统提示词。
type Prompt struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Enabled  bool     `json:"enabled"`
	Mode     Mode     `json:"mode"`             // append|prepend|replace
	Scope    string   `json:"scope,omitempty"`  // global|model|route|task
	Routes   []string `json:"routes,omitempty"` // chat|fast_context|deepwiki|codemap
	Models   []string `json:"models,omitempty"` // model id or family uid
	Tasks    []string `json:"tasks,omitempty"`  // coding|debug|research|review|explain|general
	Priority int      `json:"priority,omitempty"`
	Builtin  bool     `json:"builtin,omitempty"`
}

type fileShape struct {
	Prompts []Prompt `json:"prompts"`
}

var (
	mu    sync.RWMutex
	cache []Prompt
)

// Path 返回 system-prompts.json 路径。
func Path() string {
	return filepath.Join(paths.Dir(), "system-prompts.json")
}

// Load 从磁盘加载（带内存缓存刷新）。
func Load() ([]Prompt, error) {
	mu.Lock()
	defer mu.Unlock()
	return loadLocked()
}

func loadLocked() ([]Prompt, error) {
	p := Path()
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			cache = nil
			return nil, nil
		}
		return nil, err
	}
	var fs fileShape
	if err := json.Unmarshal(b, &fs); err != nil {
		return nil, err
	}
	cache = fs.Prompts
	if cache == nil {
		cache = []Prompt{}
	}
	out := make([]Prompt, len(cache))
	copy(out, cache)
	return out, nil
}

// Save 全量保存。
func Save(list []Prompt) error {
	mu.Lock()
	defer mu.Unlock()
	if list == nil {
		list = []Prompt{}
	}
	for i := range list {
		if list[i].ID == "" {
			list[i].ID = newID()
		}
		if list[i].Mode == "" {
			list[i].Mode = ModeAppend
		}
		list[i].Title = strings.TrimSpace(list[i].Title)
		list[i].Body = strings.TrimSpace(list[i].Body)
	}
	if _, err := paths.EnsureDir(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(fileShape{Prompts: list}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(Path(), raw, 0o644); err != nil {
		return err
	}
	cache = make([]Prompt, len(list))
	copy(cache, list)
	return nil
}

// Upsert 新增或按 id 更新。
func Upsert(p Prompt) ([]Prompt, error) {
	list, err := Load()
	if err != nil {
		return nil, err
	}
	if p.ID == "" {
		p.ID = newID()
		list = append(list, p)
	} else {
		found := false
		for i := range list {
			if list[i].ID == p.ID {
				list[i] = p
				found = true
				break
			}
		}
		if !found {
			list = append(list, p)
		}
	}
	if err := Save(list); err != nil {
		return nil, err
	}
	return list, nil
}

// Delete 按 id 删除。
func Delete(id string) ([]Prompt, error) {
	list, err := Load()
	if err != nil {
		return nil, err
	}
	out := list[:0]
	for _, p := range list {
		if p.ID != id {
			out = append(out, p)
		}
	}
	if err := Save(out); err != nil {
		return nil, err
	}
	return out, nil
}

func newID() string {
	return "sp_" + time.Now().Format("20060102150405") + "_" + random3()
}

func random3() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	n := time.Now().UnixNano()
	b := make([]byte, 4)
	for i := range b {
		b[i] = letters[(n+int64(i*17))%int64(len(letters))]
		n /= 7
	}
	return string(b)
}
