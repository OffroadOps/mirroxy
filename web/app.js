'use strict';
const $=id=>document.getElementById(id);
const escapeHTML=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const t=(key,params)=>window.MirroxyI18n?.t(key,params)||key;
let state=null,token='',importMode='v2rayn',pending=0,settingsDirty=false,remoteDirty=false,proxyDirty=false,routingDirty=false,appRoutingDirty=false,appRoutingDraft=null,offline=false,toastTimer;
const latest=new Map();
function translateEvent(value){const match=String(value).match(/^(\d{2}:\d{2}:\d{2}\s+)(.*)$/);return match?match[1]+t(match[2]):t(value)}
function toast(message,error=false){clearTimeout(toastTimer);$('toast').textContent=message;$('toast').className='toast'+(error?' error':'');toastTimer=setTimeout(()=>$('toast').classList.add('hidden'),error?9500:4500);}
async function copyText(text){if(navigator.clipboard?.writeText)return navigator.clipboard.writeText(text);const input=document.createElement('textarea');input.value=text;(document.querySelector('dialog[open]')||document.body).append(input);input.select();const ok=document.execCommand('copy');input.remove();if(!ok)throw Error(t('请手动复制：')+text);}
async function api(path,body){const response=await fetch('/api/'+path,{method:'POST',headers:{'Content-Type':'application/json','X-App-Token':token},body:JSON.stringify(body)});let result;try{result=await response.json()}catch{throw new Error(t('后台响应异常，请刷新页面'))};if(!response.ok||result.error)throw new Error(t(result.error||'操作未完成'));return result;}
async function apiGet(path){const response=await fetch('/api/'+path,{cache:'no-store'});let result;try{result=await response.json()}catch{throw new Error(t('后台响应异常，请刷新页面'))};if(!response.ok||result.error)throw new Error(t(result.error||'操作未完成'));return result;}
async function act(button,fn){if(button?.disabled)return;pending++;const original=button?.textContent;if(button){button.disabled=true;button.textContent=t('processing')}try{await fn()}catch(e){toast(e.message,true)}finally{pending--;if(button){button.disabled=false;button.textContent=original}await refresh()}}
async function refresh(){try{const response=await fetch('/api/state',{cache:'no-store'});if(!response.ok)throw Error();const previousVersion=state?.version;state=await response.json();if(previousVersion&&previousVersion!==state.version){location.reload();return}token=state.token;offline=false;$('backendStatus').innerHTML='<span class="local-dot"></span>'+t('backendOnline');render()}catch{offline=true;$('backendStatus').textContent=t('backendOffline')}}
function view(name){const settingsView=['settings','geofiles','api','changelog'].includes(name);document.querySelectorAll('.view').forEach(v=>v.classList.toggle('hidden',v.id!=='view-'+name));document.querySelectorAll('[data-view]').forEach(b=>b.classList.toggle('active',b.dataset.view===(settingsView?'settings':name)));$('settingsSubnav').classList.toggle('hidden',!settingsView);document.querySelectorAll('[data-settings-view]').forEach(b=>b.classList.toggle('active',b.dataset.settingsView===name));$('pageName').textContent={nodes:t('pageNodes'),apps:t('游戏 / 应用代理'),subscriptions:t('pageSubscriptions'),settings:t('pageSettings'),geofiles:t('pageSettings')+' / '+t('pageGeoFiles'),api:t('pageSettings')+' / '+t('pageAPI'),changelog:t('pageSettings')+' / '+t('pageChangelog')}[name];if(name==='changelog')renderChangelog();if(name==='settings')autoCheckCores();if(name==='geofiles')autoCheckGeo();try{sessionStorage.setItem('Mirroxy.view',name)}catch{}}
function render(){
 const platform=state.platform||{os:'windows',arch:'amd64',server:false,systemProxy:true};
 document.querySelectorAll('[data-windows-only]').forEach(el=>el.classList.toggle('hidden',platform.os!=='windows'));
 document.querySelectorAll('[data-server-only]').forEach(el=>el.classList.toggle('hidden',!platform.server));
	const appRoutingAvailable=platform.os==='windows'&&state.localAccess&&state.appRouting?.supported;document.querySelectorAll('.nav[data-app-routing]').forEach(el=>el.classList.toggle('hidden',!appRoutingAvailable));
 $('remoteCert').placeholder=platform.server?'/etc/ssl/fullchain.pem':'E:\\certs\\fullchain.pem';$('remoteKey').placeholder=platform.server?'/etc/ssl/privkey.pem':'E:\\certs\\privkey.pem';
 $('workspacePlatform').textContent=platform.os==='darwin'?'macOS · Apple Silicon':platform.server?'Linux '+platform.arch.toUpperCase()+t(' · 服务端'):t('Windows 工作空间');
 $('linuxServiceNote').classList.toggle('hidden',!platform.server);
 $('importDeployHint').textContent=t(state.settings.autoDeploy?'导入后自动选择核心、分配端口并启动':'导入后自行选择要启动的节点');
 const running=state.nodes.filter(n=>n.status==='running').length;
 $('version').textContent=state.version;$('navCount').textContent=state.nodes.length;$('totalCount').textContent=state.nodes.length;$('runningCount').textContent=running;$('listCount').textContent=state.nodes.length;
 renderNodes();renderProxy();renderTraffic(state.traffic);renderAppUpdate();if(appRoutingAvailable)renderAppRouting();window.MirroxyDownloads?.onState(state);
 $('appNavCount').textContent=appRoutingAvailable?((appRoutingDraft?.profiles??state.appRouting?.profiles)??[]).length:0;
 $('subNavCount').textContent=state.subscriptions.length;
 restoreView();
 $('subscriptions').innerHTML=state.subscriptions.length?state.subscriptions.map(s=>`<article class="sub-card"><div><h3>${escapeHTML(s.name)}</h3><small>${escapeHTML(s.error||t('最后更新：')+(s.updated||t('尚未更新')))}</small></div><button class="button small" data-refresh-sub="${s.id}">↻ ${t('更新订阅')}</button></article>`).join(''):`<div class="empty"><h3>${t('还没有添加订阅')}</h3><p>${t('添加订阅后，可以在这里一键更新节点。')}</p></div>`;
 renderCores()
 renderGeoFiles()
 if(!settingsDirty){for(const [id,key] of Object.entries(settingFields)){$(id)[$(id).type==='checkbox'?'checked':'value']=state.settings[key]??'';}}
 if(!remoteDirty){$('remoteEnabled').checked=state.remote.enabled;$('remoteScheme').value=state.remote.scheme;$('remotePort').value=state.remote.port;$('remoteUser').value=state.remote.username;$('remoteCert').value=state.remote.certFile;$('remoteKey').value=state.remote.keyFile;}
 $('remoteStatus').textContent=t(state.remote.error)|| (state.remote.running?t('远程 {scheme} 已开启，端口 {port}；需要账号密码。',{scheme:state.remote.scheme.toUpperCase(),port:state.remote.port}):t('远程访问已关闭。本机仍可免登录使用面板和 API。'));
 $('accessNote').textContent=t(state.remote.running?'本机免登录 · 远程需认证':'本机免登录');
$('apiAddress').textContent=location.origin+'/api/v1/';
 $('saveSettings').disabled=!state.settingsEditable;$('saveRemote').disabled=!state.settingsEditable;
 $('events').textContent=state.events?.map(translateEvent).join('\n')||t('noRecords');$('rootPath').textContent=t('运行目录：')+state.root;
}
function renderHealth(health){if(!health)return;
 const counts=[t('available')+' '+health.online,t('unavailable')+' '+health.offline];if(health.unknown)counts.push(t('pendingCheck')+' '+health.unknown);
 $('nodeHealthCounts').textContent=counts.join(' · ');
 $('healthSchedule').textContent=(health.enabled?t('每 {seconds} 秒检测所有节点',{seconds:health.intervalSeconds}):t('自动检测已关闭'))+(health.checking?' · '+t('checking')+' '+health.checking:'');
 for(const [id,h] of Object.entries(health.nodes)){
  const row=document.querySelector('[data-node-id="'+id+'"]');if(!row)continue;
  row.classList.toggle('health-offline',h.status==='offline');
  const badge=row.querySelector('[data-health]');
  const label=h.status==='online'?t('available'):h.status==='offline'?t('unavailable'):t('pendingCheck');
  badge.textContent=h.checking?(h.status==='unknown'?t('checking'):label+' · '+t('checking')):label;
  badge.dataset.status=h.status;
  const checked=Date.parse(h.checkedAt)>0?new Date(h.checkedAt).toLocaleString():'';
  badge.title=checked?t('最近检测：')+checked+'; '+(h.status==='online'?h.latencyMs+' ms':t(h.error)||label):t('尚未完成首次联网检测');
 }
}
function bytes(n){n=Number(n)||0;const units=['B','KiB','MiB','GiB','TiB'];let i=0;while(n>=1024&&i<units.length-1){n/=1024;i++}return (i?n.toFixed(n>=100?0:1):Math.round(n))+' '+units[i];}
function renderTraffic(traffic){if(!traffic)return;
 renderHealth(traffic.health);
 $('sumConnections').textContent=traffic.connections;$('sumUpload').textContent=bytes(traffic.uploadBytesPerSecond)+'/s';$('sumDownload').textContent=bytes(traffic.downloadBytesPerSecond)+'/s';$('sumUploaded').textContent=bytes(traffic.uploadedBytes);$('sumDownloaded').textContent=bytes(traffic.downloadedBytes);
 const stale=state.nodes.filter(n=>n.status==='running'&&!traffic.nodes[n.id]?.available).length;$('trafficHealth').textContent=stale?'· '+stale+' '+t('个节点等待统计'):'';
 for(const [id,v] of Object.entries(traffic.nodes)){const row=document.querySelector('[data-node-id="'+id+'"]');if(!row)continue;const live=state.nodes.find(n=>n.id===id)?.status==='running';const values={connections:live&&!v.connectionCountAvailable?'—':v.connections,upload:live&&!v.available?'—':bytes(v.uploadBytesPerSecond)+'/s',download:live&&!v.available?'—':bytes(v.downloadBytesPerSecond)+'/s',uploaded:bytes(v.uploadedBytes),downloaded:bytes(v.downloadedBytes)};for(const [key,value] of Object.entries(values)){const cell=row.querySelector('[data-stat="'+key+'"]');if(cell)cell.textContent=value}}
}
let trafficBusy=false;
async function refreshTraffic(){if(trafficBusy||document.hidden)return;trafficBusy=true;try{const res=await fetch('/api/traffic',{cache:'no-store'});if(res.ok){const traffic=await res.json();if(state){state.traffic=traffic;renderTraffic(traffic)}}}catch{$('trafficHealth').textContent='· '+t('统计连接中断')}finally{trafficBusy=false}}
function renderNodes(){
 const query=$('search').value.trim().toLowerCase();const nodes=MirroxyNodeList.sortedNodes(state.nodes.filter(n=>(n.name+' '+n.protocol+' '+n.server+' '+n.port).toLowerCase().includes(query)));
 const appsAvailable=state.platform?.os==='windows'&&state.localAccess&&state.appRouting?.supported;
 $('empty').classList.toggle('hidden',state.nodes.length>0);$('nodeTableWrap').classList.toggle('hidden',!state.nodes.length);
 $('nodes').innerHTML=nodes.map(n=>{const running=n.status==='running',port=n.port?'127.0.0.1:'+n.port:'',active=n.systemProxy;
 return `<tr data-node-id="${n.id}" class="${running?'running':''}${active?' active-system-node':''}" data-system-active="${active?'proxy':'none'}"><td class="name-cell"><div class="node-name"><h3 title="${escapeHTML(n.name)}">${escapeHTML(n.name)}</h3><button class="rename-node" data-rename-node="${n.id}" aria-label="${t('修改 {name} 的名称',{name:escapeHTML(n.name)})}" title="${t('修改节点名称')}">🖊</button></div><div class="node-meta">${active?`<span class="system-badge" title="${t('Windows 当前正在使用此节点作为系统代理')}"><span aria-hidden="true">◉</span>${t('系统代理')}</span>`:''}<span class="status ${n.status==='error'?'error':''}"><i></i>${t(running?'running':n.status==='error'?'startError':'stopped')}</span><span class="protocol">${escapeHTML(n.protocol.toUpperCase())}</span><span>${t(n.engine==='A'?'coreA':'coreB').replace(t('网络'),'')}</span><span class="node-health" data-health>${t('pendingCheck')}</span></div></td><td class="port-cell">${port?`<button class="port-copy" data-copy="${port}" title="${t('点击复制地址')}"><code>${port}</code><span>⧉</span></button>`:`<span class="muted">${t('启动后分配')}</span>`}</td><td class="numeric" data-stat="connections">—</td><td class="numeric upload" data-stat="upload">—</td><td class="numeric download" data-stat="download">—</td><td class="numeric" data-stat="uploaded">—</td><td class="numeric" data-stat="downloaded">—</td><td><select class="route-select" data-route="${n.id}" aria-label="${t('{name}出口策略',{name:escapeHTML(n.name)})}"><option value="inherit" ${!n.route||n.route==='inherit'?'selected':''}>${t('跟随全局')} · ${routeLabel(n.effectiveRoute)}</option><option value="blacklist" ${n.route==='blacklist'?'selected':''}>${t('黑名单')}</option><option value="proxy" ${n.route==='proxy'?'selected':''}>${t('全部走此节点')}</option><option value="bypass-cn" ${n.route==='bypass-cn'?'selected':''}>${t('绕过局域网和大陆')}</option></select></td><td class="row-actions"><button class="button small ${running?'':'primary'}" data-action="${n.enabled?'stop':'start'}" data-id="${n.id}">${t(n.enabled?'stop':'start')}</button><button class="text-btn more" data-more="${n.id}" aria-label="${t('{name}更多操作',{name:escapeHTML(n.name)})}">⋯</button><div class="popover hidden" id="more-${n.id}"><button data-share-node="${n.id}">${t('分享链接')}</button><button data-test="${n.id}">${t('立即检测可用性')}</button><button data-system="${n.id}" ${running?'':'disabled'}>${t('设为系统代理')}</button>${appsAvailable?`<button data-node-apps="${n.id}">${t('指定应用')}</button>`:''}<button data-action="reassign" data-id="${n.id}" ${n.enabled?'disabled':''}>${t('重新分配端口')}</button><button data-log="${n.engine}" data-log-node="${n.id}">${t('查看核心日志')}</button><button data-action="delete" data-id="${n.id}">${t('删除节点')}</button></div></td></tr>`;
 }).join('');
 for(const n of nodes){const row=$('nodes').querySelector('[data-node-id="'+n.id+'"]');if(!row)continue;row.classList.toggle('stopped',!n.enabled&&n.status!=='running');const placement=document.createElement('button');placement.dataset.action=n.listBottom?'auto-order':'move-bottom';placement.dataset.id=n.id;placement.textContent=t(n.listBottom?'恢复自动排序':'置于列表末尾');row.querySelector('.popover').prepend(placement);if(n.listBottom){const marker=document.createElement('span');marker.className='node-placement';marker.textContent=t('已置底');marker.title=t('手动置底，启动后仍保持在列表末尾');row.querySelector('.node-meta').append(marker)}}
 if(!nodes.length&&state.nodes.length)$('nodes').innerHTML=`<tr><td colspan="9" class="no-match">${t('没有匹配的节点')}</td></tr>`;
 document.querySelectorAll('[data-system]').forEach(el=>el.classList.toggle('hidden',state.platform?.systemProxy===false));
 renderTraffic(state.traffic);
}
function routeLabel(mode){return t({'bypass-cn':'绕过大陆',blacklist:'黑名单',proxy:'全局'}[mode]||'绕过大陆')}
function renderProxy(){
 if(!routingDirty)$('routingMode').value=state.settings.routingMode||'bypass-cn';
 $('routingMode').title=t({'bypass-cn':'局域网和大陆直连，其他流量走节点',blacklist:'命中代理名单走节点，其余直连',proxy:'全部流量走对应节点'}[$('routingMode').value])+t('。适用于跟随全局的节点，点击应用生效。');
 if(!proxyDirty){$('proxyMode').value=state.settings.systemProxyMode||'unchanged';$('proxyNode').innerHTML=state.nodes.filter(n=>n.status==='running').map(n=>`<option value="${n.id}">${escapeHTML(n.name)} · ${n.port}</option>`).join('');if(state.settings.systemProxyNode)$('proxyNode').value=state.settings.systemProxyNode;}
 const mode=$('proxyMode').value;$('proxyNode').classList.toggle('hidden',state.platform?.systemProxy===false||mode!=='auto');
 const hint=t({unchanged:'不写入系统设置。当前 v2rayN 等软件的系统代理保持原样；各节点端口可独立使用。',clear:'应用后将关闭当前 Windows 系统代理，包括其他代理软件配置的代理。',auto:'应用后将所选节点设为 Windows 系统代理。请先启动该节点。'}[mode]);
 const current=(state.systemProxyEnabled?t('当前系统代理：')+state.systemProxy:t('当前未启用系统代理'))+(state.settings.acceleratorPaused?t(' · 加速器运行中，已自动暂停'):'');
 $('proxyControls').title=current;
 $('proxyMode').title=current+'\n'+hint;
 $('applyProxyMode').title=t('应用路由及代理方式');
}
function cloneAppRouting(value){return JSON.parse(JSON.stringify(value||{defaultNode:'',compatMode:false,profiles:[]}))}
function renderAppRouting(){
 const routing=state.appRouting;if(!routing)return;
 if(!appRoutingDirty||!appRoutingDraft)appRoutingDraft=cloneAppRouting({defaultNode:routing.defaultNode||'',compatMode:!!routing.compatMode,profiles:routing.profiles||[]});
 if(!appRoutingDirty)$('appCompatMode').checked=!!appRoutingDraft.compatMode;
 const select=$('appDefaultNode');select.innerHTML=`<option value="">${t('请选择节点')}</option>`+state.nodes.map(n=>`<option value="${n.id}">${escapeHTML(n.name)}${n.status==='running'?' · '+t('running'):''}</option>`).join('');select.value=appRoutingDraft.defaultNode||'';
 const phase=routing.phase||'stopped',running=phase==='running',active=routing.active||running,busy=['starting','stopping'].includes(phase),failed=phase==='error';
 const errorMessage=routing.error||'';$('appRoutingError').classList.toggle('hidden',!errorMessage);$('appRoutingError').textContent=errorMessage;
 $('toggleAppRouting').textContent=t(active?'停止应用代理':busy?'处理中…':'开启应用代理');$('toggleAppRouting').disabled=busy||(!active&&(!appRoutingDraft.profiles.some(p=>p.enabled)||!state.nodes.length));$('saveAppRouting').disabled=active||busy||!appRoutingDirty;
 $('appProfileCount').textContent=appRoutingDraft.profiles.length;
 $('appProfiles').innerHTML=appRoutingDraft.profiles.length?appRoutingDraft.profiles.map(p=>{const nodeID=p.nodeId||appRoutingDraft.defaultNode,node=state.nodes.find(n=>n.id===nodeID),detailOpen=openAppDetails.has(p.id);return `<article class="app-profile-row ${p.enabled?'':'disabled'}"><div class="app-profile-info"><h3>${escapeHTML(p.name)}</h3><p>${p.enabled?t('已启用'):t('已停用')} · ${escapeHTML(node?.name||t('节点已失效'))} · ${p.executables.length} ${t('个程序')}</p></div><div class="app-profile-actions"><button class="text-btn" data-app-detail="${p.id}">${detailOpen?t('收起'):t('详情')}</button><button class="text-btn" data-app-edit="${p.id}">${t('编辑')}</button><button class="text-btn" data-app-delete="${p.id}">${t('删除')}</button></div></article><div class="app-profile-detail ${detailOpen?'':'hidden'}" data-app-detail-box="${p.id}">${p.executables.map(exe=>`<div class="app-exe-row"><code title="${escapeHTML(exe)}">${escapeHTML(exe)}</code><button class="text-btn" data-app-exe-remove="${p.id}" data-exe-value="${escapeHTML(exe)}" title="${t('移除')}">✕</button></div>`).join('')}<div class="app-exe-add"><input placeholder="${t('新增程序路径或 exe 名称')}" data-app-exe-input="${p.id}"><button class="button small" data-app-exe-add="${p.id}">${t('添加')}</button></div></div>`}).join(''):`<div class="app-profile-empty"><h3>${t('还没有应用配置')}</h3><p>${t('添加游戏或程序后，再开启应用代理。')}</p></div>`;
	const capability=routing.capabilities?.[appRoutingDraft.defaultNode],capabilityNode=state.nodes.find(n=>n.id===appRoutingDraft.defaultNode);$('probeCapabilities').disabled=!capabilityNode||capabilityNode.status!=='running';
	if(capability){const nat=t({"endpoint-independent":'映射稳定',"symmetric-likely":'对称型倾向',unknown:'未知'}[capability.nat]||'未知');$('capabilityResults').innerHTML=`<span class="${capability.tcp?'ok':'bad'}">TCP · ${t(capability.tcp?'支持':'不支持')}</span><span class="${capability.udp?'ok':'bad'}">UDP · ${t(capability.udp?'支持':'不支持')}</span><span class="${capability.ipv6?'ok':'bad'}">IPv6 · ${t(capability.ipv6?'支持':'不支持')}</span><span class="${capability.nat==='endpoint-independent'?'ok':capability.nat==='symmetric-likely'?'bad':''}">NAT · ${nat}</span>`;$('capabilityMeta').textContent=(capability.externalAddress?t('观察到的 UDP 出口：')+capability.externalAddress+' · ':'')+t('检测时间：')+new Date(capability.checkedAt).toLocaleString()+(capability.udpError?' · UDP: '+capability.udpError:'')+(capability.ipv6Error?' · IPv6: '+capability.ipv6Error:'')}else{$('capabilityResults').innerHTML=`<span>${t('尚未检测')}</span>`;$('capabilityMeta').textContent=capabilityNode&&capabilityNode.status!=='running'?t('请先启动默认应用节点'):''}
}
function appNodeOptions(selected=''){return `<option value="">${t('跟随默认应用节点')}</option>`+state.nodes.map(n=>`<option value="${n.id}" ${n.id===selected?'selected':''}>${escapeHTML(n.name)}</option>`).join('')}
let editingAppProfile='';
function openAppProfile(id='',presetNode=''){const p=appRoutingDraft?.profiles.find(x=>x.id===id);editingAppProfile=id;$('appProfileTitle').textContent=t(p?'编辑应用':'添加应用');$('appProfileName').value=p?.name||'';$('appProfileNode').innerHTML=appNodeOptions(p?p.nodeId:(presetNode||''));$('appProfileExecutables').value=(p?.executables||[]).join('\n');$('appProfileEnabled').checked=p?.enabled??true;$('dirBrowseResults').classList.add('hidden');$('scanDirResults').classList.add('hidden');$('scanDirActions').classList.add('hidden');$('appProfileDialog').showModal();}
let nodeAppsID='';
function openNodeApps(id){const node=state.nodes.find(n=>n.id===id);if(!node)return;nodeAppsID=id;$('nodeAppsNodeName').textContent=t('绑定到 {name} 的应用按它的出口策略分流。',{name:node.name});renderNodeAppsList();$('nodeAppsDialog').showModal();}
function renderNodeAppsList(){if(!nodeAppsID)return;const list=(appRoutingDraft?.profiles||[]).filter(p=>p.nodeId===nodeAppsID);const node=state.nodes.find(n=>n.id===nodeAppsID);$('nodeAppsList').innerHTML=list.length?list.map(p=>`<div class="node-app-row"><div><strong>${escapeHTML(p.name)}</strong><small>${p.enabled?t('已启用'):t('已停用')} · ${p.executables.length} ${t('个程序')} · ${t('出口策略')} ${routeLabel(node?.effectiveRoute)}</small></div><div class="app-profile-actions"><button class="text-btn" data-app-edit="${p.id}">${t('编辑')}</button><button class="text-btn" data-app-delete="${p.id}">${t('删除')}</button></div></div>`).join(''):`<div class="node-app-empty">${t('还没有应用绑定到此节点，点击「添加应用」并配合目录扫描选择程序。')}</div>`}
function refreshNodeAppsDialog(){if($('nodeAppsDialog').open)renderNodeAppsList()}
const joinDir=(base,name)=>base+(base.endsWith('\\')||base.endsWith('/')?'':'\\')+name;
async function browseDirAt(path){const result=await apiGet('app-routing/browse?path='+encodeURIComponent(path||''));$('dirBrowseResults').classList.remove('hidden');$('scanDirPath').value=result.path||'';const chips=[];if(result.drives?.length)chips.push(...result.drives.map(d=>`<button type="button" class="dir-chip" data-dir-go="${escapeHTML(d)}">${escapeHTML(d)}</button>`));if(result.parent)chips.push(`<button type="button" class="dir-chip" data-dir-go="${escapeHTML(result.parent)}">↑ ..</button>`);const dirs=(result.directories||[]).map(d=>`<button type="button" class="dir-chip" data-dir-base="${escapeHTML(result.path)}" data-dir-name="${escapeHTML(d)}">${escapeHTML(d)}</button>`);$('dirBrowseResults').innerHTML=`<div class="dir-chips">${chips.join('')}</div>`+(dirs.length?`<div class="dir-list">${dirs.join('')}</div>`:`<small class="muted">${t('没有子目录')}</small>`)}
async function scanDirAt(path){const result=await apiGet('app-routing/scan?path='+encodeURIComponent(path||''));const items=result.executables||[];$('scanDirResults').classList.remove('hidden');$('scanDirResults').innerHTML=items.map(i=>`<label class="scan-row"><input type="checkbox" data-scan-path="${escapeHTML(i.path)}" ${i.main?'checked':''}><span class="scan-name" title="${escapeHTML(i.path)}">${escapeHTML(i.name)}</span><code>${escapeHTML(i.path)}</code>${i.main?`<em class="scan-main">${t('主程序')}</em>`:''}<small>${bytes(i.size)}</small></label>`).join('');$('scanDirActions').classList.remove('hidden');updateScanHint()}
function updateScanHint(){const n=$('scanDirResults').querySelectorAll('input:checked').length;$('scanDirHint').textContent=t('已选 {count} 个程序',{count:n})}
const openAppDetails=new Set();
function addAppExecutable(id){const input=document.querySelector('[data-app-exe-input="'+id+'"]');const value=(input?.value||'').trim();if(!value){toast(t('请填写程序路径或 exe 名称'),true);return}const p=appRoutingDraft.profiles.find(p=>p.id===id);if(!p)return;if(p.executables.some(v=>v.toLowerCase()===value.toLowerCase())){toast(t('该程序已在列表中'),true);return}p.executables.push(value);appRoutingDirty=true;renderAppRouting();refreshNodeAppsDialog();toast(t('已添加，记得保存配置'))}
async function saveAppRoutingConfig(){if(!appRoutingDraft.defaultNode&&appRoutingDraft.profiles.some(p=>p.enabled&&!p.nodeId))throw Error(t('请先选择默认应用节点'));await api('app-routing/config',{appRouting:{defaultNode:appRoutingDraft.defaultNode,strictRoute:!appRoutingDraft.compatMode,compatMode:!!appRoutingDraft.compatMode,profiles:appRoutingDraft.profiles}});appRoutingDirty=false;}
function showImport(mode){mode=mode||(state?.platform?.os!=='windows'?'links':'v2rayn');setImportMode(mode);$('importResult').classList.add('hidden');$('importDialog').showModal();}
function setImportMode(mode){importMode=mode;document.querySelectorAll('[data-import]').forEach(b=>b.classList.toggle('active',b.dataset.import===mode));document.querySelectorAll('.import-pane').forEach(p=>p.classList.toggle('hidden',p.id!=='import-'+mode));$('doImport').textContent=t(mode==='subscription'?'添加并更新':state?.settings?.autoDeploy?'导入并部署':'导入节点');}
let coresAutoChecked=false;
function renderCores(){$('cores').innerHTML=state.cores.map(c=>`<article class="core-card"><div class="status ${c.running?'running':''}"><i></i>${t(c.running?'正在提供代理':'待命')}</div><h3>${t(c.engine==='A'?'coreA':'coreB')}</h3><p>${t('当前版本')} <b>${escapeHTML(t(c.version||'未检测到'))}</b></p><p>${latest.has(c.engine)?t('官方最新版本')+' <b>'+escapeHTML(latest.get(c.engine))+'</b>':t('尚未检查官方版本')}</p><div class="buttons"><button class="button small" data-core-check="${c.engine}">${t('检查更新')}</button><button class="button small primary" data-core-update="${c.engine}" ${state.updating?'disabled':''}>${t('升级内核')}</button><button class="button small ghost" data-log="${c.engine}">${t('日志')}</button></div></article>`).join('')}
function autoCheckCores(){if(coresAutoChecked||!state)return;coresAutoChecked=true;for(const c of state.cores){if(!c.installed)continue;api('core/check',{engine:c.engine}).then(r=>{latest.set(c.engine,r.version);renderCores()}).catch(()=>{})}}
function renderGeoFiles(){const files=state.geoFiles||[];const card=f=>{const engine=f.engine==='B'?'coreB':'coreA';return `<article class="core-card"><div class="status ${f.exists?'running':'error'}"><i></i>${t(f.exists?'已就绪':'缺失')}</div><h3>${escapeHTML(f.name)} <small>${t(engine)}</small></h3><p class="geo-purpose">${escapeHTML(t(f.purpose))}</p><p>${t('文件')} <code>${escapeHTML(f.asset)}</code></p><p>${f.exists?t('大小')+' <b>'+bytes(f.size)+'</b> · '+t('更新时间')+' <b>'+escapeHTML(f.updatedAt||'—')+'</b>':t('文件尚未就绪')}</p><p>${f.version?t('规则版本')+' <b>'+escapeHTML(f.version)+'</b>':t('内置快照')}</p><div class="buttons"><button class="button small primary" data-geo-update="${f.name}" ${state.updating?'disabled':''}>${t('更新')}</button></div></article>`};
 $('geoXray').innerHTML=files.filter(f=>f.kind==='dat').map(card).join('');
 $('geoSing').innerHTML=files.filter(f=>f.kind==='srs').map(card).join('');
 const missing=files.filter(f=>!f.exists).length;
 $('geoMissingHint').classList.toggle('hidden',!missing);
 if(missing)$('geoMissingHint').textContent=t('有 {count} 个规则文件缺失，点击「更新全部」可自动补齐。',{count:missing});
 $('geoXrayMeta').textContent=latest.has('geo')?t('规则源最新版本：')+latest.get('geo'):t('进入本页自动查询规则源最新版本。');
 $('geoUpdateAll').disabled=state.updating;$('geoCheck').disabled=state.updating;}
let geoAutoChecked=false;
function autoCheckGeo(){if(geoAutoChecked||!state)return;geoAutoChecked=true;api('geofiles/check',{}).then(r=>{latest.set('geo',r.version);renderGeoFiles()}).catch(()=>{})}
function renderAppUpdate(){const u=state.appUpdate||{};const busy=['checking','downloading','validating','restarting'].includes(u.phase);$('appUpdateVersion').textContent=t('当前 v')+state.version+(u.available?t(' · 可更新至 v')+u.latest:'');$('appUpdateStatus').textContent=t(u.message||'检查程序更新，保留节点和流量记录。');$('checkAppUpdate').disabled=busy;$('applyAppUpdate').disabled=busy;$('applyAppUpdate').classList.toggle('hidden',!u.available||busy);}
$('checkAppUpdate').onclick=()=>act($('checkAppUpdate'),async()=>{const u=await api('app-update/check',{});state.appUpdate=u;renderAppUpdate();toast(u.message)});
$('applyAppUpdate').onclick=()=>act($('applyAppUpdate'),async()=>{await api('app-update/apply',{});toast(t('更新已开始，下载期间代理继续运行；后台重启后页面自动恢复。'))});
let renameNodeID='',changelogEntries=null,qrNodeID='';
$('toggleNodeQR').onclick=()=>{const box=$('nodeExportQR'),show=box.classList.contains('hidden');if(show){if(!qrNodeID)return;const dark=document.documentElement.dataset.theme==='dark';$('nodeExportQRImage').src='/api/node/qr?id='+encodeURIComponent(qrNodeID)+(dark?'&theme=dark':'')+'&_='+Date.now()}box.classList.toggle('hidden',!show);$('toggleNodeQR').textContent=t(show?'只看链接':'显示二维码')};
$('cancelRename').onclick=()=>$('renameDialog').close();
$('renameDialog').addEventListener('close',()=>{renameNodeID='';$('renameName').value=''});
$('renameForm').onsubmit=e=>{e.preventDefault();const id=renameNodeID,name=$('renameName').value.trim();act($('saveRename'),async()=>{if(!name)throw Error(t('请填写节点名称'));await api('node/rename',{id,name});$('renameDialog').close();toast(t('节点名称已保存'))})};
async function renderChangelog(){
 try{
 if(!changelogEntries){$('changelog').textContent=t('正在读取更新日志…');const file=(window.MirroxyI18n?.locale==='en'?'/changelog.en.json':'/changelog.json')+'?v='+encodeURIComponent(state?.version||'0.6.3');const response=await fetch(file);if(!response.ok)throw Error();changelogEntries=await response.json();}
 $('changelog').innerHTML=changelogEntries.map(entry=>`<article class="release-note"><div class="release-heading"><h2>v${escapeHTML(entry.version)}</h2>${entry.version===(state?.version||'0.6.3')?`<span class="release-current">${t('currentVersion')}</span>`:''}${entry.date?`<time datetime="${escapeHTML(entry.date)}">${escapeHTML(entry.date)}</time>`:''}</div><ul>${entry.changes.map(change=>`<li>${escapeHTML(change)}</li>`).join('')}</ul></article>`).join('');
 }catch{$('changelog').textContent=t('更新日志读取失败，请在全局设置中重试。')}
}
document.querySelectorAll('[data-view]').forEach(b=>b.onclick=()=>view(b.dataset.view));
document.querySelectorAll('[data-settings-view]').forEach(b=>b.onclick=()=>view(b.dataset.settingsView));
document.querySelectorAll('[data-import]').forEach(b=>b.onclick=()=>setImportMode(b.dataset.import));
$('appDefaultNode').onchange=()=>{if(appRoutingDraft){appRoutingDraft.defaultNode=$('appDefaultNode').value;appRoutingDirty=true;renderAppRouting()}};
$('appCompatMode').onchange=()=>{if(!appRoutingDraft)return;appRoutingDraft.compatMode=$('appCompatMode').checked;appRoutingDirty=true};
$('addAppProfile').onclick=()=>openAppProfile();$('cancelAppProfile').onclick=()=>$('appProfileDialog').close();
$('appProfileForm').onsubmit=e=>{e.preventDefault();const executables=$('appProfileExecutables').value.split(/\r?\n/).map(v=>v.trim()).filter(Boolean);if(!executables.length){toast(t('请至少填写一个程序路径或 exe 名称'),true);return}const profile={id:editingAppProfile||crypto.randomUUID?.().replaceAll('-','')||String(Date.now()),name:$('appProfileName').value.trim()||executables[0].split(/[\\/]/).pop().replace(/\.exe$/i,''),nodeId:$('appProfileNode').value,executables,enabled:$('appProfileEnabled').checked};const index=appRoutingDraft.profiles.findIndex(p=>p.id===editingAppProfile);if(index>=0)appRoutingDraft.profiles[index]=profile;else appRoutingDraft.profiles.push(profile);appRoutingDirty=true;$('appProfileDialog').close();renderAppRouting();refreshNodeAppsDialog()};
$('closeNodeApps').onclick=()=>$('nodeAppsDialog').close();
$('addNodeApp').onclick=()=>openAppProfile('',nodeAppsID);
$('saveNodeApps').onclick=()=>act($('saveNodeApps'),async()=>{await saveAppRoutingConfig();$('nodeAppsDialog').close();toast(t('应用代理配置已保存'))});
$('browseDir').onclick=()=>act($('browseDir'),()=>browseDirAt($('scanDirPath').value.trim()));
$('scanDir').onclick=()=>act($('scanDir'),()=>scanDirAt($('scanDirPath').value.trim()));
$('scanDirResults').addEventListener('change',updateScanHint);
$('useScanned').onclick=()=>{const existing=$('appProfileExecutables').value.split(/\r?\n/).map(v=>v.trim()).filter(Boolean);const seen=new Set(existing.map(v=>v.toLowerCase()));let added=0;for(const box of $('scanDirResults').querySelectorAll('input:checked')){const value=box.dataset.scanPath;if(!seen.has(value.toLowerCase())){existing.push(value);seen.add(value.toLowerCase());added++}}if(!added){toast(t('请先勾选要加入的程序'),true);return}$('appProfileExecutables').value=existing.join('\n');toast(t('已加入 {count} 个程序，记得保存应用',{count:added}))};
$('loadProcesses').onclick=()=>act($('loadProcesses'),async()=>{const result=await apiGet('processes');$('runningProcess').innerHTML=`<option value="">${t('选择正在运行的程序…')}</option>`+result.processes.map(p=>`<option value="${escapeHTML(p.path||p.name)}">${escapeHTML(p.name)} · PID ${p.pid}${p.path?' · '+escapeHTML(p.path):''}</option>`).join('')});
$('useProcess').onclick=()=>{const value=$('runningProcess').value;if(!value)return;const values=$('appProfileExecutables').value.split(/\r?\n/).map(v=>v.trim()).filter(Boolean);if(!values.some(v=>v.toLowerCase()===value.toLowerCase()))values.push(value);$('appProfileExecutables').value=values.join('\n')};
$('saveAppRouting').onclick=()=>act($('saveAppRouting'),async()=>{await saveAppRoutingConfig();toast(t('应用代理配置已保存'))});
$('toggleAppRouting').onclick=()=>act($('toggleAppRouting'),async()=>{if(state.appRouting.active||state.appRouting.running){await api('app-routing/stop',{});toast(t('应用代理已停止'))}else{if(appRoutingDirty)await saveAppRoutingConfig();const scan=await apiGet('app-routing/conflicts');if(scan.conflicts?.length){const names=scan.conflicts.map(x=>x.product+'（'+x.process+'）').join('、');if(!confirm(t('检测到正在运行的疑似加速器：{names}。\n\n两者都会接管游戏流量，可能互相冲突、加速失效或掉线。建议先退出加速器再开启。仍要继续吗？',{names})))return toast(t('已取消开启，请先退出加速器'),true)}await api('app-routing/start',{});toast(t('应用代理已启动'))}});
$('probeCapabilities').onclick=()=>act($('probeCapabilities'),async()=>{if(!appRoutingDraft.defaultNode)throw Error(t('请先选择默认应用节点'));const result=await api('node/capabilities',{id:appRoutingDraft.defaultNode});toast(t('能力检测完成：')+` TCP ${result.tcp?'✓':'×'} · UDP ${result.udp?'✓':'×'} · IPv6 ${result.ipv6?'✓':'×'}`)});
$('importTop').onclick=$('importEmpty').onclick=()=>showImport();$('addSubscription').onclick=()=>showImport('subscription');
$('search').oninput=()=>state&&renderNodes();
$('proxyMode').onchange=()=>{proxyDirty=true;renderProxy()};$('proxyNode').onchange=()=>proxyDirty=true;
$('routingMode').onchange=()=>{routingDirty=true;renderProxy()};
$('applyProxyMode').onclick=()=>act($('applyProxyMode'),async()=>{if(routingDirty){await api('routing',{mode:$('routingMode').value});routingDirty=false}if(proxyDirty&&state.platform?.systemProxy!==false){await api('proxy',{action:'mode',mode:$('proxyMode').value,id:$('proxyNode').value});proxyDirty=false}toast(t('路由及代理方式已应用'))});
$('doImport').onclick=()=>act($('doImport'),async()=>{
 let result;if(importMode==='subscription'){if(!$('subUrl').value.trim())throw Error(t('请填写订阅地址'));const added=await api('subscription/add',{name:$('subName').value.trim(),url:$('subUrl').value.trim()});result=await api('subscription/refresh',{id:added.id})}
 else{result=await api('import',importMode==='v2rayn'?{path:$('v2raynPath').value.trim()}:{text:$('nodeLinksInput').value.trim()})}
 if(result.warnings?.length){$('importResult').textContent=t('导入 {count} 个节点。',{count:result.count})+'\n'+result.warnings.map(t).join('\n');$('importResult').className='form-result'}else{$('importDialog').close();toast(result.deployed?t('已导入 {count} 个节点，自动启动 {deployed} 个代理',{count:result.count,deployed:result.deployed}):t('已导入 {count} 个节点，点击「启动」即可使用',{count:result.count}));view('nodes')}
});
function openNodeMenu(id,x,y,toggle=false){
 const menu=$('more-'+id);if(!menu)return;
 const wasOpen=!menu.classList.contains('hidden');
 document.querySelectorAll('.popover').forEach(p=>p.classList.add('hidden'));
 if(toggle&&wasOpen)return;
 menu.classList.remove('hidden');menu.style.maxHeight=(window.innerHeight-16)+'px';menu.style.overflowY='auto';
 menu.style.left=Math.max(8,Math.min(x,window.innerWidth-menu.offsetWidth-8))+'px';
 menu.style.top=Math.max(8,Math.min(y,window.innerHeight-menu.offsetHeight-8))+'px';
}
$('nodes').addEventListener('contextmenu',e=>{const row=e.target.closest('[data-node-id]');if(!row)return;e.preventDefault();openNodeMenu(row.dataset.nodeId,e.clientX,e.clientY)});
document.addEventListener('keydown',e=>{if(e.key==='Escape')document.querySelectorAll('.popover').forEach(p=>p.classList.add('hidden'));if(e.key==='Enter'&&e.target instanceof HTMLInputElement&&e.target.dataset.appExeInput!==undefined){e.preventDefault();addAppExecutable(e.target.dataset.appExeInput)}});
$('copyNodeExport').onclick=async()=>{try{await copyText($('nodeExportURI').value);toast(t('分享链接已复制'))}catch{$('nodeExportURI').focus();$('nodeExportURI').select();toast(t('请复制已选中的链接'),true)}};
$('nodeExportDialog').addEventListener('close',()=>{$('nodeExportURI').value='';$('nodeExportName').textContent='';qrNodeID='';$('nodeExportQR').classList.add('hidden');$('toggleNodeQR').textContent=t('显示二维码')});
document.addEventListener('click',e=>{const b=e.target.closest('button');if(!b){document.querySelectorAll('.popover').forEach(p=>p.classList.add('hidden'));return;}
 if(b.dataset.copy){copyText(b.dataset.copy).then(()=>toast(t('已复制 ')+b.dataset.copy)).catch(()=>toast(t('请手动复制：')+b.dataset.copy));}
 if(b.dataset.more){const rect=b.getBoundingClientRect();openNodeMenu(b.dataset.more,rect.right-175,rect.bottom+4,true);return;}
 if(b.dataset.renameNode){const n=state.nodes.find(n=>n.id===b.dataset.renameNode);if(n){renameNodeID=n.id;$('renameName').value=n.name;$('renameDialog').showModal();$('renameName').focus();$('renameName').select();}}
	if(b.dataset.appDetail){const id=b.dataset.appDetail;openAppDetails.has(id)?openAppDetails.delete(id):openAppDetails.add(id);renderAppRouting();return;}
	if(b.dataset.appExeRemove){const p=appRoutingDraft.profiles.find(p=>p.id===b.dataset.appExeRemove);if(p){p.executables=p.executables.filter(v=>v!==b.dataset.exeValue);appRoutingDirty=true;renderAppRouting();refreshNodeAppsDialog()}return;}
	if(b.dataset.appExeAdd){addAppExecutable(b.dataset.appExeAdd);return;}
	if(b.dataset.appEdit){openAppProfile(b.dataset.appEdit);return;}
	if(b.dataset.appDelete){const p=appRoutingDraft.profiles.find(p=>p.id===b.dataset.appDelete);if(p&&confirm(t('删除应用配置 {name}？',{name:p.name}))){appRoutingDraft.profiles=appRoutingDraft.profiles.filter(p=>p.id!==b.dataset.appDelete);appRoutingDirty=true;renderAppRouting();refreshNodeAppsDialog()}return;}
	if(b.dataset.nodeApps){openNodeApps(b.dataset.nodeApps);return;}
	if(b.dataset.dirGo){act(b,()=>browseDirAt(b.dataset.dirGo));return;}
	if(b.dataset.dirName){act(b,()=>browseDirAt(joinDir(b.dataset.dirBase,b.dataset.dirName)));return;}
 if(b.dataset.shareNode){act(b,async()=>{const result=await api('node/share',{id:b.dataset.shareNode});$('nodeExportName').textContent=result.name;$('nodeExportURI').value=result.url;qrNodeID=b.dataset.shareNode;$('nodeExportQR').classList.add('hidden');$('toggleNodeQR').textContent=t('显示二维码');$('nodeExportDialog').showModal()});}
 if(b.dataset.action){if(b.dataset.action==='delete'&&!confirm(t('删除这个节点？它的本地代理端口也会停止。')))return;act(b,async()=>{await api('node',{id:b.dataset.id,action:b.dataset.action});toast(t({'start':'代理已启动，端口固定绑定此节点','move-bottom':'已置于列表末尾','auto-order':'已恢复自动排序'}[b.dataset.action]||'操作已完成'))});}
 if(b.dataset.test){act(b,async()=>{const result=await api('test',{id:b.dataset.test});toast(t('节点可用：')+result.latencyMs+' ms')});}
 
 if(b.dataset.system){act(b,async()=>{await api('proxy',{id:b.dataset.system,action:'set'});proxyDirty=false;toast(t('系统代理已启用，支持系统代理的浏览器和应用现在使用此节点'))});}
 if(b.dataset.refreshSub){act(b,async()=>{const result=await api('subscription/refresh',{id:b.dataset.refreshSub});toast(t('订阅已更新，共 {count} 个节点',{count:result.count}))});}
 if(b.dataset.coreCheck){act(b,async()=>{const result=await api('core/check',{engine:b.dataset.coreCheck});latest.set(b.dataset.coreCheck,result.version);toast(t('官方最新版本：')+result.version)});}
 if(b.dataset.coreUpdate){act(b,async()=>{toast(t('正在下载并检查核心更新，请稍候…'));await api('core/update',{engine:b.dataset.coreUpdate});toast(t('核心已就绪'))});}
 if(b.dataset.geoUpdate){act(b,async()=>{toast(t('正在下载并更新 Geo 规则文件，完成后对应内核会短暂重启…'));await api('geofiles/update',{files:[b.dataset.geoUpdate]});toast(t('Geo 规则文件已更新'))});}
 if(b.dataset.geoUpdateAll){act(b,async()=>{toast(t('正在更新全部 Geo 规则文件，请稍候…'));await api('geofiles/update',{});toast(t('Geo 规则文件已全部更新'))});}
 if(b.dataset.geoCheck){act(b,async()=>{const result=await api('geofiles/check',{});latest.set('geo',result.version);renderGeoFiles();toast(t('规则源最新版本：')+result.version)});}
 if(b.dataset.log){act(b,async()=>{const result=await api('log',{engine:b.dataset.log,id:b.dataset.logNode});$('logContent').textContent=result.text;$('logDialog').showModal()});}
 document.querySelectorAll('.popover').forEach(p=>p.classList.add('hidden'));
});
document.addEventListener('change',e=>{if(e.target.dataset.route){act(null,()=>api('node',{id:e.target.dataset.route,action:'route',route:e.target.value}))}});
const settingFields={settingPort:'startPort',settingUpdate:'autoUpdate',settingSubscribe:'autoSubscribe',settingAutostart:'autostart',settingOpenBrowser:'openBrowser',settingResume:'resumeNodes',settingRoute:'defaultRoute',settingLog:'logLevel',settingSubscribeHours:'subscribeHours',settingHealth:'healthMonitor',settingHealthInterval:'healthInterval',settingAutoDeploy:'autoDeploy',settingAutoDownload:'autoDownload',settingAccelGuard:'pauseSystemProxyOnAccelerator',settingBypassLAN:'bypassLAN',settingGeoUpdate:'geoAutoUpdate',settingPanelPort:'panelPort'};
Object.keys(settingFields).forEach(id=>$(id).addEventListener('input',()=>settingsDirty=true));
$('saveSettings').onclick=()=>act($('saveSettings'),async()=>{const data={};for(const [id,key] of Object.entries(settingFields)){const el=$(id);data[key]=el.type==='checkbox'?el.checked:el.type==='number'?Number(el.value):el.value}const previousPort=state.settings.panelPort;await api('settings',data);settingsDirty=false;toast(t('全局设置已保存'));if(data.panelPort&&data.panelPort!==previousPort){toast(t('面板端口已变更，正在跳转到新地址…'));setTimeout(()=>{location.href=location.protocol+'//127.0.0.1:'+data.panelPort+'/'},1800)}});
$('trustPanelCA').onclick=()=>act($('trustPanelCA'),async()=>{await api('panel/trust-ca',{});toast(t('根证书已安装并通过 HTTPS 校验，正在重新建立安全连接…'));setTimeout(()=>{const next=new URL(location.origin+'/');next.searchParams.set('ca-trusted',Date.now());location.replace(next)},700)});
async function loadPanelCertStatus(){try{const s=await apiGet('panel/cert-status');const el=$('panelCertStatus');if(!el)return;if(s.tls){el.textContent=(s.trusted?t('证书已受信任，浏览器显示安全锁'):t('证书已生成，点击「信任根证书」完成安装'))+' · '+escapeHTML(s.fingerprint||'')+' · '+t('有效期至')+' '+escapeHTML(s.notAfter||'')}else{el.textContent=t('面板当前为 HTTP：证书未就绪')}}catch{}}
['remoteScheme','remoteEnabled','remotePort','remoteUser','remotePassword','remoteCert','remoteKey'].forEach(id=>$(id).addEventListener('input',()=>remoteDirty=true));
$('saveRemote').onclick=()=>act($('saveRemote'),async()=>{await api('remote',{remote:{enabled:$('remoteEnabled').checked,scheme:$('remoteScheme').value,port:Number($('remotePort').value),username:$('remoteUser').value.trim(),certFile:$('remoteCert').value.trim(),keyFile:$('remoteKey').value.trim()},password:$('remotePassword').value});$('remotePassword').value='';remoteDirty=false;toast(t('远程设置已保存'))});
$('copyAPI').onclick=()=>act($('copyAPI'),async()=>{const base=$('apiAddress').textContent;await copyText(t('请使用 Mirroxy API 管理本机代理。基础地址：')+base+t(state.localAccess?'。本机无需认证。':'。使用面板账号密码（HTTP Basic）认证。')+t('先读取 ')+base+'guide '+t('和 ')+base+'openapi.json'+t('，再 GET nodes 获取节点 id。默认不要修改系统代理。'));toast(t('已复制，可直接发给 AI'))});
$('stopAll').onclick=()=>act($('stopAll'),async()=>{await api('stop-all',{});toast(t('所有本地代理已停止'))});
$('shutdown').onclick=async()=>{if(!confirm(t('退出 Mirroxy 后台？所有由它提供的代理都会停止。')))return;try{await api('shutdown',{});toast(t(state.platform?.os==='darwin'?'后台已退出，使用 launchctl kickstart 可再次启动':state.platform?.server?'后台已退出。控制台运行 sudo Mirroxyctl start 可再次启动':'后台已退出。双击 Mirroxy.exe 可再次启动'));$('backendStatus').textContent=t('后台已退出');offline=true}catch(e){toast(e.message,true)}};
let pendingView=null;
try{pendingView=sessionStorage.getItem('Mirroxy.view')}catch{}
function restoreView(){if(!pendingView)return;const base=['settings','geofiles','api','changelog'].includes(pendingView)?'settings':pendingView;const nav=document.querySelector('.nav[data-view="'+base+'"]');if(nav&&!nav.classList.contains('hidden')){const target=pendingView;pendingView=null;view(target);if(target==='downloads')window.MirroxyDownloads?.activate()}}
restoreView();
setTimeout(loadPanelCertStatus,2000);
refresh();setInterval(()=>{if(!pending&&!document.hidden&&!document.querySelector('dialog[open]')&&!document.querySelector('.popover:not(.hidden)')&&!['INPUT','TEXTAREA','SELECT'].includes(document.activeElement.tagName))refresh()},5000);setInterval(refreshTraffic,1000);window.addEventListener('scroll',()=>document.querySelectorAll('.popover').forEach(p=>p.classList.add('hidden')),true);
