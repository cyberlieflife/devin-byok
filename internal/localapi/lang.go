package localapi

import (
	"fmt"
	"net/http"
	"strings"
)

// 前端界面 message 双语字典（issue#5：英文界面支持）。
// key → {zh, en}；语言缺失时回退 zh，两者都缺失时回退 key 本身。
var uiMessages = map[string]map[string]string{
	"msg.savedAndReloaded": {
		"zh": "已保存并热重载",
		"en": "Saved and hot-reloaded",
	},
	"msg.accountNotCreated": {
		"zh": "尚未创建本地虚拟账户",
		"en": "Local virtual account not created yet",
	},
	"msg.saveAccountFailed": {
		"zh": "保存本地账户失败: %s",
		"en": "Failed to save local account: %s",
	},
	"msg.importFailed": {
		"zh": "账户已创建，但导入 Devin 失败: %s",
		"en": "Account created, but importing into Devin failed: %s",
	},
	"msg.importedReloadFailed": {
		"zh": "已导入，但配置重载失败: %s",
		"en": "Imported, but config reload failed: %s",
	},
	"msg.accountImportedRestart": {
		"zh": "本地虚拟账户已导入，请完全退出并重启 Devin",
		"en": "Local virtual account imported — fully quit and restart Devin",
	},
	"msg.createAccountFailed": {
		"zh": "创建本地账户失败: %s",
		"en": "Failed to create local account: %s",
	},
	"msg.applyFailed": {
		"zh": "apply 失败: %s",
		"en": "apply failed: %s",
	},
	"msg.enabledReloadFailed": {
		"zh": "已启用，但配置重载失败: %s",
		"en": "Enabled, but config reload failed: %s",
	},
	"msg.serviceEnabledRestart": {
		"zh": "本地服务和虚拟账户已启用，请重启 Devin 生效",
		"en": "Local service and virtual account enabled — restart Devin to take effect",
	},
	"msg.wrapperInstalled": {
		"zh": "wrapper 已安装",
		"en": "Wrapper installed",
	},
	"msg.wrapperUninstalled": {
		"zh": "wrapper 已卸载还原",
		"en": "Wrapper uninstalled and restored",
	},
	"msg.serviceStopped": {
		"zh": "BYOK 已停止，Devin 原配置已恢复；管理窗口仍可使用",
		"en": "BYOK stopped and Devin's original config restored; the management window is still usable",
	},
}

// uiMsg 按语言取 message 并用 args 填充 %s 占位符；未知 key 回退 key 本身。
// 占位符用字面替换（strings.Replace）而非 fmt.Sprintf：只处理 %s 字面量，
// 文案中的裸 % 不会被解释为格式化动词（杜绝 %!(NOVERB) 一类错乱），
// 多余的实参被忽略、缺失的 %s 原样保留。
func uiMsg(lang, key string, args ...any) string {
	m, ok := uiMessages[key]
	if !ok {
		return key
	}
	s, ok := m[lang]
	if !ok || s == "" {
		s = m["zh"]
	}
	if len(args) > 0 {
		for _, a := range args {
			s = strings.Replace(s, "%s", fmt.Sprint(a), 1)
		}
	}
	return s
}

// langFromRequest 从 X-Lang 请求头或 Accept-Language 推导界面语言。
// 规则：zh 前缀 → zh，其余语言 → en；两者都缺失时默认 zh。
// 注意：默认 zh 仅服务于非浏览器客户端（CLI/curl 等无语言标识的调用方，保持历史行为）；
// 浏览器/WebView 前端恒带 X-Lang 头（见 app.js jget/jsend），不受该默认值影响。
func langFromRequest(r *http.Request) string {
	if r == nil {
		return "zh"
	}
	if l := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Lang"))); l == "zh" || l == "en" {
		return l
	}
	al := strings.ToLower(r.Header.Get("Accept-Language"))
	if strings.HasPrefix(al, "zh") {
		return "zh"
	}
	if strings.TrimSpace(al) != "" {
		return "en"
	}
	return "zh"
}
