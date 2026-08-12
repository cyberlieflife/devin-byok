package localapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"devin-byok/internal/platform"
)

// RuntimeMetrics 运行时监控指标。
// 计数/缓存命中等可持久化；日志仅内存，停服清空且不落盘。
type RuntimeMetrics struct {
	mu sync.RWMutex

	StartedAt time.Time

	ReqTotal  int64
	ReqOK     int64
	ReqFail   int64
	ToolCalls int64

	CacheHit  int64
	CacheMiss int64

	TokensIn  int64
	TokensOut int64

	PromptTokens     int64
	CachedTokens     int64
	CacheWriteTokens int64
	// 功能维度统计（B8）
	DeepWikiOK   int64
	DeepWikiFail int64
	CodeMapOK    int64
	CodeMapFail  int64
	CodeMapFast  int64
	CodeMapSmart int64
	CommitOK     int64
	CommitFail   int64
	FastContextOK   int64
	FastContextFail int64
	FeatureModel map[string]int64

	ModelCounts map[string]int64

	logs []LogLine
}

type LogLine struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// metricsPersist 持久化形状：不含 logs。
type metricsPersist struct {
	DeepWikiOK   int64            `json:"deepwiki_ok"`
	DeepWikiFail int64            `json:"deepwiki_fail"`
	CodeMapOK    int64            `json:"codemap_ok"`
	CodeMapFail  int64            `json:"codemap_fail"`
	CodeMapFast  int64            `json:"codemap_fast"`
	CodeMapSmart int64            `json:"codemap_smart"`
	CommitOK     int64            `json:"commit_ok"`
	CommitFail   int64            `json:"commit_fail"`
	FastContextOK   int64         `json:"fast_context_ok"`
	FastContextFail int64         `json:"fast_context_fail"`
	FeatureModel map[string]int64 `json:"feature_model"`

	ReqTotal         int64            `json:"req_total"`
	ReqOK            int64            `json:"req_ok"`
	ReqFail          int64            `json:"req_fail"`
	ToolCalls        int64            `json:"tool_calls"`
	CacheHit         int64            `json:"cache_hit"`
	CacheMiss        int64            `json:"cache_miss"`
	TokensIn         int64            `json:"tokens_in"`
	TokensOut        int64            `json:"tokens_out"`
	PromptTokens     int64            `json:"prompt_tokens"`
	CachedTokens     int64            `json:"cached_tokens"`
	CacheWriteTokens int64            `json:"cache_write_tokens"`
	ModelCounts      map[string]int64 `json:"model_counts"`
	SavedAt          string           `json:"saved_at"`
}

var runtimeStats = &RuntimeMetrics{
	StartedAt:   time.Now(),
	ModelCounts: map[string]int64{},
	logs:        make([]LogLine, 0, 256),
		FeatureModel: map[string]int64{},
}

func metricsPath() string {
	dir := platform.DataDir()
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "metrics.json")
}

// MetricsLoad 启动时加载计数（不含日志）。
func MetricsLoad() {
	b, err := os.ReadFile(metricsPath())
	if err != nil {
		return
	}
	var p metricsPersist
	if json.Unmarshal(b, &p) != nil {
		return
	}
	runtimeStats.mu.Lock()
	defer runtimeStats.mu.Unlock()
	runtimeStats.ReqTotal = p.ReqTotal
	runtimeStats.ReqOK = p.ReqOK
	runtimeStats.ReqFail = p.ReqFail
	runtimeStats.ToolCalls = p.ToolCalls
	runtimeStats.CacheHit = p.CacheHit
	runtimeStats.CacheMiss = p.CacheMiss
	runtimeStats.TokensIn = p.TokensIn
	runtimeStats.TokensOut = p.TokensOut
	runtimeStats.PromptTokens = p.PromptTokens
	runtimeStats.CachedTokens = p.CachedTokens
	runtimeStats.CacheWriteTokens = p.CacheWriteTokens
	runtimeStats.DeepWikiOK = p.DeepWikiOK
	runtimeStats.DeepWikiFail = p.DeepWikiFail
	runtimeStats.CodeMapOK = p.CodeMapOK
	runtimeStats.CodeMapFail = p.CodeMapFail
	runtimeStats.CodeMapFast = p.CodeMapFast
	runtimeStats.CodeMapSmart = p.CodeMapSmart
	runtimeStats.CommitOK = p.CommitOK
	runtimeStats.CommitFail = p.CommitFail
	runtimeStats.FastContextOK = p.FastContextOK
	runtimeStats.FastContextFail = p.FastContextFail
	if p.FeatureModel != nil {
		runtimeStats.FeatureModel = p.FeatureModel
	}
	if p.ModelCounts != nil {
		runtimeStats.ModelCounts = p.ModelCounts
	}
	// 日志不恢复
	runtimeStats.logs = runtimeStats.logs[:0]
	runtimeStats.StartedAt = time.Now()
}

// MetricsSave 持久化计数；日志故意不写盘。
func MetricsSave() {
	runtimeStats.mu.RLock()
	p := metricsPersist{
		ReqTotal: runtimeStats.ReqTotal, ReqOK: runtimeStats.ReqOK, ReqFail: runtimeStats.ReqFail,
		ToolCalls: runtimeStats.ToolCalls, CacheHit: runtimeStats.CacheHit, CacheMiss: runtimeStats.CacheMiss,
		TokensIn: runtimeStats.TokensIn, TokensOut: runtimeStats.TokensOut,
		PromptTokens: runtimeStats.PromptTokens,
		DeepWikiOK: runtimeStats.DeepWikiOK, DeepWikiFail: runtimeStats.DeepWikiFail,
		CodeMapOK: runtimeStats.CodeMapOK, CodeMapFail: runtimeStats.CodeMapFail,
		CodeMapFast: runtimeStats.CodeMapFast, CodeMapSmart: runtimeStats.CodeMapSmart,
		CommitOK: runtimeStats.CommitOK, CommitFail: runtimeStats.CommitFail,
		FastContextOK: runtimeStats.FastContextOK, FastContextFail: runtimeStats.FastContextFail,
		FeatureModel: runtimeStats.FeatureModel, CachedTokens: runtimeStats.CachedTokens,
		CacheWriteTokens: runtimeStats.CacheWriteTokens,
		ModelCounts:      map[string]int64{},
		SavedAt:          time.Now().Format(time.RFC3339),
	}
	for k, v := range runtimeStats.ModelCounts {
		p.ModelCounts[k] = v
	}
	runtimeStats.mu.RUnlock()
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(metricsPath(), b, 0o644)
}

// MetricsClearLogs 清空内存日志（停服时调用）。
func MetricsClearLogs() {
	runtimeStats.mu.Lock()
	defer runtimeStats.mu.Unlock()
	runtimeStats.logs = runtimeStats.logs[:0]
}

// MetricsStartPeriodicSave 定时落盘计数。
func MetricsStartPeriodicSave(interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			MetricsSave()
		}
	}()
}

func metricsSnapshot() map[string]any {
	runtimeStats.mu.RLock()
	defer runtimeStats.mu.RUnlock()
	models := make([]map[string]any, 0, len(runtimeStats.ModelCounts))
	type kv struct {
		k string
		v int64
	}
	arr := make([]kv, 0, len(runtimeStats.ModelCounts))
	for k, v := range runtimeStats.ModelCounts {
		arr = append(arr, kv{k, v})
	}
	for i := 0; i < len(arr); i++ {
		for j := i + 1; j < len(arr); j++ {
			if arr[j].v > arr[i].v {
				arr[i], arr[j] = arr[j], arr[i]
			}
		}
	}
	for _, it := range arr {
		models = append(models, map[string]any{"model": it.k, "count": it.v})
	}
	hitRate := 0.0
	if runtimeStats.PromptTokens > 0 {
		hitRate = float64(runtimeStats.CachedTokens) / float64(runtimeStats.PromptTokens)
		if hitRate > 1 {
			hitRate = 1
		}
	} else {
		totalCache := runtimeStats.CacheHit + runtimeStats.CacheMiss
		if totalCache > 0 {
			hitRate = float64(runtimeStats.CacheHit) / float64(totalCache)
		}
	}
	featureRank := featureModelRankLocked()
	// 保持时间正序（旧在上、新在下），GUI 按命令行方式向下追加
	logs := append([]LogLine(nil), runtimeStats.logs...)
	return map[string]any{
		"started_at":         runtimeStats.StartedAt.Format(time.RFC3339),
		"uptime_sec":         int(time.Since(runtimeStats.StartedAt).Seconds()),
		"req_total":          runtimeStats.ReqTotal,
		"req_ok":             runtimeStats.ReqOK,
		"req_fail":           runtimeStats.ReqFail,
		"tool_calls":         runtimeStats.ToolCalls,
		"cache_hit":          runtimeStats.CacheHit,
		"cache_miss":         runtimeStats.CacheMiss,
		"cache_hit_rate":     hitRate,
		"tokens_in":          runtimeStats.TokensIn,
		"tokens_out":         runtimeStats.TokensOut,
		"prompt_tokens":      runtimeStats.PromptTokens,
		"cached_tokens":      runtimeStats.CachedTokens,
		"cache_write_tokens": runtimeStats.CacheWriteTokens,
		"model_rank":         models,
		"deepwiki_ok":        runtimeStats.DeepWikiOK,
		"deepwiki_fail":      runtimeStats.DeepWikiFail,
		"codemap_ok":         runtimeStats.CodeMapOK,
		"codemap_fail":       runtimeStats.CodeMapFail,
		"codemap_fast":       runtimeStats.CodeMapFast,
		"codemap_smart":      runtimeStats.CodeMapSmart,
		"commit_ok":          runtimeStats.CommitOK,
		"commit_fail":        runtimeStats.CommitFail,
		"fast_context_ok":    runtimeStats.FastContextOK,
		"fast_context_fail":  runtimeStats.FastContextFail,
		"feature_model_rank": featureRank,
		"logs":               logs,
	}
}

func metricsAddLog(level, msg string) {
	runtimeStats.mu.Lock()
	defer runtimeStats.mu.Unlock()
	runtimeStats.logs = append(runtimeStats.logs, LogLine{
		Time: time.Now().Format("15:04:05"), Level: level, Message: msg,
	})
	if len(runtimeStats.logs) > 300 {
		n := copy(runtimeStats.logs, runtimeStats.logs[len(runtimeStats.logs)-300:])
		runtimeStats.logs = runtimeStats.logs[:n]
	}
}

func metricsAddPromptUsage(promptTok, cachedTok, cacheWriteTok, completionTok int) {
	runtimeStats.mu.Lock()
	defer runtimeStats.mu.Unlock()
	runtimeStats.PromptTokens += int64(promptTok)
	runtimeStats.CachedTokens += int64(cachedTok)
	runtimeStats.CacheWriteTokens += int64(cacheWriteTok)
	if completionTok > 0 {
		runtimeStats.TokensOut += int64(completionTok)
	}
	if promptTok > 0 {
		runtimeStats.TokensIn += int64(promptTok)
	}
}

func metricsReqOK(model string, tokensIn, tokensOut, toolN int) {
	runtimeStats.mu.Lock()
	defer runtimeStats.mu.Unlock()
	runtimeStats.ReqTotal++
	runtimeStats.ReqOK++
	runtimeStats.TokensIn += int64(tokensIn)
	runtimeStats.TokensOut += int64(tokensOut)
	runtimeStats.ToolCalls += int64(toolN)
	if model == "" {
		model = "(unknown)"
	}
	runtimeStats.ModelCounts[model]++
}

func metricsReqFail(model string) {
	runtimeStats.mu.Lock()
	defer runtimeStats.mu.Unlock()
	runtimeStats.ReqTotal++
	runtimeStats.ReqFail++
	if model == "" {
		model = "(unknown)"
	}
	runtimeStats.ModelCounts[model]++
}

func metricsCache(hit bool) {
	runtimeStats.mu.Lock()
	defer runtimeStats.mu.Unlock()
	if hit {
		runtimeStats.CacheHit++
	} else {
		runtimeStats.CacheMiss++
	}
}

func estimateTokens(s string) int {
	n := 0
	for range s {
		n++
	}
	if n == 0 {
		return 0
	}
	t := n / 3
	if t < 1 {
		t = 1
	}
	return t
}


func featureModelRankLocked() []map[string]any {
	type kv struct {
		k string
		v int64
	}
	arr := make([]kv, 0, len(runtimeStats.FeatureModel))
	for k, v := range runtimeStats.FeatureModel {
		arr = append(arr, kv{k, v})
	}
	for i := 0; i < len(arr); i++ {
		for j := i + 1; j < len(arr); j++ {
			if arr[j].v > arr[i].v {
				arr[i], arr[j] = arr[j], arr[i]
			}
		}
	}
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		out = append(out, map[string]any{"model": it.k, "count": it.v})
	}
	return out
}

// metricsFeatureOK 记录 DeepWiki/CodeMap 成功。
func metricsFeatureOK(kind, model, mode string) {
	runtimeStats.mu.Lock()
	defer runtimeStats.mu.Unlock()
	if runtimeStats.FeatureModel == nil {
		runtimeStats.FeatureModel = map[string]int64{}
	}
	switch kind {
	case "deepwiki":
		runtimeStats.DeepWikiOK++
	case "codemap":
		runtimeStats.CodeMapOK++
		if mode == "fast" {
			runtimeStats.CodeMapFast++
		} else {
			runtimeStats.CodeMapSmart++
		}
	case "commit", "command":
		runtimeStats.CommitOK++
	case "fast_context", "fastcontext", "find_code_context":
		runtimeStats.FastContextOK++
	}
	if model != "" {
		runtimeStats.FeatureModel[kind+":"+model]++
	}
}

// metricsFeatureFail 记录 DeepWiki/CodeMap/FastContext 失败。
func metricsFeatureFail(kind, model string) {
	runtimeStats.mu.Lock()
	defer runtimeStats.mu.Unlock()
	switch kind {
	case "deepwiki":
		runtimeStats.DeepWikiFail++
	case "codemap":
		runtimeStats.CodeMapFail++
	case "commit", "command":
		runtimeStats.CommitFail++
	case "fast_context", "fastcontext", "find_code_context":
		runtimeStats.FastContextFail++
	}
	if model != "" && runtimeStats.FeatureModel != nil {
		runtimeStats.FeatureModel[kind+":fail:"+model]++
	}
}
