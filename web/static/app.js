const $ = id => document.getElementById(id);
let token = localStorage.getItem('token') || '';

function authHeaders(){ return {'Content-Type':'application/json','Authorization':'Bearer '+token}; }
async function api(path, opts={}){
  opts.headers = opts.headers || authHeaders();
  const res = await fetch(path, opts);
  if(res.status===401){ logout(); throw new Error('unauthorized'); }
  const data = await res.json().catch(()=>({}));
  if(!res.ok) throw new Error(data.error || res.statusText);
  return data;
}
function pretty(x){ return JSON.stringify(x,null,2); }
function logout(){ localStorage.removeItem('token'); token=''; $('loginPage').style.display='block'; $('content').style.display='none'; }
async function login(){
  const data = await api('/api/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:$('loginUser').value,password:$('loginPass').value})});
  token=data.token; localStorage.setItem('token',token); await init();
}
async function init(){
  if(!token){ logout(); return; }
  $('loginPage').style.display='none'; $('content').style.display='block';
  try{ const me=await api('/api/me'); $('me').innerText=me.username+' / '+me.role; }catch(e){ logout(); return; }
  await Promise.allSettled([loadAccounts(), loadKeys(), loadJobs(), loadTelegram()]);
  showPage('dashboard');
}
function showPage(id){
  document.querySelectorAll('.page').forEach(p=>p.style.display='none');
  $(id).style.display='block';
  $('pageTitle').innerText = {dashboard:'仪表盘',ip:'IP 查询',connectivity:'通信分析',accounts:'云账户管理',identity:'身份与密钥',telegram:'Telegram 管理',jobs:'同步任务'}[id] || id;
}
async function loadAccounts(){
  const rows=await api('/api/accounts'); $('mAccounts').innerText=rows.length;
  $('accountsTable').innerHTML='<tr><th>云</th><th>别名</th><th>账户ID</th><th>地区模式</th><th>有效地区</th><th>状态</th><th>操作</th></tr>'+rows.map(a=>`<tr><td>${a.provider}</td><td>${a.alias}</td><td>${a.account_id}</td><td>${a.region_mode}</td><td>${(a.effective_regions||[]).join(', ')}</td><td>${a.enabled?'启用':'禁用'}</td><td><button onclick="triggerJob('${a.id}','region_discovery')">检测地区</button> <button onclick="triggerJob('${a.id}','inventory_sync')">同步资源</button> <button onclick="triggerJob('${a.id}','identity_sync')">同步身份</button></td></tr>`).join('');
}
async function saveAccount(){
  const regions=$('accRegions').value.split(',').map(s=>s.trim()).filter(Boolean);
  await api('/api/accounts',{method:'POST',body:JSON.stringify({provider:$('accProvider').value,alias:$('accAlias').value,account_id:$('accID').value,access_key_id:$('accAK').value,access_key_secret:$('accSK').value,enabled:true,region_mode:$('accRegionMode').value,selected_regions:regions})});
  $('accSK').value=''; await loadAccounts(); alert('保存成功');
}
async function triggerJob(id,type){
  try{ const r=await api(`/api/accounts/${id}/jobs/${type}`,{method:'POST'}); alert('已触发：'+r.job_id); }catch(e){ alert(e.message); }
  await loadJobs();
}
async function queryIP(){
  try{ $('ipResult').innerText=pretty(await api('/api/query/ip',{method:'POST',body:JSON.stringify({query:$('ipQuery').value})})); }catch(e){ $('ipResult').innerText=e.message; }
}
async function analyzeConn(){
  try{ $('connResult').innerText=pretty(await api('/api/query/connectivity',{method:'POST',body:JSON.stringify({source_ip:$('srcIP').value,target_ip:$('dstIP').value,protocol:'tcp',port:Number($('port').value||443)})})); }catch(e){ $('connResult').innerText=e.message; }
}
async function lookupAK(){
  try{ $('akResult').innerText=pretty(await api('/api/identity/access-keys/lookup',{method:'POST',body:JSON.stringify({access_key_id:$('akLookup').value})})); }catch(e){ $('akResult').innerText=e.message; }
}
async function loadKeys(){
  const rows=await api('/api/identity/access-keys'); $('mKeys').innerText=rows.length;
  $('keysTable').innerHTML='<tr><th>云</th><th>账户</th><th>AK</th><th>用户</th><th>状态</th><th>创建</th><th>最近使用</th><th>服务</th></tr>'+rows.map(k=>`<tr><td>${k.provider}</td><td>${k.account_alias}</td><td>${k.access_key_id_masked}</td><td>${k.owner_user_name}</td><td>${k.status}</td><td>${fmt(k.create_date)}</td><td>${fmt(k.last_used_date)}</td><td>${k.last_used_service||''}</td></tr>`).join('');
}
async function loadJobs(){
  const rows=await api('/api/jobs'); $('mJobs').innerText=rows.length;
  $('jobsTable').innerHTML='<tr><th>类型</th><th>账户</th><th>状态</th><th>触发</th><th>开始</th><th>结束</th><th>消息</th></tr>'+rows.map(j=>`<tr><td>${j.job_type}</td><td>${j.account_id||j.provider||''}</td><td><span class="pill">${j.status}</span></td><td>${j.trigger_type}</td><td>${fmt(j.started_at)}</td><td>${fmt(j.finished_at)}</td><td>${j.message||''}</td></tr>`).join('');
}
async function loadTelegram(){
  try{
    const cfg=await api('/api/telegram/config'); if(!cfg) return;
    $('tgEnabled').checked=!!cfg.enabled; $('tgMode').value=cfg.mode||'webhook'; $('tgBotName').value=cfg.bot_name||''; $('tgTokenEnv').value=cfg.bot_token_env||''; $('tgWebhook').value=cfg.webhook_url||''; $('tgParse').value=cfg.parse_mode||'PlainText';
  }catch(e){}
}
async function saveTelegram(){
  const body={enabled:$('tgEnabled').checked,mode:$('tgMode').value,bot_name:$('tgBotName').value,bot_token:$('tgToken').value,bot_token_env:$('tgTokenEnv').value,webhook_url:$('tgWebhook').value,parse_mode:$('tgParse').value,rate_limit_per_second:20,max_workers:4};
  const r=await api('/api/telegram/config',{method:'PUT',body:JSON.stringify(body)}); $('tgToken').value=''; $('telegramResult').innerText=pretty(r);
}
async function saveChat(){
  const body={chat_id:Number($('chatID').value),chat_title:$('chatTitle').value,chat_type:'group',enabled:true,allow_query:true,allow_notification:true,default_language:'zh-CN',max_result_count:10};
  const r=await api('/api/telegram/chats',{method:'POST',body:JSON.stringify(body)}); $('telegramResult').innerText=pretty(r);
}
function fmt(v){ if(!v) return ''; try{return new Date(v).toLocaleString();}catch(e){return v;} }
init();
