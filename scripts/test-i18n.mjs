#!/usr/bin/env node
/**
 * 前端 i18n 运行时核心测试（issue#5 配套）：
 *   在 Node 中 mock window/document/localStorage/navigator 后加载 i18n.js，
 *   验证 currentLang 语言解析矩阵与 setLang→applyLang→i18n:changed 链路。
 * 无第三方依赖，node >= 14 直接运行：node scripts/test-i18n.mjs
 */
'use strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const ROOT = path.join(path.dirname(fileURLToPath(import.meta.url)), '..');
const i18nSrc = fs.readFileSync(path.join(ROOT, 'internal', 'localapi', 'ui', 'i18n.js'), 'utf8');

class CustomEventMock {
  constructor(type, opts) { this.type = type; this.detail = (opts || {}).detail || {}; }
}

function makeStorage() {
  const map = new Map();
  return {
    getItem(k) { return map.has(k) ? map.get(k) : null; },
    setItem(k, v) { map.set(k, String(v)); },
    removeItem(k) { map.delete(k); },
    clear() { map.clear(); },
  };
}

function makeElement(attrs) {
  return {
    attrs: Object.assign({}, attrs),
    textContent: '',
    innerHTML: '',
    setAttribute(k, v) { this.attrs[k] = String(v); },
    getAttribute(k) { return k in this.attrs ? this.attrs[k] : null; },
  };
}

function makeDocument(elements) {
  const listeners = {};
  return {
    documentElement: {
      lang: 'zh-CN',
      setAttribute(k, v) { if (k === 'lang') this.lang = v; },
    },
    readyState: 'complete',
    addEventListener(ev, fn) { (listeners[ev] = listeners[ev] || []).push(fn); },
    dispatchEvent(ev) { (listeners[ev.type] || []).forEach(fn => fn(ev)); },
    querySelectorAll(sel) { return (elements && elements[sel]) || []; },
    getElementById() { return null; },
    _listeners: listeners,
  };
}

function boot(elements) {
  const sandbox = { navigator: { language: 'en-US' }, localStorage: makeStorage(), CustomEvent: CustomEventMock };
  sandbox.document = makeDocument(elements);
  sandbox.window = sandbox; // i18n.js 以 window.I18N 暴露 API，window.toast 可选
  let toastCalls = [];
  sandbox.toast = (m) => toastCalls.push(m);
  vm.createContext(sandbox);
  vm.runInContext(i18nSrc, sandbox, { filename: 'i18n.js' });
  return { I18N: sandbox.I18N, sandbox, get toastCalls() { return toastCalls; } };
}

let failures = 0;
function assertEq(actual, expected, label) {
  if (actual !== expected) {
    failures++;
    console.error(`  ✗ ${label}: got ${JSON.stringify(actual)}, want ${JSON.stringify(expected)}`);
  } else {
    console.log(`  ✓ ${label}`);
  }
}

// —— currentLang 解析矩阵 ——
console.log('currentLang 解析矩阵:');
{
  const b = boot();
  const cases = [
    { ls: null, nav: 'zh-CN', want: 'zh' },
    { ls: null, nav: 'zh-TW', want: 'zh' },
    { ls: null, nav: 'en-US', want: 'en' },
    { ls: null, nav: 'ja-JP', want: 'en' },
    { ls: 'zh', nav: 'en-US', want: 'zh' },   // localStorage 优先
    { ls: 'en', nav: 'zh-CN', want: 'en' },   // localStorage 优先
    { ls: 'fr', nav: 'zh-CN', want: 'zh' },   // 非法值回退浏览器语言
    { ls: '', nav: 'en-GB', want: 'en' },     // 空串视为未设置
  ];
  for (const c of cases) {
    b.sandbox.localStorage.clear();
    if (c.ls !== null && c.ls !== '') b.sandbox.localStorage.setItem('devin-byok.lang', c.ls);
    b.sandbox.navigator.language = c.nav;
    assertEq(b.I18N.currentLang(), c.want, `ls=${JSON.stringify(c.ls)} nav=${c.nav}`);
  }
}

// —— setLang → applyLang → i18n:changed 链路 ——
console.log('setLang 链路:');
{
  const b = boot();
  let changed = 0;
  b.sandbox.document.addEventListener('i18n:changed', () => { changed++; });
  b.I18N.setLang('en');
  assertEq(b.sandbox.localStorage.getItem('devin-byok.lang'), 'en', 'localStorage 持久化');
  assertEq(b.sandbox.document.documentElement.lang, 'en', 'documentElement.lang 更新');
  assertEq(changed, 1, 'i18n:changed 派发一次');
  assertEq(b.toastCalls.length, 1, '切换 toast 提示');
  b.I18N.setLang('zh');
  assertEq(b.sandbox.localStorage.getItem('devin-byok.lang'), 'zh', '切回 zh 持久化');
  assertEq(changed, 2, '再次派发');
  b.I18N.setLang('fr');
  assertEq(changed, 2, '非法语言不派发事件');
  assertEq(b.sandbox.localStorage.getItem('devin-byok.lang'), 'zh', '非法语言不写入');
}

// —— applyLang 的 DOM 应用路径（textContent/innerHTML/placeholder/title）——
console.log('applyLang DOM 应用路径:');
{
  const els = {
    '[data-i18n]': [makeElement({ 'data-i18n': 'btn.start' })],
    '[data-i18n-html]': [makeElement({ 'data-i18n-html': 'settings.notes.body' })],
    '[data-i18n-placeholder]': [makeElement({ 'data-i18n-placeholder': 'settings.update.repoLabel' })],
    '[data-i18n-title]': [makeElement({ 'data-i18n-title': 'lang.title' })],
  };
  const b = boot(els);
  b.sandbox.navigator.language = 'en-US';
  b.I18N.applyLang();
  assertEq(els['[data-i18n]'][0].textContent, 'Enable & Import', 'data-i18n textContent 应用');
  assertEq(els['[data-i18n-html]'][0].innerHTML,
    'Providers (Base URL / API Key / upstream model) are configured in <b>Models → Family cards</b>, consistent with cursor-byok: multiple models, each with its own provider.',
    'data-i18n-html innerHTML 应用');
  assertEq(els['[data-i18n-placeholder]'][0].attrs['placeholder'], 'GitHub repo owner/name', 'data-i18n-placeholder 应用');
  assertEq(els['[data-i18n-title]'][0].attrs['title'], 'Switch interface language (current: English)', 'data-i18n-title 应用');
  assertEq(b.sandbox.document.documentElement.lang, 'en', 'documentElement.lang 更新');
  // 切回中文
  b.I18N.applyLang(); // 语言仍 en（localStorage 未设置）
  b.sandbox.navigator.language = 'zh-CN';
  b.I18N.applyLang();
  assertEq(els['[data-i18n]'][0].textContent, '启用并一键导入', '切回 zh 后文本重应用');
  assertEq(b.sandbox.document.documentElement.lang, 'zh-CN', '切回 zh 后 lang 更新');
}

// —— t() 占位符与缺失回退 ——
console.log('t() 行为:');
{
  const b = boot();
  b.sandbox.navigator.language = 'en-US';
  assertEq(b.I18N.t('btn.start'), 'Enable & Import', 'en 字典取值');
  b.sandbox.navigator.language = 'zh-CN';
  assertEq(b.I18N.t('btn.start'), '启用并一键导入', 'zh 字典取值');
  assertEq(b.I18N.t('no.such.key'), 'no.such.key', '缺失 key 回退 key 本身');
  assertEq(b.I18N.t('confirm.deleteFamily', { uid: 'grok' }), '删除模型 grok 及其所有思考强度变体？', '占位符替换');
}

if (failures) {
  console.error(`\ni18n 运行时测试失败: ${failures} 项`);
  process.exit(1);
}
console.log('\ni18n 运行时测试通过: currentLang 矩阵 / setLang 链路 / t() 行为');
