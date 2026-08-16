
let currentPage = 'monitor';

async function jget(url){const r=await fetch(url,{headers:{'X-Lang':I18N.currentLang()}}); if(!r.ok) throw new Error(await r.text()); return r.json()}
async function jsend(url, method, body){
  const r=await fetch(url,{method,headers:{'Content-Type':'application/json','X-Lang':I18N.currentLang()},body:body?JSON.stringify(body):undefined});
  const t=await r.text(); let data; try{data=JSON.parse(t)}catch{data={raw:t}}
  if(!r.ok) throw new Error(data.error||data.message||t||r.statusText);
  return data;
}

// 数量紧凑显示：不足 1000 原样；否则 K/M/B 保留 2 位小数
function formatCompact(n){
  n = Number(n);
  if(!isFinite(n)) return '0';
  const sign = n < 0 ? '-' : '';
  let v = Math.abs(n);
  if(v < 1000) return sign + String(Math.round(v));
  let unit = 'K';
  if(v >= 1e9){ v = v/1e9; unit = 'B'; }
  else if(v >= 1e6){ v = v/1e6; unit = 'M'; }
  else { v = v/1e3; unit = 'K'; }
  return sign + v.toFixed(2) + unit;
}

function toast(msg){const el=document.getElementById('toast'); el.textContent=msg; el.style.display='block'; setTimeout(()=>el.style.display='none',2800)}
function showPage(name){
  currentPage=name;
  document.querySelectorAll('.page').forEach(p=>p.classList.remove('active'));
  const page=document.getElementById('page-'+name);
  if(page) page.classList.add('active');
  document.querySelectorAll('.btn-nav[data-page]').forEach(btn=>{
    const selected = btn.dataset.page === name;
    btn.classList.toggle('active', selected);
    btn.setAttribute('aria-current', selected ? 'page' : 'false');
  });
  if(name==='models') refreshFamilies();
  if(name==='settings'){ refreshConfig(); refreshDesktopPrefs(); refreshLocalAccount(); }
  if(name==='prompts') refreshPrompts();
}
function setDot(ok,text){
  const d=document.getElementById('apiDot'); const t=document.getElementById('apiText');
  d.className='dot '+(ok?'ok':'bad'); t.textContent=text;
}
function drawPie(hit, miss){
  const c=document.getElementById('pieCache'); const ctx=c.getContext('2d');
  const total=hit+miss; const rate=total?hit/total:0;
  document.getElementById('pieRate').textContent=Math.round(rate*100)+'%';
  document.getElementById('cacheDetail').textContent=`cached ${hit} / prompt ${miss}`;
  ctx.clearRect(0,0,c.width,c.height);
  const cx=90,cy=90,r=78;
  // track
  ctx.beginPath(); ctx.arc(cx,cy,r,0,Math.PI*2); ctx.strokeStyle='#efeee8'; ctx.lineWidth=16; ctx.stroke();
  if(total>0){
    const start=-Math.PI/2; const end=start+rate*Math.PI*2;
    ctx.beginPath(); ctx.arc(cx,cy,r,start,end); ctx.strokeStyle='#f54e00'; ctx.lineWidth=16; ctx.lineCap='round'; ctx.stroke();
  }
}
function renderFeatureRank(list){
  const el=document.getElementById('featureModelRank');
  if(!el) return;
  if(!list||!list.length){el.innerHTML='<div class="muted">暂无数据</div>';return;}
  const max=Math.max(...list.map(x=>x.count||0),1);
  el.innerHTML=list.slice(0,12).map((x,i)=>`<div class="rank-item"><span class="rank-n">${i+1}</span><span class="mono">${escapeHtml(x.model||'')}</span><div class="rank-bar"><i style="width:${Math.round((x.count||0)*100/max)}%"></i></div><span>${x.count||0}</span></div>`).join('');
}
function renderRank(list){
  const el=document.getElementById('modelRank');
  if(!list||!list.length){el.innerHTML='<div class="muted">暂无数据</div>'; return}
  const max=Math.max(...list.map(x=>x.count||0),1);
  el.innerHTML=list.slice(0,8).map((x,i)=>`
    <div class="rank-item">
      <span class="rank-n">${i+1}</span>
      <div style="flex:1">
        <div style="display:flex;justify-content:space-between;gap:8px;margin-bottom:4px">
          <span class="mono">${escapeHtml(x.model||'')}</span>
          <span class="muted">${x.count}</span>
        </div>
        <div class="rank-bar"><i style="width:${Math.round((x.count/max)*100)}%"></i></div>
      </div>
    </div>`).join('');
}
function renderLogs(logs){
  const box=document.getElementById('logBox');
  if(!box) return;
  if(!logs||!logs.length){box.innerHTML='<div class="muted">等待日志…</div>'; return}
  // 命令行风格：正序展示，新日志在底部；取最后 120 条
  const arr = logs.length>120 ? logs.slice(logs.length-120) : logs;
  const atBottom = (box.scrollTop + box.clientHeight) >= (box.scrollHeight - 24);
  box.innerHTML=arr.map(l=>{
    const lv=(l.level||'info').toLowerCase();
    return `<div class="log-line ${lv==='error'||lv==='err'?'err':'info'}"><span class="t">${escapeHtml(l.time||'')}</span>${escapeHtml(l.message||'')}</div>`;
  }).join('');
  // 用户未上滚查看历史时，自动贴底
  if(atBottom || box.dataset.forceBottom==='1' || !box.dataset.inited){
    box.scrollTop = box.scrollHeight;
    box.dataset.inited='1';
  }
}

async function refreshMetrics(){
  try{
    if(typeof window.nativeOnline==='function' && !(await window.nativeOnline())){ setDot(false,'API 离线'); return }
    const m=await jget('/api/metrics');
    document.getElementById('mOk').textContent=m.req_ok??0;
    document.getElementById('mFail').textContent=m.req_fail??0;
    document.getElementById('mTin').textContent=formatCompact(m.tokens_in??0);
    document.getElementById('mTout').textContent=formatCompact(m.tokens_out??0);
    const elDW=document.getElementById('mDeepWiki'); if(elDW) elDW.textContent=m.deepwiki_ok??0;
    const elDWf=document.getElementById('mDeepWikiFail'); if(elDWf) elDWf.textContent=m.deepwiki_fail??0;
    const elCM=document.getElementById('mCodeMap'); if(elCM) elCM.textContent=m.codemap_ok??0;
    const elCMf=document.getElementById('mCodeMapFail'); if(elCMf) elCMf.textContent=m.codemap_fail??0;
    const elF=document.getElementById('mCodeMapFast'); if(elF) elF.textContent=m.codemap_fast??0;
    const elS=document.getElementById('mCodeMapSmart'); if(elS) elS.textContent=m.codemap_smart??0;
  const elCmt=document.getElementById('mCommit'); if(elCmt) elCmt.textContent=m.commit_ok??0;
  const elCmtF=document.getElementById('mCommitFail'); if(elCmtF) elCmtF.textContent=m.commit_fail??0;
  const elFC=document.getElementById('mFastContext'); if(elFC) elFC.textContent=m.fast_context_ok??0;
  const elFCf=document.getElementById('mFastContextFail'); if(elFCf) elFCf.textContent=m.fast_context_fail??0;
    renderFeatureRank(m.feature_model_rank||[]);
    const prompt=m.prompt_tokens||0, cached=m.cached_tokens||0;
    if(prompt>0){ drawPie(cached, Math.max(prompt-cached,0)); document.getElementById('cacheDetail').textContent=`cached ${formatCompact(cached)} / prompt ${formatCompact(prompt)}`; }
    else { drawPie(m.cache_hit||0, m.cache_miss||0); document.getElementById('cacheDetail').textContent=`local hit ${m.cache_hit||0} / miss ${m.cache_miss||0}`; }
    renderRank(m.model_rank||[]);
    renderLogs(m.logs||[]);
    document.getElementById('uptime').textContent='uptime '+fmtDur(m.uptime_sec||0);
    setDot(true,'API 在线');
  }catch(e){
    setDot(false,'API 离线');
  }
}
async function refreshStatus(){
  try{
    const s=await jget('/api/status');
    setDot(true,'管理端在线');
    renderServiceStatus(s);
    const cfgPath=document.getElementById('cfgPath');
    if(cfgPath) cfgPath.textContent='config: '+(s.config_path||'-');
  }catch(e){ setDot(false,'API 离线') }
}
function renderServiceStatus(s){
  const active=!!s.service_active;
  const indicator=document.getElementById('serviceIndicator');
  const title=document.getElementById('serviceTitle');
  const message=document.getElementById('serviceMessage');
  const start=document.getElementById('btnServiceStart');
  const stop=document.getElementById('btnServiceStop');
  if(indicator) indicator.className='service-indicator '+(active?'active':'stopped');
  if(title) title.textContent=active?'本地模型服务已启用':'本地模型服务未启用';
  if(message) message.textContent=active
    ? (s.restart_required?'配置已导入。重启 Devin 后加载当前模型。':'Devin 已连接本地模型服务。')
    : '管理端保持在线，可随时一键创建虚拟账户并导入。';
  if(start){ start.disabled=active; start.textContent=active?'已启用':'启用并一键导入'; }
  if(stop) stop.disabled=!active;
  const account=document.getElementById('factAccount');
  const model=document.getElementById('factModel');
  const devin=document.getElementById('factDevin');
  const config=document.getElementById('factConfig');
  if(account) account.textContent=s.account_imported?'已导入':(s.account_exists?'待导入':'未创建');
  if(model) model.textContent=s.default_model_name||s.default_model||'-';
  if(devin) devin.textContent=s.devin_running?'正在运行':'未运行';
  if(config) config.textContent=active?'已应用':'原始配置';
}
async function refreshLocalAccount(){
  const state=document.getElementById('localAccountState');
  const badge=document.getElementById('localAccountBadge');
  const fields=document.getElementById('localAccountFields');
  if(!state||!badge||!fields) return;
  try{
    const account=await jget('/api/local-account');
    if(account.ok===false) throw new Error(account.message||'状态读取失败');
    fields.hidden=!account.account_exists;
    if(!account.account_exists){
      state.textContent='尚未创建本机身份';
      badge.textContent='未创建';
      badge.className='account-badge';
      return;
    }
    document.getElementById('localAccountName').textContent=account.name||'-';
    document.getElementById('localAccountEmail').textContent=account.email||'-';
    document.getElementById('localAccountID').textContent=account.id||'-';
    document.getElementById('localAccountKey').textContent=account.api_key_masked||'****';
    const imported=!!account.imported;
    state.textContent=imported?'已连接本地服务，重启 Devin 后生效':(account.message||'账户已创建，尚未导入 Devin');
    badge.textContent=imported?'已导入':'待导入';
    badge.className='account-badge '+(imported?'ok':'pending');
    const button=document.getElementById('btnImportLocalAccount');
    if(button) button.textContent=imported?'重新导入配置':'一键创建并导入';
  }catch(e){
    fields.hidden=true;
    state.textContent=e.message||String(e);
    badge.textContent='异常';
    badge.className='account-badge bad';
  }
}
async function importLocalAccount(){
  const button=document.getElementById('btnImportLocalAccount');
  if(!button) return;
  const oldText=button.textContent;
  button.disabled=true;
  button.textContent='正在导入…';
  try{
    const account=await jsend('/api/local-account','POST');
    if(account.ok===false) throw new Error(account.message||'导入失败');
    toast(account.message||'本地虚拟账户已导入');
    await Promise.all([refreshLocalAccount(),refreshConfig()]);
    await refreshStatus();
  }catch(e){
    toast('导入失败: '+(e.message||e));
    await refreshLocalAccount();
  }finally{
    button.disabled=false;
    button.textContent=oldText;
  }
}
async function refreshConfig(){
  refreshDesktopPrefs();
  try{
    const c=await jget('/api/config');
    const ue=document.getElementById('update_enabled'); if(ue && c.update_enabled!=null) ue.checked=!!c.update_enabled;
    const ua=document.getElementById('update_auto_apply'); if(ua && c.update_auto_apply!=null) ua.checked=!!c.update_auto_apply;
    const ur=document.getElementById('update_repo'); if(ur && c.update_repo!=null) ur.value=c.update_repo||'';
    const us=document.getElementById('updateStatus');
    if(us){ try{ const v=await jget('/api/version'); us.textContent='当前版本 v'+(v.version||'?'); }catch(_e){} }
  }catch(_e){}

  try{
    const c=await jget('/api/config');
    const to=document.getElementById('timeout_sec'); if(to) to.value=c.timeout_sec||120;
    fillFeatureModelSelects(c.models||window.__models||[], c);
    document.getElementById('tools_mode').value=c.tools_mode||'standard';
    document.getElementById('tools_timeout_sec').value=c.tools_timeout_sec??300;
    document.getElementById('enable_stream').checked=!!c.enable_stream;
    document.getElementById('enable_cascade_tools').checked=!!c.enable_cascade_tools;
    document.getElementById('pure_local').checked=!!c.pure_local;
    const qe=document.getElementById('quality_enabled'); if(qe) qe.checked=!!c.quality_enabled;
    const qm=document.getElementById('quality_mode'); if(qm) qm.value=c.quality_mode||'balanced';
    const qr=document.getElementById('max_verification_rounds'); if(qr) qr.value=c.max_verification_rounds||1;
    document.getElementById('cfgPath').textContent='config: '+(c.config_path||'-');
  }catch(e){}
}
window.__fams={};
async function refreshFamilies(){
  try{ const cfg=await jget('/api/config'); fillFeatureModelSelects(cfg.models||[], cfg);}catch(_e){}

  const box=document.getElementById('familyCards');
  try{
    const res=await jget('/api/families');
    const fams=res.families||[];
    window.__fams={};
    fams.forEach(f=>{window.__fams[f.uid]=f});
    if(!fams.length){box.innerHTML='<div class="card muted">还没有模型，点击右上角添加</div>'; return}
    box.innerHTML=fams.map(f=>`
      <div class="card family-card">
        <h3>${escapeHtml(f.label||f.uid)}</h3>
        <div class="muted small mono">${escapeHtml(f.uid)}</div>
        <div class="muted small" style="margin-top:6px">${escapeHtml(providerLabel(f.provider))} · ctx ${f.context_window?formatCompact(f.context_window):'-'} · max_out ${f.max_tokens?formatCompact(f.max_tokens):'-'}</div>
        <div class="muted small mono" style="margin-top:4px">${escapeHtml(f.base_url||'(no base_url)')}</div>
        <div class="muted small">key: ${escapeHtml(f.api_key_set?(f.api_key_masked||'****'):'未设置')} · model: ${escapeHtml(f.upstream_model||'-')}</div>
        <div class="chip-row">
          ${(f.variants||[]).map(v=>`<span class="chip ${v.thinking==='medium'?'med':''}">${escapeHtml(v.thinking||'?')} · ${escapeHtml(v.id)}</span>`).join('')}
        </div>
        <div class="muted small">upstream: ${escapeHtml((f.variants&&f.variants[0]&&f.variants[0].upstream_model)||'-')}</div>
        <div class="row gap" style="margin-top:12px">
          <button class="btn btn-secondary" type="button" data-fuid="${escapeHtml(f.uid)}">编辑</button>
          <button class="btn btn-secondary" type="button" data-fuid="${escapeHtml(f.uid)}">复制</button>
          <button class="btn btn-danger" type="button" data-fuid="${escapeHtml(f.uid)}">删除</button>
        </div>
      </div>`).join('');
    // 家族卡片按钮：data-* + addEventListener 绑定，避免属性内联 JS 的双上下文转义失效
    box.querySelectorAll('button[data-act]').forEach(btn=>{
      const uid=btn.getAttribute('data-fuid');
      const act=btn.getAttribute('data-act');
      btn.addEventListener('click',()=>{
        if(!uid) return;
        if(act==='edit'){editFamilyByUid(uid);}
        else if(act==='copy'){copyFamily(uid);}
        else if(act==='del'){deleteFamily(uid);}
      });
    });
  }catch(e){box.innerHTML='<div class="card bad">加载失败: '+escapeHtml(e.message)+'</div>'}
}
function formPatch(){
  return {
    timeout_sec:Number((document.getElementById('timeout_sec')||{}).value||0),
    tools_mode:document.getElementById('tools_mode').value,
    tools_timeout_sec:Number(document.getElementById('tools_timeout_sec').value||0),
    enable_stream:document.getElementById('enable_stream').checked,
    update_enabled: !!(document.getElementById('update_enabled')||{}).checked,
      update_auto_apply: !!(document.getElementById('update_auto_apply')||{}).checked,
      update_repo: ((document.getElementById('update_repo')||{}).value||'').trim(),
      quality_enabled: !!(document.getElementById('quality_enabled')||{}).checked,
      quality_mode: ((document.getElementById('quality_mode')||{}).value||'balanced').trim(),
      max_verification_rounds: Number((document.getElementById('max_verification_rounds')||{}).value||1),
      enable_cascade_tools:document.getElementById('enable_cascade_tools').checked,
    pure_local:document.getElementById('pure_local').checked,
  };
}
async function saveConfig(){
  try{ const res=await jsend('/api/config','PUT',formPatch()); toast(res.message||'已保存'); await refreshAll(); }
  catch(e){ toast('保存失败: '+e.message) }
}
async function testUpstream(){
  const el=document.getElementById('upResult'); el.textContent='测试中…';
  try{
    const res=await jsend('/api/test-upstream','POST');
    el.textContent=res.ok?('OK '+ (res.text||'')):('FAIL '+(res.error||''));
    toast(res.ok?'上游正常':(res.error||'失败'));
  }catch(e){ el.textContent=e.message; toast(e.message) }
}
async function control(action){
  const button=document.getElementById(action==='start'?'btnServiceStart':'btnServiceStop');
  const original=button?button.textContent:'';
  if(button){ button.disabled=true; button.textContent=action==='start'?'正在启用…':'正在恢复…'; }
  try{
    const res=await jsend('/api/control/'+action,'POST');
    if(res.ok===false) throw new Error(res.message||'操作失败');
    toast(res.message||action);
    await refreshAll();
  }catch(e){
    if(action==='start' && typeof window.nativeStart==='function'){
      try{
        const msg=await window.nativeStart();
        toast(msg==='ok'?'服务已启动':msg);
        await waitOnline(8000);
        await refreshAll();
        return;
      }catch(e2){ toast(String(e2)); return }
    }
    toast(String(e.message||e));
  }finally{
    if(button){ button.disabled=false; button.textContent=original; }
    await refreshStatus();
  }
}

async function restartDevin(){
  const button=document.getElementById('btnRestartDevin');
  if(!button) return;
  const original=button.textContent;
  button.disabled=true;
  button.textContent='正在重启…';
  try{
    const result=await jsend('/api/devin/restart','POST');
    if(result.ok===false) throw new Error(result.message||'重启失败');
    toast(result.message||'Devin 已重启');
    await refreshStatus();
  }catch(e){ toast('重启失败: '+(e.message||e)); }
  finally{ button.disabled=false; button.textContent=original; }
}

async function exportChats(){
  const button=document.getElementById('btnExportChats');
  const result=document.getElementById('chatExportResult');
  if(!button) return;
  const original=button.textContent;
  button.disabled=true;
  button.textContent='正在导出…';
  if(result) result.textContent='正在创建聊天备份…';
  try{
    const data=await jsend('/api/chats/export','POST');
    if(data.ok===false) throw new Error(data.message||'导出失败');
    const text=(data.message||'导出完成')+'：'+(data.path||'');
    if(result) result.textContent=text;
    toast('聊天记录已导出');
  }catch(e){
    if(result) result.textContent='导出失败：'+(e.message||e);
    toast('导出失败: '+(e.message||e));
  }finally{ button.disabled=false; button.textContent=original; }
}
function waitOnline(ms){
  const t0=Date.now();
  return new Promise(async (resolve)=>{
    while(Date.now()-t0<ms){
      try{
        if(typeof window.nativeOnline==='function'){
          if(await window.nativeOnline()){ resolve(true); return }
        }else{
          const r=await fetch('/healthz'); if(r.ok){ resolve(true); return }
        }
      }catch(_e){}
      await new Promise(r=>setTimeout(r,200));
    }
    resolve(false);
  });
}

function slugish(s){
  return String(s||'').trim().toLowerCase()
    .replace(/[^a-z0-9]+/g,'-')
    .replace(/^-+|-+$/g,'')
    .replace(/-+/g,'-');
}
function ensureByokUID(raw){
  let s = slugish(raw);
  if(s.endsWith('-byok')) s = s.slice(0,-5);
  s = s.replace(/^-+|-+$/g,'');
  if(!s) s = 'model';
  return s + '-byok';
}
function providerLabel(p){
  p = String(p||'openai').toLowerCase();
  if(p==='anthropic') return 'Anthropic';
  if(p==='responses') return 'OpenAI Responses';
  return 'OpenAI 兼容';
}
function selectedLevels(){
  return [...document.querySelectorAll('.f-level:checked')].map(x=>x.value);
}
function setLevels(levels){
  const set = new Set((levels||[]).map(x=>String(x).toLowerCase()));
  document.querySelectorAll('.f-level').forEach(cb=>{
    cb.checked = set.size ? set.has(cb.value) : (cb.value==='low'||cb.value==='medium'||cb.value==='high');
  });
}
function refreshUidHint(){
  const label = (document.getElementById('f_label')||{}).value || '';
  const up = (document.getElementById('f_upstream')||{}).value || '';
  const existing = (document.getElementById('f_uid')||{}).value || '';
  const uid = existing ? ensureByokUID(existing) : ensureByokUID(label || up);
  const hint = document.getElementById('f_uid_hint');
  if(hint) hint.textContent = 'Family UID 将自动生成为：' + (uid || '-');
}
function focusFamilyForm(){
  // macOS 原生 WebView 启动时可能只激活窗口，不会把键盘焦点交给 HTML。
  setTimeout(()=>{
    const el = document.getElementById('f_label');
    if(el){ el.focus({preventScroll:true}); el.select(); }
  }, 30);
}
function openFamilyModal(){
  const title = document.getElementById('familyModalTitle');
  if(title) title.textContent = '添加模型';
  document.getElementById('f_uid').value = '';
  document.getElementById('f_label').value = '';
  document.getElementById('f_upstream').value = '';
  document.getElementById('f_provider').value = 'openai';
  document.getElementById('f_base').value = '';
  document.getElementById('f_key').value = '';
  document.getElementById('f_key').placeholder = 'sk-...';
  document.getElementById('f_ctx').value = 128000;
  document.getElementById('f_max').value = 8192;
  document.getElementById('f_thinking_type').value = '';
  document.getElementById('f_thinking_budget').value = 0;
  document.getElementById('f_thinking_param').value = 'reasoning_effort';
  setLevels(['low','medium','high']);
  refreshUidHint();
  document.getElementById('familyModal').hidden = false;
  focusFamilyForm();
}
function closeFamilyModal(){ document.getElementById('familyModal').hidden = true }
function editFamilyByUid(uid){ const f=window.__fams[uid]; if(!f){toast('模型不存在');return;} editFamily(f);}
function copyFamily(uid){
  const f=window.__fams[uid]; if(!f){toast('模型不存在');return;}
  editFamily(f);
  const title = document.getElementById('familyModalTitle');
  if(title) title.textContent = '复制模型';
  document.getElementById('f_uid').value = '';
  document.getElementById('f_label').value = (f.label || f.uid) + ' Copy';
  refreshUidHint();
}
function editFamily(f){
  const title = document.getElementById('familyModalTitle');
  if(title) title.textContent = '编辑模型';
  document.getElementById('f_uid').value = f.uid || '';
  document.getElementById('f_label').value = f.label || '';
  document.getElementById('f_provider').value = f.provider || 'openai';
  document.getElementById('f_base').value = f.base_url || '';
  document.getElementById('f_key').value = '';
  document.getElementById('f_key').placeholder = f.api_key_set ? ('当前: '+(f.api_key_masked||'****')) : '未设置';
  document.getElementById('f_upstream').value = f.upstream_model || (f.variants&&f.variants[0]&&f.variants[0].upstream_model) || '';
  document.getElementById('f_ctx').value = f.context_window || 128000;
  document.getElementById('f_max').value = f.max_tokens || 8192;
  document.getElementById('f_thinking_type').value = f.thinking_type || '';
  document.getElementById('f_thinking_budget').value = f.thinking_budget_tokens || 0;
  document.getElementById('f_thinking_param').value = f.thinking_param || 'reasoning_effort';
  const levels = (f.variants||[]).map(v=>v.thinking).filter(Boolean);
  setLevels(levels.length ? levels : ['low','medium','high']);
  refreshUidHint();
  document.getElementById('familyModal').hidden = false;
  focusFamilyForm();
}
async function saveFamily(){
  const label = document.getElementById('f_label').value.trim();
  const upstream = document.getElementById('f_upstream').value.trim();
  if(!label){ toast('请填写显示名 Label'); return }
  if(!upstream){ toast('请填写上游模型 ID'); return }
  const existing = document.getElementById('f_uid').value.trim();
  const uid = existing ? ensureByokUID(existing) : ensureByokUID(label || upstream);
  const levels = selectedLevels();
  if(!levels.length){ toast('请至少选择一种思考强度'); return }
  const body = {
    label,
    uid,
    provider: document.getElementById('f_provider').value,
    base_url: document.getElementById('f_base').value.trim(),
    api_key: document.getElementById('f_key').value.trim(),
    upstream_model: upstream,
    context_window: Number(document.getElementById('f_ctx').value||0),
    max_tokens: Number(document.getElementById('f_max').value||0),
    thinking_type: document.getElementById('f_thinking_type').value,
    thinking_budget_tokens: Number(document.getElementById('f_thinking_budget').value||0),
    thinking_param: document.getElementById('f_thinking_param').value.trim(),
    levels,
  };
  try{
    await jsend('/api/families','POST', body);
    toast('模型已保存');
    closeFamilyModal();
    refreshFamilies();
  }catch(e){ toast(e.message) }
}
async function deleteFamily(uid){
  if(!confirm('删除模型 '+uid+' 及其所有思考强度变体？')) return;
  try{
    await jsend('/api/families?uid='+encodeURIComponent(uid),'DELETE');
    toast('已删除'); refreshFamilies();
  }catch(e){ toast(e.message) }
}

function escapeHtml(s){return String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
function fmtDur(sec){const h=Math.floor(sec/3600),m=Math.floor((sec%3600)/60),s=sec%60; return (h?h+'h ':'')+(m?m+'m ':'')+s+'s'}

// ---- DeepWiki / CodeMap Fast/Smart 模型绑定 ----
function modelOptionLabel(m){
  const fam = m.family || m.family_uid || '';
  const th = m.thinking || '';
  const label = m.label || m.id || '';
  const parts = [];
  if(fam) parts.push(fam);
  if(th) parts.push(th);
  if(label && label!==fam) parts.push(label);
  return (parts.join(' · ') || m.id) + '  (' + (m.id||'') + ')';
}
function fillOneModelSelect(selId, cur, list){
  const sel = document.getElementById(selId);
  if(!sel) return;
  let html = '<option value="">（默认模型）</option>';
  for(const m of list){
    const id = m.id || '';
    if(!id) continue;
    const selected = id===cur ? ' selected' : '';
    html += '<option value="'+escapeHtml(id)+'"'+selected+'>'+escapeHtml(modelOptionLabel(m))+'</option>';
  }
  sel.innerHTML = html;
  if(cur && ![...sel.options].some(o=>o.value===cur)){
    sel.innerHTML = '<option value="'+escapeHtml(cur)+'" selected>'+escapeHtml(cur)+' （已配置，列表中不存在）</option>' + sel.innerHTML;
  }
}
function fillFeatureModelSelects(models, cfg){
  const list = Array.isArray(models) ? models.slice() : [];
  const thOrder = {none:0,minimal:1,low:2,medium:3,high:4,xhigh:5,max:6};
  list.sort((a,b)=>{
    const fa=(a.family_uid||a.family||'').localeCompare(b.family_uid||b.family||'');
    if(fa!==0) return fa;
    const ta = thOrder[String(a.thinking||'').toLowerCase()];
    const tb = thOrder[String(b.thinking||'').toLowerCase()];
    const ta2 = (ta===undefined?50:ta), tb2=(tb===undefined?50:tb);
    if(ta2!==tb2) return ta2-tb2;
    return String(a.id||'').localeCompare(String(b.id||''));
  });
  window.__models = list;
  const def = (cfg && cfg.default_model) || '';
  const pairs = [
    ['deepwiki_model', (cfg&& (cfg.deepwiki_model||cfg.deepwiki_model_resolved)) || def, 'deepwiki_hint', (cfg&&cfg.deepwiki_model_resolved)||def],
    ['codemap_fast_model', (cfg&& (cfg.codemap_fast_model||cfg.codemap_fast_model_resolved||cfg.codemap_model)) || def, 'codemap_fast_hint', (cfg&&cfg.codemap_fast_model_resolved)||def],
    ['codemap_smart_model', (cfg&& (cfg.codemap_smart_model||cfg.codemap_smart_model_resolved||cfg.codemap_model)) || def, 'codemap_smart_hint', (cfg&&cfg.codemap_smart_model_resolved)||def],
    ['command_model', (cfg&& (cfg.command_model||cfg.command_model_resolved)) || def, 'command_hint', (cfg&&cfg.command_model_resolved)||def],
    ['title_model', (cfg&& (cfg.title_model||cfg.title_model_resolved)) || def, 'title_hint', (cfg&&cfg.title_model_resolved)||def],
    ['fast_context_model', (cfg&& (cfg.fast_context_model||cfg.fast_context_model_resolved)) || def, 'fast_context_hint', (cfg&&cfg.fast_context_model_resolved)||def],
  ];
  // 兼容旧页面可能仍有 codemap_model 单选
  if(document.getElementById('codemap_model')){
    pairs.push(['codemap_model', (cfg&& (cfg.codemap_model||cfg.codemap_model_resolved)) || def, 'codemap_hint', (cfg&&cfg.codemap_model_resolved)||def]);
  }
  for(const [selId, cur, hintId, resolved] of pairs){
    fillOneModelSelect(selId, cur||'', list);
    const hint = document.getElementById(hintId);
    if(hint) hint.textContent = '生效: ' + (resolved||cur||def||'-');
  }
}
async function saveFeatureModels(){
  const val = (id)=> ((document.getElementById(id)||{}).value || '').trim();
  const body = {
    deepwiki_model: val('deepwiki_model'),
    codemap_fast_model: val('codemap_fast_model'),
    codemap_smart_model: val('codemap_smart_model'),
    command_model: val('command_model'),
    title_model: val('title_model'),
    fast_context_model: val('fast_context_model'),
    // 兼容：旧 codemap_model 同步为 smart
    codemap_model: val('codemap_smart_model') || val('codemap_model'),
  };
  try{
    await jsend('/api/config','PUT', body);
    const el=document.getElementById('featureModelResult');
    if(el) el.textContent = '已保存';
    toast('功能模型绑定已保存');
    await refreshConfig();
  }catch(e){
    const el=document.getElementById('featureModelResult');
    if(el) el.textContent = e.message||String(e);
    toast(e.message||String(e));
  }
}


async function refreshDesktopPrefs(){
  try{
    const d = await jget('/api/desktop');
    const a=document.getElementById('desktop_autostart'); if(a) a.checked=!!d.autostart;
    const s=document.getElementById('desktop_start_minimized'); if(s) s.checked=!!d.start_minimized;
    const m=document.getElementById('desktop_minimize_to_tray'); if(m) m.checked=!!d.minimize_to_tray;
  }catch(_e){}
}
async function saveDesktopPrefs(){
  const body={
    autostart: !!(document.getElementById('desktop_autostart')||{}).checked,
    start_minimized: !!(document.getElementById('desktop_start_minimized')||{}).checked,
    minimize_to_tray: !!(document.getElementById('desktop_minimize_to_tray')||{}).checked,
  };
  try{
    await jsend('/api/desktop','PUT', body);
    const el=document.getElementById('desktopResult'); if(el) el.textContent='已保存';
    toast('桌面设置已保存');
  }catch(e){
    const el=document.getElementById('desktopResult'); if(el) el.textContent=e.message||String(e);
    toast(e.message||String(e));
  }
}
function hideToTray(){
  if(typeof window.nativeHideToTray==='function'){
    window.nativeHideToTray();
    toast('已隐藏到托盘');
  }else{
    toast('请使用 devin-byok-gui.exe（浏览器模式无托盘）');
  }
}


async function refreshUpdatePrefs(){
  try{
    const c = await jget('/api/config');
    // version
    try{
      const v = await jget('/api/version');
      const el = document.getElementById('updateStatus');
      if(el) el.textContent = '当前版本 v'+(v.version||'?')+(v.build_time?(' · build '+v.build_time):'');
    }catch(_e){}
    // update fields may come from dedicated endpoint later; use /api/status not enough
  }catch(_e){}
  try{
    // load from a lightweight check that includes config: embed in desktop or version
    const st = await jget('/api/update/check');
    // if disabled, still show
    const en = document.getElementById('update_enabled');
    // fetch config raw - extend handleAPIConfig later; for now read check message
  }catch(_e){}
}
async function loadUpdateConfig(){
  try{
    const c = await jget('/api/config');
    // fields added below in config get
  }catch(_e){}
}
async function checkUpdate(){
  const el=document.getElementById('updateResult');
  if(el) el.textContent='检查中…';
  try{
    const r = await jget('/api/update/check');
    const msg = r.message || '';
    let text = '当前 '+(r.current||'?')+' / 最新 '+(r.latest||'?')+' — '+msg;
    if(r.release_url) text += ' · '+r.release_url;
    if(el) el.textContent = text;
    if(r.update_available){ toast('发现新版本 '+(r.latest||'')); }
    else { toast(msg||'已是最新'); }
    return r;
  }catch(e){
    if(el) el.textContent = e.message||String(e);
    toast(e.message||String(e));
  }
}
async function applyUpdate(){
  if(!confirm('将下载新版本并替换当前目录下的 exe，服务会短暂中断。继续？')) return;
  const el=document.getElementById('updateResult');
  if(el) el.textContent='下载更新中…';
  try{
    const r = await jsend('/api/update/apply','POST',{});
    if(el) el.textContent = r.message||JSON.stringify(r);
    toast(r.message||'已调度更新');
  }catch(e){
    if(el) el.textContent = e.message||String(e);
    toast(e.message||String(e));
  }
}
async function saveUpdatePrefs(){
  const body = {
    update_enabled: !!(document.getElementById('update_enabled')||{}).checked,
    update_auto_apply: !!(document.getElementById('update_auto_apply')||{}).checked,
    update_repo: ((document.getElementById('update_repo')||{}).value||'').trim(),
  };
  try{
    await jsend('/api/config','PUT', body);
    toast('更新设置已保存');
  }catch(e){ toast(e.message||String(e)); }
}


// ===== 底栏更新状态 + 弹窗 + 进度 =====
// 与 internal/version.Version 保持一致（硬编码兜底，避免接口未就绪显示 v?）
const APP_VERSION = '1.0.0';
let __lastUpdateCheck = null;
let __updateProgressTimer = null;
let __updateModalForced = false;
let __updateDownloadActive = false;

function setFooterVersion(v){
  const el = document.getElementById('footerVersion');
  if(el) el.textContent = 'v' + (v || APP_VERSION);
}
function setFooterUpdate(text, hasUpdate){
  const el = document.getElementById('footerUpdate');
  if(!el) return;
  el.textContent = text;
  el.classList.toggle('has-update', !!hasUpdate);
}
function showFooterProgress(show){
  const w = document.getElementById('footerProgressWrap');
  if(w) w.hidden = !show;
  if(!show){
    const bar = document.getElementById('footerProgressBar');
    const tx = document.getElementById('footerProgressText');
    if(bar) bar.style.width = '0%';
    if(tx) tx.textContent = '0%';
  }
}
function setFooterProgress(pct, text){
  // 仅下载流程允许显示进度条
  if(!__updateDownloadActive){
    showFooterProgress(false);
    return;
  }
  showFooterProgress(true);
  const bar = document.getElementById('footerProgressBar');
  const tx = document.getElementById('footerProgressText');
  const p = Math.max(0, Math.min(100, Math.round(pct||0)));
  if(bar) bar.style.width = p + '%';
  if(tx) tx.textContent = (text || (p + '%'));
}

function loadFooterVersion(){
  setFooterVersion(APP_VERSION);
  jget('/api/version').then(v=>{
    if(v && v.version){
      setFooterVersion(v.version);
      const sub=document.querySelector('.brand .sub');
      if(sub){ sub.textContent = 'v'+v.version+' · 本地模型接入控制台'; }
    }
  }).catch(()=>{ setFooterVersion(APP_VERSION); });
}

async function runUpdateCheck(opts){
  opts = opts || {};
  // 检查更新绝不打开进度条
  showFooterProgress(false);
  setFooterUpdate('检查更新中…', false);
  try{
    const r = await jget('/api/update/check');
    __lastUpdateCheck = r;
    const cur = (r && r.current) || APP_VERSION;
    const latest = (r && r.latest) || cur;
    setFooterVersion(cur);
    if(!r || !r.ok){
      setFooterUpdate((r && r.message) || '更新检查失败', false);
      return r;
    }
    if(r.update_available){
      setFooterUpdate('有新版本 v' + latest, true);
      if(opts.showModal !== false){
        showUpdateModal(r, !!opts.force);
      }
    }else{
      // 关闭在线更新 / 已最新 等
      if(r.message && /关闭|未配置|失败|HTTP|error/i.test(r.message) && !/最新/.test(r.message)){
        setFooterUpdate(r.message, false);
      }else{
        setFooterUpdate('已是最新 v' + latest, false);
      }
    }
    return r;
  }catch(e){
    setFooterUpdate('更新检查失败: ' + (e.message || String(e)), false);
    showFooterProgress(false);
    return null;
  }
}

function showUpdateModal(r, force){
  const modal = document.getElementById('updateModal');
  if(!modal) return;
  __updateModalForced = !!force;
  const ver = document.getElementById('updateModalVersions');
  const notes = document.getElementById('updateModalNotes');
  if(ver) ver.textContent = '当前 v' + (r.current||APP_VERSION) + '  →  最新 v' + (r.latest||'?');
  if(notes) notes.textContent = r.chinese_notes || r.body || ('发现新版本 v'+(r.latest||'')+'，请更新。');
  const later = document.getElementById('btnUpdateLater');
  if(later) later.hidden = false;
  modal.hidden = false;
}

function dismissUpdateModal(){
  const modal = document.getElementById('updateModal');
  if(modal) modal.hidden = true;
  __updateModalForced = false;
}

async function acceptUpdateAndDownload(){
  dismissUpdateModal();
  __updateDownloadActive = true;
  setFooterUpdate('正在下载更新…', true);
  setFooterProgress(0, '准备下载');
  if(__updateProgressTimer) clearInterval(__updateProgressTimer);
  __updateProgressTimer = setInterval(pollUpdateProgress, 400);
  try{
    const r = await jsend('/api/update/apply','POST',{});
    await pollUpdateProgress();
    if(r && r.ok){
      const msg = r.message || '';
      // 无需更新时不显示进度、不退出
      if(/无需更新|已是最新|不用更新/.test(msg)){
        __updateDownloadActive = false;
        showFooterProgress(false);
        setFooterUpdate('已是最新 v' + APP_VERSION, false);
        toast(msg);
        return r;
      }
      setFooterProgress(100, '即将重启…');
      toast(msg || '即将安装更新并重启');
      if(typeof window.nativeQuitForce === 'function'){
        setTimeout(()=>window.nativeQuitForce(), 300);
      }else if(typeof window.nativeQuit === 'function'){
        setTimeout(()=>window.nativeQuit(), 500);
      }else{
        setTimeout(()=>{ toast('请关闭窗口以完成文件替换'); }, 800);
      }
    }else{
      __updateDownloadActive = false;
      setFooterUpdate('更新失败', true);
      showFooterProgress(false);
      toast((r && r.message) || '更新失败');
    }
  }catch(e){
    __updateDownloadActive = false;
    setFooterUpdate('更新失败', true);
    showFooterProgress(false);
    toast(e.message || String(e));
  }finally{
    if(__updateProgressTimer){ clearInterval(__updateProgressTimer); __updateProgressTimer = null; }
  }
}

async function pollUpdateProgress(){
  try{
    const p = await jget('/api/update/progress');
    if(!p) return;
    const phase = p.phase || 'idle';
    // 非下载流程：强制隐藏进度条
    if(!__updateDownloadActive){
      showFooterProgress(false);
      return;
    }
    if(phase === 'idle' || phase === 'checking'){
      // 检查阶段不展示进度条
      return;
    }
    if(phase === 'error'){
      setFooterProgress(p.percent||0, p.message||'错误');
      return;
    }
    if(phase === 'downloading' || phase === 'verifying' || phase === 'scheduling' || phase === 'done'){
      setFooterProgress(p.percent||0, p.message || ((Math.round(p.percent||0))+'%'));
      return;
    }
    showFooterProgress(false);
  }catch(_e){}
}

function startUpdateAutoCheck(){
  setFooterUpdate('可在设置中手动检查更新', false);
}

// 覆盖旧的 checkUpdate / applyUpdate，复用底栏逻辑
async function checkUpdate(){
  const el=document.getElementById('updateResult');
  if(el) el.textContent='检查中…';
  const r = await runUpdateCheck({showModal:true, force:false});
  if(el && r){
    el.textContent = (r.message||'') + (r.update_available ? '（可更新）' : '');
  }
  return r;
}
async function applyUpdate(){
  return acceptUpdateAndDownload();
}



// ===== 系统提示词 + 扩展管理 =====
window.__prompts = {};

async function refreshPrompts(){
  const box = document.getElementById('promptList');
  const st = document.getElementById('extStatus');
  if(st) st.textContent = '加载中…';
  try{
    const j = await jget('/api/prompts');
    const list = (j && j.prompts) ? j.prompts : [];
    window.__prompts = {};
    if(box){
      if(!list.length){
        box.innerHTML = '<div class="muted">暂无自定义提示词（固定内置工具提示仍会注入）</div>';
      }else{
        box.innerHTML = list.map(p => {
          window.__prompts[p.id] = p;
          const en = !!p.enabled;
          return `<div class="card model-card">
            <div class="card-h">${escapeHtml(p.title||'(无标题)')}
 <span class="muted small">${en?'启用':'禁用'} · ${escapeHtml(p.mode||'append')} · ${escapeHtml(p.scope||'global')}</span></div>
            <pre class="muted small" style="white-space:pre-wrap;max-height:120px;overflow:auto">${escapeHtml(p.body||'')}</pre>
            <div class="row gap">
              <button class="btn btn-secondary" type="button" data-pid="${escapeHtml(p.id)}" data-act="edit">编辑</button>
              <button class="btn btn-secondary" type="button" data-pid="${escapeHtml(p.id)}" data-act="toggle">${en?'禁用':'启用'}</button>
              <button class="btn btn-secondary" type="button" data-pid="${escapeHtml(p.id)}" data-act="del">删除</button>
            </div>
          </div>`;
        }).join('');
        box.querySelectorAll('button[data-act]').forEach(btn => {
          btn.addEventListener('click', () => {
            const id = btn.getAttribute('data-pid');
            const act = btn.getAttribute('data-act');
            if(act==='edit') editPromptById(id);
            else if(act==='toggle') togglePrompt(id);
            else if(act==='del') deletePrompt(id);
          });
        });
      }
    }
    // 扩展状态（失败不阻断列表）
    try{
      const ej = await jget('/api/extension');
      if(st){
        const inst = ej && ej.installed;
        const dis = ej && ej.disabled;
        let line = (inst ? '已安装' : '未安装') + ' · ' + (ej.id || '');
        if(dis) line += ' · 已禁用(.obsolete)';
        else if(inst) line += ' · 已启用';
        if(ej.dir) line += ' @ ' + ej.dir;
        if(ej.folder) line += ' / ' + ej.folder;
        st.textContent = line;
      }
    }catch(ex){
      if(st) st.textContent = '扩展状态获取失败: ' + (ex.message || ex);
    }
  }catch(e){
    if(st) st.textContent = '提示词 API 失败: ' + (e.message || e);
    if(box) box.innerHTML = '<div class="muted">加载失败，请确认服务已启动</div>';
    toast('提示词加载失败: ' + (e.message || e));
  }
}

function openPromptEditor(p){
  const title = window.prompt('标题', (p && p.title) || '');
  if(title === null) return;
  const mode = window.prompt('模式 append | prepend | replace', (p && p.mode) || 'append');
  if(mode === null) return;
  const body = window.prompt('正文', (p && p.body) || '');
  if(body === null) return;
  const scope = window.prompt('范围 global | model | route | task（留空表示全局）', (p && p.scope) || '');
  if(scope === null) return;
  const routes = window.prompt('路由，逗号分隔：chat,fast_context,deepwiki,codemap', ((p && p.routes)||[]).join(','));
  if(routes === null) return;
  const models = window.prompt('模型 ID 或 family UID，逗号分隔（留空不限）', ((p && p.models)||[]).join(','));
  if(models === null) return;
  const tasks = window.prompt('任务，逗号分隔：coding,debug,research,review,explain,general', ((p && p.tasks)||[]).join(','));
  if(tasks === null) return;
  const priority = window.prompt('优先级数字，越小越先注入', String((p && p.priority) || 50));
  if(priority === null) return;
  const enabled = window.confirm('是否启用该提示词？');
  savePrompt({
    id: (p && p.id) || '',
    title: title,
    mode: (mode || 'append').trim(),
    body: body,
    enabled: !!enabled,
    scope: (scope || '').trim(),
    routes: (routes || '').split(',').map(x=>x.trim()).filter(Boolean),
    models: (models || '').split(',').map(x=>x.trim()).filter(Boolean),
    tasks: (tasks || '').split(',').map(x=>x.trim()).filter(Boolean),
    priority: Number(priority || 50)
  });
}
function editPrompt(p){ openPromptEditor(p); }
function editPromptById(id){ editPrompt(window.__prompts && window.__prompts[id]); }

async function savePrompt(p){
  try{
    const j = await jsend('/api/prompts', 'POST', p);
    if(j && j.ok === false){ toast(j.message || '保存失败'); return; }
    toast('已保存');
    await refreshPrompts();
  }catch(e){
    toast('保存失败: ' + (e.message || e));
  }
}

async function togglePrompt(id){
  const p = window.__prompts && window.__prompts[id];
  if(!p){ toast('未找到提示词'); return; }
  const next = Object.assign({}, p, { enabled: !p.enabled });
  await savePrompt(next);
}

async function deletePrompt(id){
  if(!window.confirm('删除该提示词？')) return;
  try{
    const j = await jsend('/api/prompts?id=' + encodeURIComponent(id), 'DELETE');
    if(j && j.ok === false){ toast(j.message || '删除失败'); return; }
    toast('已删除');
    await refreshPrompts();
  }catch(e){
    toast('删除失败: ' + (e.message || e));
  }
}

async function extAction(action){
  const st = document.getElementById('extStatus');
  if(st) st.textContent = '执行中: ' + action + '…';
  try{
    const j = await jsend('/api/extension?action=' + encodeURIComponent(action), 'POST');
    if(j && j.ok === false){
      toast(j.message || (action + ' 失败'));
    }else{
      toast('扩展 ' + action + ' 成功' + (j.path ? (' → ' + j.path) : ''));
    }
    await refreshPrompts();
  }catch(e){
    if(st) st.textContent = '扩展操作失败: ' + (e.message || e);
    toast('扩展操作失败: ' + (e.message || e));
  }
}

async function refreshAll(){ await Promise.all([refreshMetrics(), refreshStatus(), refreshConfig(), refreshLocalAccount()]); if(currentPage==='models') refreshFamilies(); }
async function loadVersion(){
  loadFooterVersion();
}
// 启动：硬编码版本 → 拉接口覆盖 → 自动检查更新
loadFooterVersion();
refreshAll();
startUpdateAutoCheck();
setInterval(refreshMetrics, 2000);
['f_label','f_upstream'].forEach(id=>{ const el=document.getElementById(id); if(el){ el.addEventListener('input', refreshUidHint); }});
