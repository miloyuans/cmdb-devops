const $ = id => document.getElementById(id);
let token = localStorage.getItem('token') || '';
let me = null;
let accounts = [];
let selectedAccount = null;
let users = [];
let settings = null;

const pageTitles = {
  dashboard: ['总览','云资源与身份密钥缓存状态'],
  query: ['查询','IP / CIDR 与 AK 统一查询入口'],
  connectivity: ['通信分析','基于缓存资源、VPC、子网和安全组规则分析通信可达性'],
  accounts: ['云账户','AWS / 阿里云账户、地区发现和资源同步'],
  telegram: ['Telegram','Bot、群、允许交互用户的配置管理'],
  jobs: ['异步任务','地区发现、资源同步、身份同步和 miss refresh 状态'],
  audit: ['审计事件','登录、配置、任务触发等操作记录'],
  users: ['用户管理','平台普通用户和管理用户'],
  settings: ['系统设置','页面名称、异步时间、Mongo 配置展示']
};
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
function fmt(v){ if(!v) return ''; try{return new Date(v).toLocaleString();}catch(e){return v;} }
function bool(v){ return v === true || v === 'true' || v === 1 || v === '1'; }
function esc(s){ return String(s ?? '').replace(/[&<>"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c])); }
function badge(text, cls=''){ return `<span class="badge ${cls}">${esc(text)}</span>`; }

function logout(){ localStorage.removeItem('token'); token=''; me=null; $('loginView').style.display='flex'; $('app').style.display='none'; }
async function login(){
  try{
    const data = await api('/api/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:$('loginUser').value,password:$('loginPass').value})});
    token=data.token; localStorage.setItem('token',token); await init();
  }catch(e){ alert(e.message); }
}
async function init(){
  if(!token){ logout(); return; }
  $('loginView').style.display='none'; $('app').style.display='flex';
  try{ me=await api('/api/me'); }catch(e){ logout(); return; }
  $('meBadge').innerText = `${me.username} / ${me.role === 'admin' ? '管理用户' : '普通用户'}`;
  await Promise.allSettled([loadSettings(), loadAccounts(), loadKeys(), loadIdentityUsers(), loadJobs(), loadUsers(), loadTelegramAll(), loadAudit()]);
  showPage('dashboard');
}
function showPage(id){
  document.querySelectorAll('.page').forEach(p=>p.style.display='none');
  $(id).style.display='block';
  document.querySelectorAll('.nav-item').forEach(b=>b.classList.toggle('active', b.dataset.page===id));
  const t=pageTitles[id] || [id,'']; $('pageTitle').innerText=t[0]; $('pageDesc').innerText=t[1];
  if(id==='accounts') loadAccounts();
  if(id==='jobs') loadJobs();
  if(id==='audit') loadAudit();
  if(id==='users') loadUsers();
  if(id==='settings') loadSettings();
  if(id==='telegram') loadTelegramAll();
}
function switchTab(scope, panel){
  const root = $(scope);
  root.querySelectorAll('.subpanel').forEach(p=>p.style.display='none');
  root.querySelectorAll('.tab').forEach(t=>t.classList.remove('active'));
  $(panel).style.display='block';
  event.target.classList.add('active');
}

async function loadSettings(){
  try{
    settings = await api('/api/settings');
    if(!settings) return;
    $('siteBrand').innerText=settings.site_name || 'CMDB DevOps';
    document.title=settings.site_name || 'CMDB DevOps';
    $('setSiteName').value=settings.site_name||''; $('setPublicBase').value=settings.public_base_url||'';
    $('setInventory').value=settings.inventory_interval_seconds||1800; $('setRegion').value=settings.region_check_interval_seconds||86400; $('setIdentity').value=settings.identity_interval_seconds||21600; $('setMiss').value=settings.miss_refresh_debounce_seconds||300;
    $('setMongoURI').value=settings.mongo_uri||''; $('setMongoDB').value=settings.mongo_admin_db||'';
  }catch(e){}
}
async function saveSettings(){
  const body={id:'default',site_name:$('setSiteName').value,public_base_url:$('setPublicBase').value,inventory_interval_seconds:Number($('setInventory').value||1800),region_check_interval_seconds:Number($('setRegion').value||86400),identity_interval_seconds:Number($('setIdentity').value||21600),miss_refresh_debounce_seconds:Number($('setMiss').value||300),mongo_uri:$('setMongoURI').value,mongo_admin_db:$('setMongoDB').value};
  settings = await api('/api/settings',{method:'PUT',body:JSON.stringify(body)}); await loadSettings(); alert('系统设置已保存');
}

async function loadAccounts(){
  accounts=await api('/api/accounts'); $('mAccounts').innerText=accounts.length;
  const rows=accounts.map(a=>`<tr class="clickable ${selectedAccount&&selectedAccount.id===a.id?'selected':''}" onclick="selectAccount('${a.id}')"><td>${badge(a.provider.toUpperCase())}</td><td><b>${esc(a.alias)}</b><small>${esc(a.account_id)}</small></td><td>${esc(a.region_mode)}</td><td>${(a.effective_regions||[]).map(r=>badge(r,'soft')).join('')}</td><td>${a.enabled?badge('启用','ok'):badge('禁用','muted')}</td><td>${fmt(a.last_sync_at)||'-'}</td></tr>`).join('');
  $('accountsTable').innerHTML='<tr><th>平台</th><th>账户</th><th>地区</th><th>有效地区</th><th>状态</th><th>最近同步</th></tr>'+rows;
  if(!selectedAccount && accounts.length) selectAccount(accounts[0].id, false);
}
function newAccount(){ selectedAccount=null; $('accFormTitle').innerText='新增云账户'; ['accFormID','accAlias','accID','accAK','accSK','accRegions'].forEach(id=>$(id).value=''); $('accProvider').value='aws'; $('accEnabled').value='true'; $('accRegionMode').value='auto'; $('accDetails').innerHTML=''; }
function selectAccount(id, redraw=true){
  selectedAccount=accounts.find(a=>a.id===id); if(!selectedAccount) return;
  $('accFormTitle').innerText=`账户详情：${selectedAccount.alias}`; $('accFormID').value=selectedAccount.id; $('accProvider').value=selectedAccount.provider; $('accAlias').value=selectedAccount.alias; $('accID').value=selectedAccount.account_id||''; $('accAK').value=selectedAccount.credential?.access_key_id||''; $('accSK').value=''; $('accEnabled').value=String(!!selectedAccount.enabled); $('accRegionMode').value=selectedAccount.region_mode||'auto'; $('accRegions').value=(selectedAccount.selected_regions||[]).join(',');
  $('accDetails').innerHTML=`<b>已发现地区</b><div class="chips">${(selectedAccount.detected_regions||[]).map(r=>badge(`${r.region}:${r.has_resource?'有资源':'无'}`, r.has_resource?'ok':'muted')).join('')||'<span class="hint">暂无检测结果</span>'}</div><b>数据库</b><p>${esc('cmdb_'+selectedAccount.provider+'_'+selectedAccount.alias.replace(/[^a-zA-Z0-9]/g,'_'))}</p>`;
  if(redraw) loadAccounts();
}
async function saveAccount(){
  const regions=$('accRegions').value.split(',').map(s=>s.trim()).filter(Boolean);
  const id=$('accFormID').value;
  const body={id,provider:$('accProvider').value,alias:$('accAlias').value,account_id:$('accID').value,access_key_id:$('accAK').value,access_key_secret:$('accSK').value,enabled:bool($('accEnabled').value),region_mode:$('accRegionMode').value,selected_regions:regions};
  const path=id?`/api/accounts/${encodeURIComponent(id)}`:'/api/accounts';
  await api(path,{method:id?'PUT':'POST',body:JSON.stringify(body)}); $('accSK').value=''; selectedAccount=null; await loadAccounts(); alert('保存成功');
}
async function triggerSelectedAccountJob(type){ if(!selectedAccount){alert('请先选择账户');return;} await triggerJob(selectedAccount.id,type); }
async function triggerJob(id,type){
  try{ const r=await api(`/api/accounts/${encodeURIComponent(id)}/jobs/${type}`,{method:'POST'}); alert('已触发：'+r.job_id); }catch(e){ alert(e.message); }
  await loadJobs();
}

async function queryIP(){
  try{
    const res=await api('/api/query/ip',{method:'POST',body:JSON.stringify({query:$('ipQuery').value})});
    $('ipSummary').innerHTML=`${badge(res.cache_hit?'缓存命中':'缓存未命中',res.cache_hit?'ok':'warn')} ${badge(`命中 ${res.matches?.length||0} 条`)} ${badge(res.query?.ip_type||'')}`;
    $('ipResult').innerText=pretty(res);
  }catch(e){ $('ipResult').innerText=e.message; $('ipSummary').innerHTML=badge('查询失败','danger'); }
}
async function lookupAK(){
  try{ const res=await api('/api/identity/access-keys/lookup',{method:'POST',body:JSON.stringify({access_key_id:$('akLookup').value})}); $('akResult').innerText=pretty(res); }catch(e){ $('akResult').innerText=e.message; }
}
async function loadKeys(){
  const rows=await api('/api/identity/access-keys'); $('mKeys').innerText=rows.length;
  $('keysTable').innerHTML='<tr><th>云</th><th>账户</th><th>AK</th><th>用户</th><th>状态</th><th>创建</th><th>最近使用</th><th>服务/地区</th></tr>'+rows.map(k=>`<tr><td>${badge(k.provider)}</td><td>${esc(k.account_alias)}</td><td><code>${esc(k.access_key_id_masked)}</code></td><td>${esc(k.owner_user_name)}</td><td>${k.enabled?badge(k.status,'ok'):badge(k.status,'muted')}</td><td>${fmt(k.create_date)}</td><td>${fmt(k.last_used_date)||'从未使用/未获取'}</td><td>${esc(k.last_used_service||'')} ${esc(k.last_used_region||'')}</td></tr>`).join('');
}
async function loadIdentityUsers(){
  const rows=await api('/api/identity/users');
  $('iamUsersTable').innerHTML='<tr><th>云</th><th>账户</th><th>用户</th><th>类型</th><th>状态</th><th>组</th><th>策略</th><th>创建</th></tr>'+rows.map(u=>`<tr><td>${badge(u.provider)}</td><td>${esc(u.account_alias)}</td><td>${esc(u.user_name)}</td><td>${esc(u.user_type)}</td><td>${u.enabled?badge('启用','ok'):badge('禁用','muted')}</td><td>${(u.groups||[]).map(g=>badge(g,'soft')).join('')}</td><td>${(u.attached_policies||[]).map(p=>badge(p.policy_name,'soft')).join('')}</td><td>${fmt(u.create_date)}</td></tr>`).join('');
}
async function analyzeConn(){
  try{
    const res=await api('/api/query/connectivity',{method:'POST',body:JSON.stringify({source_ip:$('srcIP').value,target_ip:$('dstIP').value,protocol:$('proto').value,port:Number($('port').value||443)})});
    const cls=res.result==='allowed'?'ok':res.result==='blocked'?'danger':'warn'; $('connSummary').innerHTML=`${badge(res.result||'unknown',cls)} ${badge(res.network_status||'')} ${badge(res.security_group_status||'')}`; $('connResult').innerText=pretty(res);
  }catch(e){ $('connResult').innerText=e.message; $('connSummary').innerHTML=badge('分析失败','danger'); }
}

async function loadTelegramAll(){ await Promise.allSettled([loadBots(),loadChats(),loadTelegramUsers()]); }
function clearBotForm(){ ['botID','botToken','botTokenEnv','botWebhook'].forEach(id=>$(id).value=''); $('botName').value='CMDB DevOps Bot'; $('botEnabled').value='true'; $('botDefault').value='true'; $('botMode').value='webhook'; $('botParse').value='PlainText'; }
async function loadBots(){
  const rows=await api('/api/telegram/bots');
  $('botsTable').innerHTML='<tr><th>名称</th><th>状态</th><th>默认</th><th>模式</th><th>Token 来源</th><th>Webhook</th><th>操作</th></tr>'+rows.map(b=>`<tr><td>${esc(b.name)}</td><td>${b.enabled?badge('启用','ok'):badge('禁用','muted')}</td><td>${b.is_default?badge('默认','ok'):''}</td><td>${esc(b.mode)}</td><td>${esc(b.token_env||'encrypted')}</td><td>${esc(b.webhook_url||'')}</td><td><button class="mini" onclick='editBot(${JSON.stringify(b)})'>编辑</button></td></tr>`).join('');
}
function editBot(b){ $('botID').value=b.id; $('botName').value=b.name||''; $('botEnabled').value=String(!!b.enabled); $('botDefault').value=String(!!b.is_default); $('botMode').value=b.mode||'webhook'; $('botToken').value=''; $('botTokenEnv').value=b.token_env||''; $('botWebhook').value=b.webhook_url||''; $('botParse').value=b.parse_mode||'PlainText'; }
async function saveBot(){ const body={id:$('botID').value,name:$('botName').value,enabled:bool($('botEnabled').value),is_default:bool($('botDefault').value),mode:$('botMode').value,bot_token:$('botToken').value,token_env:$('botTokenEnv').value,webhook_url:$('botWebhook').value,parse_mode:$('botParse').value,rate_limit_per_second:20,max_workers:4}; await api(body.id?`/api/telegram/bots/${body.id}`:'/api/telegram/bots',{method:body.id?'PUT':'POST',body:JSON.stringify(body)}); clearBotForm(); await loadBots(); }
function clearChatForm(){ ['chatFormID','chatID','chatTitle'].forEach(id=>$(id).value=''); $('chatEnabled').value='true'; $('chatAllowQuery').value='true'; $('chatAllowNotify').value='true'; $('chatMax').value='10'; }
async function loadChats(){ const rows=await api('/api/telegram/chats'); $('chatsTable').innerHTML='<tr><th>Chat</th><th>名称</th><th>状态</th><th>查询</th><th>通知</th><th>最大</th><th>操作</th></tr>'+rows.map(c=>`<tr><td><code>${c.chat_id}</code></td><td>${esc(c.chat_title)}</td><td>${c.enabled?badge('启用','ok'):badge('禁用','muted')}</td><td>${c.allow_query?'是':'否'}</td><td>${c.allow_notification?'是':'否'}</td><td>${c.max_result_count||10}</td><td><button class="mini" onclick='editChat(${JSON.stringify(c)})'>编辑</button></td></tr>`).join(''); }
function editChat(c){ $('chatFormID').value=c.id; $('chatID').value=c.chat_id; $('chatTitle').value=c.chat_title||''; $('chatEnabled').value=String(!!c.enabled); $('chatAllowQuery').value=String(!!c.allow_query); $('chatAllowNotify').value=String(!!c.allow_notification); $('chatMax').value=c.max_result_count||10; }
async function saveChat(){ const body={id:$('chatFormID').value,chat_id:Number($('chatID').value),chat_title:$('chatTitle').value,chat_type:'group',enabled:bool($('chatEnabled').value),allow_query:bool($('chatAllowQuery').value),allow_notification:bool($('chatAllowNotify').value),default_language:'zh-CN',max_result_count:Number($('chatMax').value||10)}; await api(body.id?`/api/telegram/chats/${body.id}`:'/api/telegram/chats',{method:body.id?'PUT':'POST',body:JSON.stringify(body)}); clearChatForm(); await loadChats(); }
function clearTelegramUserForm(){ ['tgUserFormID','tgUserID','tgUsername','tgDisplayName','tgUserNote'].forEach(id=>$(id).value=''); $('tgUserEnabled').value='true'; }
async function loadTelegramUsers(){ const rows=await api('/api/telegram/users'); $('telegramUsersTable').innerHTML='<tr><th>User ID</th><th>Username</th><th>显示名</th><th>状态</th><th>备注</th><th>操作</th></tr>'+rows.map(u=>`<tr><td><code>${u.telegram_user_id}</code></td><td>${esc(u.username)}</td><td>${esc(u.display_name)}</td><td>${u.enabled?badge('启用','ok'):badge('禁用','muted')}</td><td>${esc(u.note)}</td><td><button class="mini" onclick='editTelegramUser(${JSON.stringify(u)})'>编辑</button></td></tr>`).join(''); }
function editTelegramUser(u){ $('tgUserFormID').value=u.id; $('tgUserID').value=u.telegram_user_id; $('tgUsername').value=u.username||''; $('tgDisplayName').value=u.display_name||''; $('tgUserEnabled').value=String(!!u.enabled); $('tgUserNote').value=u.note||''; }
async function saveTelegramUser(){ const body={id:$('tgUserFormID').value,telegram_user_id:Number($('tgUserID').value),username:$('tgUsername').value,display_name:$('tgDisplayName').value,enabled:bool($('tgUserEnabled').value),note:$('tgUserNote').value}; await api(body.id?`/api/telegram/users/${body.id}`:'/api/telegram/users',{method:body.id?'PUT':'POST',body:JSON.stringify(body)}); clearTelegramUserForm(); await loadTelegramUsers(); }

async function loadJobs(){ const rows=await api('/api/jobs'); $('mJobs').innerText=rows.length; $('jobsTable').innerHTML='<tr><th>类型</th><th>目标</th><th>状态</th><th>触发</th><th>开始</th><th>结束</th><th>消息</th></tr>'+rows.map(j=>`<tr><td>${esc(j.job_type)}</td><td>${esc(j.account_id||j.provider||'')}</td><td>${badge(j.status,j.status==='success'?'ok':j.status==='failed'?'danger':'warn')}</td><td>${esc(j.trigger_type)}</td><td>${fmt(j.started_at)}</td><td>${fmt(j.finished_at)}</td><td>${esc(j.message||'')}</td></tr>`).join(''); }
async function loadAudit(){ const rows=await api('/api/audit'); $('auditTable').innerHTML='<tr><th>时间</th><th>用户</th><th>操作</th><th>目标</th><th>详情</th></tr>'+rows.map(a=>`<tr><td>${fmt(a.created_at)}</td><td>${esc(a.actor)}</td><td>${esc(a.action)}</td><td>${esc(a.target)}</td><td><code>${esc(JSON.stringify(a.meta||{}))}</code></td></tr>`).join(''); }

async function loadUsers(){ users=await api('/api/users'); $('mUsers').innerText=users.length; $('usersTable').innerHTML='<tr><th>用户名</th><th>角色</th><th>状态</th><th>Telegram</th><th>操作</th></tr>'+users.map(u=>`<tr><td>${esc(u.username)}</td><td>${u.role==='admin'?badge('管理用户','ok'):badge('普通用户','soft')}</td><td>${u.enabled?badge('启用','ok'):badge('禁用','muted')}</td><td>${u.telegram_user_id||''}</td><td><button class="mini" onclick='editUser(${JSON.stringify(u)})'>编辑</button></td></tr>`).join(''); }
function newUser(){ ['userID','username','userTelegramID','userPassword'].forEach(id=>$(id).value=''); $('userRole').value='viewer'; $('userEnabled').value='true'; }
function editUser(u){ $('userID').value=u.id; $('username').value=u.username; $('userRole').value=u.role; $('userEnabled').value=String(!!u.enabled); $('userTelegramID').value=u.telegram_user_id||''; $('userPassword').value=''; }
async function saveUser(){ const id=$('userID').value; const body={id,username:$('username').value,role:$('userRole').value,enabled:bool($('userEnabled').value),telegram_user_id:Number($('userTelegramID').value||0),password:$('userPassword').value}; await api(id?`/api/users/${id}`:'/api/users',{method:id?'PUT':'POST',body:JSON.stringify(body)}); newUser(); await loadUsers(); }

function showProfile(){ if(!me) return; $('profileText').innerText=`${me.username} / ${me.role==='admin'?'管理用户':'普通用户'}`; $('profileTelegram').value=me.telegram_user_id||''; $('profilePassword').value=''; $('profileModal').style.display='flex'; }
function hideProfile(){ $('profileModal').style.display='none'; }
async function saveProfile(){ await api('/api/me/profile',{method:'PUT',body:JSON.stringify({telegram_user_id:Number($('profileTelegram').value||0),password:$('profilePassword').value})}); hideProfile(); me=await api('/api/me'); $('meBadge').innerText=`${me.username} / ${me.role === 'admin' ? '管理用户' : '普通用户'}`; alert('已保存'); }

init();
