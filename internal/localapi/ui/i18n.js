/* i18n: 中/英文界面支持（issue#5）。
   语言解析优先级：localStorage('devin-byok.lang') 显式设置 → navigator.language（zh*→zh，其余→en）→ 默认 zh。
   字典缺失时 t() 回退返回 key 本身，避免界面出现空文本（同时方便排查漏项）。
   页头「中 / EN」按钮切换语言，选择写入 localStorage，切换后立即全量重刷界面。 */
(function () {
  'use strict';

  var LS_KEY = 'devin-byok.lang';
  var DEFAULT_LANG = 'zh';

  // 字典：扁平 key → {zh, en}。C2/C3 阶段随 data-i18n 与 t() 化逐步补全。
  var DICTS = {
    zh: {
      'lang.label': '中',
      'lang.title': '切换界面语言（当前：中文）',
      'lang.toast.zh': '已切换为中文界面',
      'lang.toast.en': '已切换为英文界面'
    },
    en: {
      'lang.label': 'EN',
      'lang.title': 'Switch interface language (current: English)',
      'lang.toast.zh': 'Interface switched to Chinese',
      'lang.toast.en': 'Interface switched to English'
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
