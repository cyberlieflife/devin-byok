#!/usr/bin/env node
/**
 * 前端 app.js 顶层执行回归测试（issue#5 配套）：
 *   在 Node 中 mock 浏览器环境后加载 i18n.js + app.js，断言顶层执行不抛错、
 *   关键全局函数已定义——防止"t 未全局暴露导致 app.js 顶层中断"类回归。
 * 无第三方依赖，node >= 14 直接运行：node scripts/test-appjs.mjs
 */
'use strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const ROOT = path.join(path.dirname(fileURLToPath(import.meta.url)), '..');
const ui = path.join(ROOT, 'internal', 'localapi', 'ui');
const i18nSrc = fs.readFileSync(path.join(ui, 'i18n.js'), 'utf8');
const appSrc = fs.readFileSync(path.join(ui, 'app.js'), 'utf8');

function makeEl() {
  return {
    textContent: '', innerHTML: '', hidden: false, value: '', checked: false,
    dataset: {}, style: {}, options: [],
    classList: { toggle() {}, add() {}, remove() {} },
    setAttribute() {}, getAttribute() { return null; },
    addEventListener() {}, removeEventListener() {},
  };
}

const sandbox = {
  console, setTimeout, clearTimeout, setInterval, clearInterval,
  fetch: async () => ({ ok: true, status: 200, text: async () => '{}', json: async () => ({ ok: true }) }),
  navigator: { language: 'zh-CN' },
  localStorage: { getItem: () => null, setItem() {}, removeItem() {} },
  CustomEvent: class { constructor(t, o) { this.type = t; this.detail = (o || {}).detail || {}; } },
  document: {
    documentElement: { setAttribute() {} },
    readyState: 'complete',
    addEventListener() {}, dispatchEvent() {},
    querySelectorAll: () => [], querySelector: () => null,
    getElementById: () => makeEl(), createElement: () => makeEl(),
  },
};
sandbox.window = sandbox;
vm.createContext(sandbox);

let failures = 0;
function assert(cond, label) {
  if (cond) { console.log(`  ✓ ${label}`); }
  else { failures++; console.error(`  ✗ ${label}`); }
}

try {
  vm.runInContext(i18nSrc, sandbox, { filename: 'i18n.js' });
  console.log('i18n.js 执行 OK');
} catch (e) {
  failures++;
  console.error(`  ✗ i18n.js 执行抛错: ${e.message}`);
}

assert(typeof sandbox.t === 'function', '全局 t() 已暴露');
assert(typeof sandbox.I18N === 'object' && typeof sandbox.I18N.t === 'function', 'I18N.t 存在');

try {
  vm.runInContext(appSrc, sandbox, { filename: 'app.js' });
  console.log('app.js 顶层执行 OK');
} catch (e) {
  failures++;
  console.error(`  ✗ app.js 顶层执行抛错: ${e.message}`);
}

for (const fn of ['showPage', 'refreshAll', 'refreshStatus', 'refreshMetrics', 'refreshFamilies', 'loadFooterVersion', 'control', 'startUpdateAutoCheck', 'renderFooterUpdate', 'saveConfig']) {
  assert(typeof sandbox[fn] === 'function', `关键函数 ${fn} 已定义`);
}

if (failures) {
  console.error(`\napp.js 顶层执行测试失败: ${failures} 项`);
  process.exit(1);
}
console.log('\napp.js 顶层执行测试通过: i18n.js/app.js 加载无错，关键函数全部就绪');
// app.js 顶层注册了 setInterval，需显式退出否则进程被 timer 挂住
process.exit(0);
