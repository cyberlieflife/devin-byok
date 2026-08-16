/* i18n: 中/英文界面支持（issue#5）。
   语言解析优先级：localStorage('devin-byok.lang') 显式设置 → navigator.language（zh*→zh，其余→en）→ 默认 zh。
   字典缺失时 t() 回退返回 key 本身，避免界面出现空文本（同时方便排查漏项）。
   页头「中 / EN」按钮切换语言，选择写入 localStorage，切换后立即全量重刷界面。 */
(function () {
  'use strict';

  var LS_KEY = 'devin-byok.lang';
  var DEFAULT_LANG = 'zh';

  // 字典：扁平 key → {zh, en}。data-i18n / t() 引用，缺失时回退显示 key 本身。
  var DICTS = {
    zh: {
      'lang.label': '中',
      'lang.title': '切换界面语言（当前：中文）',
      'lang.toast.zh': '已切换为中文界面',
      'lang.toast.en': '已切换为英文界面',
      // ---- header / nav ----
      'sub.title': '本地模型接入控制台',
      'nav.settings': '设置',
      'nav.monitor': '监控',
      'nav.models': '模型',
      'nav.prompts': '提示词',
      // ---- states ----
      'state.checking': '检测中',
      'state.apiChecking': '管理端检测中',
      'state.extChecking': '检测中…',
      'state.versionLoading': '当前版本加载中…',
      'state.waitingCheck': '等待检查…',
      'state.noData': '暂无数据',
      'state.notCreated': '未创建',
      // ---- service console ----
      'service.title.checking': '正在检查本地服务',
      'service.message.checking': '读取账户、Devin 和本地路由状态…',
      'btn.start': '启用并一键导入',
      'btn.stop': '停止并恢复',
      'btn.restartDevin': '重启 Devin',
      'fact.account': '本地账户',
      'fact.model': '默认模型',
      'fact.config': '配置状态',
      // ---- monitor ----
      'monitor.overview.title': '运行概览',
      'monitor.overview.sub': '请求、缓存、Token 与模型使用',
      'btn.refresh': '刷新',
      'metric.reqOk': '请求成功',
      'metric.reqFail': '请求失败',
      'metric.tokIn': 'Token 输入≈',
      'metric.tokOut': 'Token 输出≈',
      'metric.cacheRate': 'Prompt 缓存命中率',
      'monitor.modelRank': '模型使用排行',
      'monitor.featureCalls': '功能调用',
      'metric.deepwikiOk': 'DeepWiki 成功',
      'metric.deepwikiFail': 'DeepWiki 失败',
      'metric.codemapOk': 'CodeMap 成功',
      'metric.codemapFail': 'CodeMap 失败',
      'metric.codemapFast': 'CodeMap Fast',
      'metric.codemapSmart': 'CodeMap Smart',
      'metric.commitOk': 'Commit 成功',
      'metric.commitFail': 'Commit 失败',
      'metric.fastContextOk': 'Fast Context 成功',
      'metric.fastContextFail': 'Fast Context 失败',
      'monitor.featureModelRank': '功能模型排行',
      'monitor.logs': '实时日志',
      // ---- models page ----
      'models.title': '模型配置',
      'models.sub': '按模型卡片管理 · 自动生成思考强度变体',
      'models.addFamily': '+ 添加模型',
      'models.featureBind.title': '功能模型绑定',
      'models.featureBind.sub': 'DeepWiki、CodeMap、Commit、Fast Context 各自绑定下方已有模型变体；保存后热重载。若此处不可见请重启 GUI/服务。',
      'models.featureBind.deepwikiLabel': '模型（思考强度变体）',
      'models.featureBind.active': '生效：-',
      'models.featureBind.codemapFastLabel': '快速模式模型',
      'models.featureBind.codemapSmartLabel': '智能模式模型',
      'models.featureBind.commitTitle': 'Commit 消息',
      'models.featureBind.titleTitle': '会话标题',
      'models.featureBind.titleLabel': 'Title Generation (标题生成模型)',
      'models.featureBind.fastContextLabel': 'find_code_context / Instant Context（本地 fast_context_model）',
      'btn.saveBindings': '保存绑定',
      // ---- prompts page ----
      'prompts.title': '系统提示词',
      'prompts.sub': '自定义提示词与侧栏扩展同步；另有固定内置英文工具提示（write/edit 优先）始终注入。服务启动安装扩展，停止时禁用。',
      'btn.newPrompt': '+ 新建',
      'prompts.extStatus.title': '扩展状态',
      'prompts.extInstall': '安装/启用扩展',
      'prompts.extDisable': '禁用扩展',
      // ---- settings page ----
      'settings.title': '设置',
      'settings.sub': '供应商以外的运行策略（工具 / 流式 / 超时）',
      'settings.localAccount.title': '本地虚拟账户',
      'settings.localAccount.name': '名称',
      'settings.localAccount.email': '邮箱',
      'settings.localAccount.id': '账户 ID',
      'settings.localAccount.key': '本地密钥',
      'btn.importAccount': '一键创建并导入',
      'btn.refreshStatus': '刷新状态',
      'settings.data.title': '数据与恢复',
      'settings.data.sub': '聊天导出保留 Devin 原始数据库及 WAL 文件；停止服务会恢复启用前的 Devin 设置。',
      'btn.exportChats': '导出聊天记录',
      'btn.restoreDevin': '恢复 Devin 原配置',
      'settings.notes.title': '说明',
      'settings.notes.body': '供应商（Base URL / API Key / 上游模型名）已改为在 <b>模型 → Family 卡片</b> 内配置，与 cursor-byok 一致：多模型，每个模型自带供应商。',
      'settings.notes.timeout': '全局默认超时 timeout_sec（Family 未单独设置时使用）',
      'btn.savePolicy': '保存策略',
      'settings.policy.title': '功能策略',
      'settings.policy.toolsTimeout': 'tools.timeout_sec（有工具时的聊天总超时，秒；命令/写入假流式保活依赖此值）',
      'settings.policy.quality': '质量模式',
      'settings.policy.qualityEnabled': '启用可靠性提示词',
      'settings.policy.qualityMode': '模式',
      'settings.policy.maxRounds': '最大验证轮数',
      'settings.policy.stream': '流式输出',
      'settings.policy.cascadeTools': 'Cascade 工具',
      'settings.policy.pureLocal': '纯本地 pure_local（已强制：仅单机，混合模式已取消）',
      'settings.desktop.title': '桌面与自启',
      'settings.desktop.sub': '开机自启、启动最小化到托盘、最小化时托盘化。启用会自动导入，停止会自动恢复。',
      'settings.desktop.autostart': '开机自动启动服务（start + apply）',
      'settings.desktop.startMinimized': '启动 GUI 时最小化到托盘',
      'settings.desktop.minimizeToTray': '最小化窗口时隐藏到托盘',
      'btn.saveDesktop': '保存桌面设置',
      'btn.hideToTray': '立即隐藏到托盘',
      'settings.update.title': '更新',
      'settings.update.sub': '从 GitHub Releases 检查新版本。自动更新会下载 zip、校验 SHA256（若有）并替换 exe 后重启。',
      'settings.update.enabled': '启用在线检查',
      'settings.update.autoApply': '检查到更新后自动应用（谨慎）',
      'btn.checkUpdate': '检查更新',
      'btn.applyUpdate': '下载并更新',
      // ---- family modal ----
      'models.modal.addTitle': '添加模型',
      'models.modal.label': '显示名 Label',
      'models.modal.upstream': '上游模型 ID',
      'models.modal.uidHint': 'Family UID 将自动生成为：-',
      'models.modal.provider': '接口类型',
      'models.modal.providerOpenai': 'OpenAI 兼容（/v1/chat/completions）',
      'models.modal.providerResponses': 'OpenAI Responses（/v1/responses）',
      'models.modal.apiKey': 'API Key（编辑时留空表示保留原 Key）',
      'models.modal.maxTokens': 'max_tokens（输出）',
      'models.modal.thinkingOff': '不启用',
      'models.modal.thinkingBudget': 'thinking budget_tokens（小于 max_tokens）',
      'models.modal.thinkingParam': 'OpenAI thinking 参数',
      'models.modal.levels': '思考强度（可多选）',
      'btn.saveFamily': '保存模型',
      'btn.cancel': '取消',
      // ---- update modal ----
      'update.modal.title': '发现新版本',
      'btn.later': '稍后',
      'btn.updateNow': '立即更新'
    },
    en: {
      'lang.label': 'EN',
      'lang.title': 'Switch interface language (current: English)',
      'lang.toast.zh': 'Interface switched to Chinese',
      'lang.toast.en': 'Interface switched to English',
      // ---- header / nav ----
      'sub.title': 'Local Model Access Console',
      'nav.settings': 'Settings',
      'nav.monitor': 'Monitor',
      'nav.models': 'Models',
      'nav.prompts': 'Prompts',
      // ---- states ----
      'state.checking': 'Checking',
      'state.apiChecking': 'Checking management endpoint',
      'state.extChecking': 'Checking…',
      'state.versionLoading': 'Loading current version…',
      'state.waitingCheck': 'Waiting to check…',
      'state.noData': 'No data',
      'state.notCreated': 'Not created',
      // ---- service console ----
      'service.title.checking': 'Checking local service',
      'service.message.checking': 'Reading account, Devin and local route status…',
      'btn.start': 'Enable & Import',
      'btn.stop': 'Stop & Restore',
      'btn.restartDevin': 'Restart Devin',
      'fact.account': 'Local account',
      'fact.model': 'Default model',
      'fact.config': 'Config state',
      // ---- monitor ----
      'monitor.overview.title': 'Overview',
      'monitor.overview.sub': 'Requests, cache, tokens and model usage',
      'btn.refresh': 'Refresh',
      'metric.reqOk': 'Requests OK',
      'metric.reqFail': 'Requests failed',
      'metric.tokIn': 'Token in ≈',
      'metric.tokOut': 'Token out ≈',
      'metric.cacheRate': 'Prompt cache hit rate',
      'monitor.modelRank': 'Model usage ranking',
      'monitor.featureCalls': 'Feature calls',
      'metric.deepwikiOk': 'DeepWiki OK',
      'metric.deepwikiFail': 'DeepWiki failed',
      'metric.codemapOk': 'CodeMap OK',
      'metric.codemapFail': 'CodeMap failed',
      'metric.codemapFast': 'CodeMap Fast',
      'metric.codemapSmart': 'CodeMap Smart',
      'metric.commitOk': 'Commit OK',
      'metric.commitFail': 'Commit failed',
      'metric.fastContextOk': 'Fast Context OK',
      'metric.fastContextFail': 'Fast Context failed',
      'monitor.featureModelRank': 'Feature model ranking',
      'monitor.logs': 'Live logs',
      // ---- models page ----
      'models.title': 'Model Configuration',
      'models.sub': 'Manage by model card · auto-generates thinking-level variants',
      'models.addFamily': '+ Add Model',
      'models.featureBind.title': 'Feature Model Bindings',
      'models.featureBind.sub': 'DeepWiki, CodeMap, Commit and Fast Context each bind to the model variants below; hot-reloaded after saving. If this section is missing, restart the GUI/service.',
      'models.featureBind.deepwikiLabel': 'Model (thinking variant)',
      'models.featureBind.active': 'Active: -',
      'models.featureBind.codemapFastLabel': 'Fast mode model',
      'models.featureBind.codemapSmartLabel': 'Smart mode model',
      'models.featureBind.commitTitle': 'Commit Message',
      'models.featureBind.titleTitle': 'Session Title',
      'models.featureBind.titleLabel': 'Title Generation model',
      'models.featureBind.fastContextLabel': 'find_code_context / Instant Context (local fast_context_model)',
      'btn.saveBindings': 'Save Bindings',
      // ---- prompts page ----
      'prompts.title': 'System Prompts',
      'prompts.sub': 'Custom prompts sync with the sidebar extension; a fixed built-in English tool prompt (write/edit first) is always injected. The extension is installed on service start and disabled on stop.',
      'btn.newPrompt': '+ New',
      'prompts.extStatus.title': 'Extension Status',
      'prompts.extInstall': 'Install/Enable Extension',
      'prompts.extDisable': 'Disable Extension',
      // ---- settings page ----
      'settings.title': 'Settings',
      'settings.sub': 'Runtime policy outside providers (tools / streaming / timeout)',
      'settings.localAccount.title': 'Local Virtual Account',
      'settings.localAccount.name': 'Name',
      'settings.localAccount.email': 'Email',
      'settings.localAccount.id': 'Account ID',
      'settings.localAccount.key': 'Local key',
      'btn.importAccount': 'Create & Import',
      'btn.refreshStatus': 'Refresh Status',
      'settings.data.title': 'Data & Restore',
      'settings.data.sub': 'Chat export keeps the original Devin database and WAL files; stopping the service restores the Devin settings from before.',
      'btn.exportChats': 'Export Chat History',
      'btn.restoreDevin': 'Restore Devin Config',
      'settings.notes.title': 'Notes',
      'settings.notes.body': 'Providers (Base URL / API Key / upstream model) are configured in <b>Models → Family cards</b>, consistent with cursor-byok: multiple models, each with its own provider.',
      'settings.notes.timeout': 'Global default timeout timeout_sec (used when a Family has none set)',
      'btn.savePolicy': 'Save Policy',
      'settings.policy.title': 'Feature Policy',
      'settings.policy.toolsTimeout': 'tools.timeout_sec (total chat timeout in seconds when tools are enabled; command/write fake-stream keepalive depends on it)',
      'settings.policy.quality': 'Quality Mode',
      'settings.policy.qualityEnabled': 'Enable reliability prompts',
      'settings.policy.qualityMode': 'Mode',
      'settings.policy.maxRounds': 'Max verification rounds',
      'settings.policy.stream': 'Streaming output',
      'settings.policy.cascadeTools': 'Cascade tools',
      'settings.policy.pureLocal': 'Pure local pure_local (enforced: single machine only; mixed mode removed)',
      'settings.desktop.title': 'Desktop & Autostart',
      'settings.desktop.sub': 'Autostart on boot, start minimized to tray, minimize to tray. Enabling auto-imports; stopping auto-restores.',
      'settings.desktop.autostart': 'Auto-start service on boot (start + apply)',
      'settings.desktop.startMinimized': 'Start GUI minimized to tray',
      'settings.desktop.minimizeToTray': 'Minimize window to tray',
      'btn.saveDesktop': 'Save Desktop Settings',
      'btn.hideToTray': 'Hide to Tray Now',
      'settings.update.title': 'Update',
      'settings.update.sub': 'Check for new versions from GitHub Releases. Auto-update downloads a zip, verifies SHA256 (if present), replaces the exe and restarts.',
      'settings.update.enabled': 'Enable online check',
      'settings.update.autoApply': 'Auto-apply updates when found (careful)',
      'btn.checkUpdate': 'Check Update',
      'btn.applyUpdate': 'Download & Update',
      // ---- family modal ----
      'models.modal.addTitle': 'Add Model',
      'models.modal.label': 'Display Label',
      'models.modal.upstream': 'Upstream Model ID',
      'models.modal.uidHint': 'Family UID will be auto-generated as: -',
      'models.modal.provider': 'API Type',
      'models.modal.providerOpenai': 'OpenAI compatible (/v1/chat/completions)',
      'models.modal.providerResponses': 'OpenAI Responses (/v1/responses)',
      'models.modal.apiKey': 'API Key (leave blank when editing to keep the existing key)',
      'models.modal.maxTokens': 'max_tokens (output)',
      'models.modal.thinkingOff': 'Disabled',
      'models.modal.thinkingBudget': 'thinking budget_tokens (less than max_tokens)',
      'models.modal.thinkingParam': 'OpenAI thinking parameter',
      'models.modal.levels': 'Thinking levels (multi-select)',
      'btn.saveFamily': 'Save Model',
      'btn.cancel': 'Cancel',
      // ---- update modal ----
      'update.modal.title': 'New Version Available',
      'btn.later': 'Later',
      'btn.updateNow': 'Update Now'
    }
  };

  function currentLang() {
    var saved = null;
    try { saved = localStorage.getItem(LS_KEY); } catch (e) { /* localStorage 不可用时跟随浏览器 */ }
    if (saved === 'zh' || saved === 'en') return saved;
    var nav = String(navigator.language || navigator.userLanguage || '').toLowerCase();
    return nav.indexOf('zh') === 0 ? 'zh' : 'en';
  }

  function t(key, vars) {
    var lang = currentLang();
    var dict = DICTS[lang] || DICTS[DEFAULT_LANG] || {};
    var s = Object.prototype.hasOwnProperty.call(dict, key) ? dict[key] : key;
    if (vars) {
      s = s.replace(/\{(\w+)\}/g, function (m, k) {
        return Object.prototype.hasOwnProperty.call(vars, k) ? String(vars[k]) : m;
      });
    }
    return s;
  }

  function applyLang() {
    var lang = currentLang();
    document.documentElement.setAttribute('lang', lang === 'zh' ? 'zh-CN' : 'en');
    document.querySelectorAll('[data-i18n]').forEach(function (el) {
      el.textContent = t(el.getAttribute('data-i18n'));
    });
    document.querySelectorAll('[data-i18n-html]').forEach(function (el) {
      el.innerHTML = t(el.getAttribute('data-i18n-html'));
    });
    document.querySelectorAll('[data-i18n-placeholder]').forEach(function (el) {
      el.setAttribute('placeholder', t(el.getAttribute('data-i18n-placeholder')));
    });
    document.querySelectorAll('[data-i18n-title]').forEach(function (el) {
      el.setAttribute('title', t(el.getAttribute('data-i18n-title')));
    });
    var btn = document.getElementById('btnLang');
    if (btn) {
      btn.textContent = t('lang.label');
      btn.setAttribute('title', t('lang.title'));
    }
    document.dispatchEvent(new CustomEvent('i18n:changed', { detail: { lang: lang } }));
  }

  function setLang(lang) {
    if (lang !== 'zh' && lang !== 'en') return;
    try { localStorage.setItem(LS_KEY, lang); } catch (e) { /* 忽略写入失败 */ }
    applyLang();
    _toast(t('lang.toast.' + lang));
  }

  function toggleLang() {
    setLang(currentLang() === 'zh' ? 'en' : 'zh');
  }

  function _toast(msg) {
    if (typeof window.toast === 'function') window.toast(msg);
  }

  // 暴露为全局 I18N（app.js 在 body 末尾执行，此处 head 加载，无竞态）
  window.I18N = {
    currentLang: currentLang,
    t: t,
    applyLang: applyLang,
    setLang: setLang,
    toggleLang: toggleLang
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', applyLang);
  } else {
    applyLang();
  }
})();
