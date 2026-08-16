package localapi

import (
	"net/http/httptest"
	"testing"
)

// 双语字典完整性：每个 key 的 zh/en 均非空，且 zh 与 en 文案不同（排除漏翻）。
func TestUIMessagesBilingual(t *testing.T) {
	if len(uiMessages) == 0 {
		t.Fatal("uiMessages is empty")
	}
	for key, langs := range uiMessages {
		if langs["zh"] == "" {
			t.Errorf("key %q: zh translation is empty", key)
		}
		if langs["en"] == "" {
			t.Errorf("key %q: en translation is empty", key)
		}
		if langs["zh"] == langs["en"] {
			t.Errorf("key %q: zh equals en (%q), likely untranslated", key, langs["zh"])
		}
	}
}

// uiMsg：语言选择、%s 占位符、未知 key 回退。
func TestUIMsg(t *testing.T) {
	if got := uiMsg("en", "msg.savedAndReloaded"); got != "Saved and hot-reloaded" {
		t.Errorf("uiMsg(en) = %q", got)
	}
	if got := uiMsg("zh", "msg.savedAndReloaded"); got != "已保存并热重载" {
		t.Errorf("uiMsg(zh) = %q", got)
	}
	// 未知语言回退 zh
	if got := uiMsg("fr", "msg.savedAndReloaded"); got != "已保存并热重载" {
		t.Errorf("uiMsg(fr) = %q, want zh fallback", got)
	}
	// 占位符
	if got := uiMsg("en", "msg.saveAccountFailed", "boom"); got != "Failed to save local account: boom" {
		t.Errorf("uiMsg(en, arg) = %q", got)
	}
	// 未知 key 回退 key 本身
	if got := uiMsg("en", "msg.unknown.key"); got != "msg.unknown.key" {
		t.Errorf("uiMsg(unknown) = %q, want key itself", got)
	}
	// 占位符数量与实参不匹配时返回原文（避免 %! 错乱）
	if got := uiMsg("en", "msg.saveAccountFailed"); got != "Failed to save local account: %s" {
		t.Errorf("uiMsg(en, no arg) = %q, want raw template", got)
	}
	if got := uiMsg("zh", "msg.saveAccountFailed", "a", "b"); got != "保存本地账户失败: %s" {
		t.Errorf("uiMsg(zh, 2 args) = %q, want raw template", got)
	}
	// 文案含裸 % 且不传参时原样返回（防御新增文案）
	if got := uiMsg("zh", "msg.accountNotCreated"); got != "尚未创建本地虚拟账户" {
		t.Errorf("uiMsg(zh, no-pct) = %q", got)
	}
}

// langFromRequest：X-Lang 优先，其次 Accept-Language，默认 zh。
func TestLangFromRequest(t *testing.T) {
	cases := []struct {
		name    string
		xLang   string
		accLang string
		want    string
	}{
		{"x-lang en", "en", "zh-CN", "en"},
		{"x-lang zh", "zh", "en-US", "zh"},
		{"x-lang uppercase", "EN", "zh-CN", "en"},
		{"x-lang padded", " EN ", "zh-CN", "en"},
		{"x-lang invalid falls back to accept", "fr", "zh-CN,zh;q=0.9", "zh"},
		{"accept zh", "", "zh-CN,zh;q=0.9", "zh"},
		{"accept zh uppercase", "", "ZH-cn", "zh"},
		{"accept en", "", "en-US,en;q=0.9", "en"},
		{"accept other", "", "ja-JP", "en"},
		{"no header", "", "", "zh"},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/api/status", nil)
		if c.xLang != "" {
			req.Header.Set("X-Lang", c.xLang)
		}
		if c.accLang != "" {
			req.Header.Set("Accept-Language", c.accLang)
		}
		if got := langFromRequest(req); got != c.want {
			t.Errorf("%s: langFromRequest = %q, want %q", c.name, got, c.want)
		}
	}
	if got := langFromRequest(nil); got != "zh" {
		t.Errorf("langFromRequest(nil) = %q, want zh", got)
	}
}
