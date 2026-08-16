#!/usr/bin/env node
/**
 * 前端 i18n 一致性校验（issue#5 配套）：
 *   1. zh/en 字典 key 对称；
 *   2. index.html 的 data-i18n* 引用与 app.js 的 t('...') 引用全部命中字典；
 *   3. en 疑似漏翻（与 zh 相同且含中文）报错；
 *   4. index.html / app.js 中残留的界面中文（非 data-i18n、非注释、非正则）报错。
 * 无第三方依赖，node >= 14 直接运行：node scripts/check-i18n.mjs
 */
'use strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.join(path.dirname(fileURLToPath(import.meta.url)), '..');
const UI = path.join(ROOT, 'internal', 'localapi', 'ui');
const i18nSrc = fs.readFileSync(path.join(UI, 'i18n.js'), 'utf8');
const htmlSrc = fs.readFileSync(path.join(UI, 'index.html'), 'utf8');
const appSrc = fs.readFileSync(path.join(UI, 'app.js'), 'utf8');

function dictKeys(block) {
  const m = {};
  for (const mm of block.matchAll(/'([^']+)':\s*'([^']*)'/g)) m[mm[1]] = mm[2];
  return m;
}
const zhStart = i18nSrc.indexOf('zh: {');
const enStart = i18nSrc.indexOf('en: {');
if (zhStart < 0 || enStart < 0) {
  console.error('i18n 校验失败: 找不到 zh:/en: 字典块');
  process.exit(1);
}
const zh = dictKeys(i18nSrc.slice(zhStart, enStart));
const en = dictKeys(i18nSrc.slice(enStart, i18nSrc.indexOf('};', enStart)));

const errors = [];

// 1. 字典对称
for (const k of Object.keys(zh)) if (!(k in en)) errors.push(`en 字典缺失 key: ${k}`);
for (const k of Object.keys(en)) if (!(k in zh)) errors.push(`zh 字典缺失 key: ${k}`);

// 2. 引用覆盖
for (const mm of htmlSrc.matchAll(/data-i18n(?:-html|-placeholder|-title)?="([^"]+)"/g)) {
  if (!(k => k in zh)(mm[1])) errors.push(`index.html 引用缺失 key: ${mm[1]}`);
}
for (const mm of appSrc.matchAll(/(?<![A-Za-z])t\('([^']+)'/g)) {
  if (!(mm[1] in zh)) errors.push(`app.js t() 引用缺失 key: ${mm[1]}`);
}

// 3. en 疑似漏翻
for (const k of Object.keys(zh)) {
  if (en[k] === zh[k] && /[\u4e00-\u9fff]/.test(zh[k])) {
    errors.push(`en 疑似未翻译（与 zh 相同且含中文）: ${k}`);
  }
}

// 4. index.html 行级中文残留（跳过含 data-i18n 的行与注释行）
for (const line of htmlSrc.split('\n')) {
  if (line.includes('data-i18n')) continue;
  if (line.includes('<!--')) continue;
  if (/[\u4e00-\u9fff]/.test(line)) errors.push(`index.html 未本地化中文: ${line.trim().slice(0, 90)}`);
}

// 5. app.js 行级中文残留（跳过注释、t() 调用、正则字面量）
for (const line of appSrc.split('\n')) {
  const t = line.trim();
  if (!t || t.startsWith('//') || t.startsWith('*')) continue;
  if (t.includes("t('") || t.includes('t("')) continue;
  if (/\/[^/]*[\u4e00-\u9fff][^/]*\//.test(t)) continue; // 正则字面量（中英双匹配关键词）
  if (/[\u4e00-\u9fff]/.test(t)) errors.push(`app.js 未本地化中文: ${t.slice(0, 90)}`);
}

if (errors.length) {
  console.error(`i18n 校验失败（${errors.length} 项）:`);
  for (const e of errors) console.error('  - ' + e);
  process.exit(1);
}
console.log(`i18n 校验通过: zh/en 各 ${Object.keys(zh).length} key，HTML/JS 引用全部覆盖，无界面中文残留`);
