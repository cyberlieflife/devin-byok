// Devin BYOK 系统提示词编辑扩展 —— 与 local-api /api/prompts 同步
const vscode = require("vscode");

function portalUrl() {
  const c = vscode.workspace.getConfiguration("devinByok");
  const u = (c.get("portalUrl") || "http://127.0.0.1:8787").replace(/\/$/, "");
  return u;
}

async function api(method, path, body) {
  const url = portalUrl() + path;
  const init = {
    method,
    headers: { "Content-Type": "application/json" },
  };
  if (body !== undefined) init.body = JSON.stringify(body);
  const res = await fetch(url, init);
  const text = await res.text();
  let data;
  try {
    data = JSON.parse(text);
  } catch {
    data = { ok: false, message: text || res.statusText };
  }
  if (!res.ok) {
    throw new Error(data.message || data.error || "HTTP " + res.status);
  }
  return data;
}

function webviewHtml() {
  return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8"/>
<style>
  :root {
    --bg: var(--vscode-sideBar-background, #1e1e1e);
    --fg: var(--vscode-foreground, #ddd);
    --muted: var(--vscode-descriptionForeground, #999);
    --card: var(--vscode-editor-background, #252526);
    --border: var(--vscode-panel-border, #333);
    --accent: #f54e00;
    --ok: #1f8a65;
  }
  body { font-family: var(--vscode-font-family, system-ui); color: var(--fg); background: var(--bg); margin: 0; padding: 12px; }
  h2 { font-size: 14px; font-weight: 600; margin: 0 0 8px; }
  .muted { color: var(--muted); font-size: 12px; margin-bottom: 12px; }
  .row { display: flex; gap: 6px; margin-bottom: 8px; flex-wrap: wrap; }
  button, select, input, textarea {
    font: inherit; color: var(--fg); background: var(--card);
    border: 1px solid var(--border); border-radius: 6px; padding: 6px 10px;
  }
  button { cursor: pointer; }
  button.primary { background: var(--accent); border-color: var(--accent); color: #fff; }
  button.danger { border-color: #cf2d56; color: #cf2d56; }
  .card {
    background: var(--card); border: 1px solid var(--border); border-radius: 8px;
    padding: 10px; margin-bottom: 8px;
  }
  .card .title { font-weight: 600; margin-bottom: 4px; }
  .card .meta { font-size: 11px; color: var(--muted); margin-bottom: 6px; }
  textarea { width: 100%; min-height: 90px; box-sizing: border-box; resize: vertical; }
  input[type=text] { width: 100%; box-sizing: border-box; }
  label { font-size: 12px; color: var(--muted); display: block; margin: 6px 0 4px; }
  .status { font-size: 12px; margin-top: 8px; color: var(--muted); white-space: pre-wrap; }
  .status.ok { color: var(--ok); }
  .status.err { color: #cf2d56; }
</style>
</head>
<body>
  <h2>BYOK 系统提示词</h2>
  <div class="muted">服务启动时生效；数据存于 local-api，与 GUI 同步。</div>
  <div class="row">
    <button class="primary" id="btnRefresh">刷新</button>
    <button id="btnNew">新建</button>
  </div>
  <div id="list"></div>
  <div id="editor" style="display:none" class="card">
    <label>标题</label>
    <input type="text" id="title"/>
    <label>模式</label>
    <select id="mode">
      <option value="append">追加 (append)</option>
      <option value="prepend">前置 (prepend)</option>
      <option value="replace">替换官方 system (replace)</option>
    </select>
    <label><input type="checkbox" id="enabled" checked/> 启用</label>
    <label>正文</label>
    <textarea id="body" placeholder="写入要注入的系统提示词…"></textarea>
    <div class="row" style="margin-top:8px">
      <button class="primary" id="btnSave">保存</button>
      <button id="btnCancel">取消</button>
      <button class="danger" id="btnDel" style="display:none">删除</button>
    </div>
  </div>
  <div class="status" id="status"></div>
<script>
  const vscode = acquireVsCodeApi();
  let items = [];
  let editingId = null;

  function setStatus(msg, kind) {
    const el = document.getElementById('status');
    el.textContent = msg || '';
    el.className = 'status' + (kind ? ' ' + kind : '');
  }

  function render() {
    const list = document.getElementById('list');
    if (!items.length) {
      list.innerHTML = '<div class="muted">暂无提示词。点「新建」添加。</div>';
      return;
    }
    list.innerHTML = items.map(p => \`
      <div class="card" data-id="\${p.id}">
        <div class="title">\${escapeHtml(p.title || '(无标题)')} \${p.enabled ? '●' : '○'}</div>
        <div class="meta">\${p.mode || 'append'} · \${(p.body||'').length} 字</div>
        <div class="row">
          <button data-act="edit">编辑</button>
          <button data-act="toggle">\${p.enabled ? '禁用' : '启用'}</button>
          <button class="danger" data-act="del">删除</button>
        </div>
      </div>\`).join('');
    list.querySelectorAll('.card').forEach(card => {
      card.querySelectorAll('button').forEach(btn => {
        btn.addEventListener('click', () => {
          const id = card.getAttribute('data-id');
          const act = btn.getAttribute('data-act');
          vscode.postMessage({ type: act, id });
        });
      });
    });
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  }

  function openEditor(p) {
    editingId = p ? p.id : null;
    document.getElementById('editor').style.display = 'block';
    document.getElementById('title').value = p ? (p.title || '') : '';
    document.getElementById('mode').value = p ? (p.mode || 'append') : 'append';
    document.getElementById('enabled').checked = p ? !!p.enabled : true;
    document.getElementById('body').value = p ? (p.body || '') : '';
    document.getElementById('btnDel').style.display = p ? 'inline-block' : 'none';
  }

  document.getElementById('btnRefresh').onclick = () => vscode.postMessage({ type: 'refresh' });
  document.getElementById('btnNew').onclick = () => openEditor(null);
  document.getElementById('btnCancel').onclick = () => {
    document.getElementById('editor').style.display = 'none';
    editingId = null;
  };
  document.getElementById('btnSave').onclick = () => {
    vscode.postMessage({
      type: 'save',
      prompt: {
        id: editingId || '',
        title: document.getElementById('title').value,
        mode: document.getElementById('mode').value,
        enabled: document.getElementById('enabled').checked,
        body: document.getElementById('body').value,
      }
    });
  };
  document.getElementById('btnDel').onclick = () => {
    if (editingId) vscode.postMessage({ type: 'del', id: editingId });
  };

  window.addEventListener('message', ev => {
    const m = ev.data || {};
    if (m.type === 'list') {
      items = m.prompts || [];
      render();
      setStatus('已加载 ' + items.length + ' 条', 'ok');
    } else if (m.type === 'saved') {
      items = m.prompts || items;
      render();
      document.getElementById('editor').style.display = 'none';
      setStatus('已保存', 'ok');
    } else if (m.type === 'open' || m.type === "open") {
      openEditor(m.prompt || null);
    } else if (m.type === 'error') {
      setStatus(m.message || '错误', 'err');
    }
  });

  vscode.postMessage({ type: 'refresh' });
</script>
</body>
</html>`;
}

class PromptsViewProvider {
  constructor(context) {
    this._context = context;
    this._view = undefined;
  }
  resolveWebviewView(webviewView) {
    this._view = webviewView;
    webviewView.webview.options = { enableScripts: true };
    webviewView.webview.html = webviewHtml();
    webviewView.webview.onDidReceiveMessage(async (msg) => {
      try {
        if (msg.type === "refresh") {
          const data = await api("GET", "/api/prompts");
          webviewView.webview.postMessage({ type: "list", prompts: data.prompts || [] });
        } else if (msg.type === "save") {
          const data = await api("POST", "/api/prompts", msg.prompt || {});
          webviewView.webview.postMessage({ type: "saved", prompts: data.prompts || [] });
        } else if (msg.type === "del" || msg.type === "delete") {
          const data = await api("DELETE", "/api/prompts?id=" + encodeURIComponent(msg.id || ""));
          webviewView.webview.postMessage({ type: "saved", prompts: data.prompts || [] });
        } else if (msg.type === "edit") {
          const data = await api("GET", "/api/prompts");
          const p = (data.prompts || []).find((x) => x.id === msg.id);
          // re-send list; editor open is client-side via separate post — simplify: refresh then instruct
          webviewView.webview.postMessage({ type: "list", prompts: data.prompts || [] });
          if (p) {
            // inject open via custom
            webviewView.webview.postMessage({ type: "open", prompt: p });
          }
        } else if (msg.type === "toggle") {
          const data = await api("GET", "/api/prompts");
          const p = (data.prompts || []).find((x) => x.id === msg.id);
          if (!p) throw new Error("未找到");
          p.enabled = !p.enabled;
          const saved = await api("POST", "/api/prompts", p);
          webviewView.webview.postMessage({ type: "saved", prompts: saved.prompts || [] });
        }
      } catch (e) {
        webviewView.webview.postMessage({ type: "error", message: String(e && e.message ? e.message : e) });
      }
    });
  }
}

function activate(context) {
  const provider = new PromptsViewProvider(context);
  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider("devinByok.prompts", provider)
  );
  context.subscriptions.push(
    vscode.commands.registerCommand("devinByok.prompts.refresh", () => {
      if (provider._view) provider._view.webview.postMessage({ type: "noop" });
      // trigger refresh via re-resolve not needed; post to webview if exists
      try {
        if (provider._view) {
          provider._view.webview.postMessage({ type: "list", prompts: [] });
        }
      } catch (_) {}
    })
  );
  context.subscriptions.push(
    vscode.commands.registerCommand("devinByok.prompts.openGui", () => {
      vscode.env.openExternal(vscode.Uri.parse(portalUrl() + "/ui/"));
    })
  );
}

function deactivate() {}

module.exports = { activate, deactivate };
