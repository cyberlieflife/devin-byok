package localapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"devin-byok/internal/config"
	"devin-byok/internal/upstream/openai"
)

type respCacheEntry struct {
	Text      string
	Thinking  string
	ToolCalls []openai.ToolCall
	ExpireAt  time.Time
}

type respCache struct {
	mu      sync.Mutex
	entries map[string]respCacheEntry
}

var chatRespCache = &respCache{entries: map[string]respCacheEntry{}}

func respCacheEnabled(cfg *config.File) bool {
	return cfg != nil && cfg.ResponseCacheEnabled()
}

func respCacheKey(model, effort, promptHash string, msgs []openai.ChatMessage, tools []openai.Tool) string {
	type finger struct {
		Model      string               `json:"m"`
		Effort     string               `json:"e"`
		PromptHash string               `json:"p"`
		Msgs       []openai.ChatMessage `json:"msgs"`
		Tools      []openai.Tool        `json:"tools"`
	}
	b, _ := json.Marshal(finger{Model: model, Effort: effort, PromptHash: promptHash, Msgs: msgs, Tools: tools})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func respCacheGet(cfg *config.File, key string) (respCacheEntry, bool) {
	if !respCacheEnabled(cfg) || key == "" {
		return respCacheEntry{}, false
	}
	chatRespCache.mu.Lock()
	defer chatRespCache.mu.Unlock()
	e, ok := chatRespCache.entries[key]
	if !ok {
		metricsCache(false)
		return respCacheEntry{}, false
	}
	if time.Now().After(e.ExpireAt) {
		delete(chatRespCache.entries, key)
		metricsCache(false)
		return respCacheEntry{}, false
	}
	metricsCache(true)
	return e, true
}

func respCachePut(cfg *config.File, key string, text, thinking string, tools []openai.ToolCall) {
	if !respCacheEnabled(cfg) || key == "" {
		return
	}
	// 有工具调用结果不缓存（下一轮依赖执行结果）
	if len(tools) > 0 {
		return
	}
	ttl := time.Duration(cfg.Cache.TTLSec) * time.Second
	if ttl <= 0 {
		ttl = 180 * time.Second
	}
	maxN := cfg.Cache.MaxEntries
	if maxN <= 0 {
		maxN = 128
	}
	chatRespCache.mu.Lock()
	defer chatRespCache.mu.Unlock()
	if len(chatRespCache.entries) >= maxN {
		// 简单清空一半
		n := 0
		for k := range chatRespCache.entries {
			delete(chatRespCache.entries, k)
			n++
			if n >= maxN/2 {
				break
			}
		}
	}
	chatRespCache.entries[key] = respCacheEntry{
		Text: text, Thinking: thinking,
		ExpireAt: time.Now().Add(ttl),
	}
}
