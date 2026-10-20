'use strict';
(() => {
  let data = {tasks:[],defaultDir:''}, selected = new Set(), filter = 'all', query = '', loading = false, editing = null, timer;
  const labels = {active:'下载中',waiting:'等待中',paused:'已暂停',complete:'已完成',error:'失败',removed:'已移除'};
  const dl = id => document.getElementById(id);
  const esc = escapeHTML;
  const text = key => t(key);
  const size = value => bytes(Number(value)||0);
  const nav = document.createElement('button');
  nav.className = 'nav'; nav.dataset.view = 'downloads'; nav.dataset.downloads = '';
  nav.innerHTML = '<span>↓</span><span>下载管理</span><span class="nav-count" id="downloadNavCount">0</span>';
  document.querySelector('.sidebar nav').insertBefore(nav,document.querySelector('[data-view="subscriptions"]'));
  document.querySelector('main').insertAdjacentHTML('beforeend',`<section id="view-downloads" class="view hidden" aria-label="下载管理">
    <div class="page-heading dl-heading"><div><div class="eyebrow">ARIA2 · DOWNLOAD MANAGER</div><h1>下载管理</h1><p>熟悉的任务列表，独立的下载代理。关闭网页，下载继续。</p></div><span id="dlEngine" class="dl-engine">引擎待命</span></div>
    <div class="dl-toolbar" role="toolbar" aria-label="下载工具栏">
      <button class="dl-tool dl-add" id="dlAdd"><span>＋</span>新建下载</button>
      <button class="dl-tool" data-dl-bulk="resume"><span class="dl-green">▶</span>继续</button>
      <button class="dl-tool" data-dl-bulk="pause"><span class="dl-amber">Ⅱ</span>暂停</button>
      <button class="dl-tool" id="dlEdit"><span class="dl-blue">✎</span>编辑</button>
      <button class="dl-tool" data-dl-bulk="retry"><span class="dl-blue">↻</span>重试</button>
      <button class="dl-tool" data-dl-bulk="remove"><span class="dl-red">×</span>删除记录</button>
      <span class="dl-toolbar-divider"></span>
      <button class="dl-tool" id="dlPauseAll"><span class="dl-amber">Ⅱ</span>全部暂停</button>
      <button class="dl-tool" id="dlRefresh"><span>↻</span>刷新</button><button class="dl-tool hidden" id="dlInstall"><span>↓</span>安装 aria2 引擎</button>
    </div>
    <div class="dl-list-controls"><label class="search"><span>⌕</span><input id="dlSearch" placeholder="搜索文件名或链接" aria-label="搜索下载任务"></label><select id="dlFilter" aria-label="任务状态"><option value="all">全部任务</option><option value="active">下载中 / 等待中</option><option value="paused">已暂停</option><option value="complete">已完成</option><option value="error">失败</option></select><span id="dlSelection">未选择任务</span></div>
    <p id="dlError" class="form-result hidden" role="alert"></p>
    <div class="dl-table-wrap"><table class="dl-table"><thead><tr><th><input id="dlSelectAll" type="checkbox" aria-label="选择当前列表全部任务"></th><th>文件名</th><th>大小</th><th>状态 / 进度</th><th>速度</th><th>剩余时间</th><th>连接</th><th>代理</th><th>操作</th></tr></thead><tbody id="dlTasks"></tbody></table><div id="dlEmpty" class="dl-empty"><div>↓</div><h3>添加你的第一个下载</h3><p>粘贴链接，选择代理和连接数，即可开始下载。</p><button class="button primary" id="dlEmptyAdd">＋ 新建下载</button></div></div>
    <div class="dl-statusbar"><span id="dlTotals">0 个任务</span><span>最多同时下载 30 个任务 · 每任务最多 16 个连接</span><span id="dlSpeed">↓ 0 B/s</span></div>
    <div id="dlDetails" class="dl-details hidden"></div>
    <p class="field-hint dl-note">默认只删除任务记录；勾选删除文件时移入可恢复的回收区。多连接需要服务器支持 Range；节点代理遵循该节点的出口策略。</p>
  </section>`);
  document.body.insertAdjacentHTML('beforeend',`<dialog id="dlDialog" aria-labelledby="dlTitle"><form id="dlForm">
    <div class="dialog-heading"><div><h2 id="dlTitle">新建下载</h2><p>下载到运行 Mirroxy 的这台电脑。</p></div><button type="button" class="icon-button" id="dlCancel" aria-label="关闭下载弹窗">×</button></div>
    <label class="field">下载地址<input id="dlURL" type="url" required autocomplete="off" placeholder="https://example.com/file.zip"></label>
    <div class="dl-form-grid"><label class="field">文件名（留空自动识别）<input id="dlName" maxlength="240" placeholder="由服务器或链接决定"></label><label class="field">保存目录<input id="dlDir" required placeholder="绝对路径，例如 D:\\Downloads"></label></div>
    <div class="dl-form-grid"><label class="field">代理方式<select id="dlProxyMode"><option value="direct">直连（不使用系统代理）</option><option value="node">Mirroxy 节点</option><option value="custom">自定义 HTTP 代理</option></select></label><label class="field hidden" id="dlNodeField">代理节点<select id="dlNode"></select></label><label class="field hidden" id="dlProxyField">代理地址<input id="dlProxy" autocomplete="off" placeholder="http://127.0.0.1:10808"></label></div>
    <div class="dl-form-grid"><label class="field">每任务最大连接数<select id="dlConnections">${[1,2,4,8,16].map(n=>`<option value="${n}">${n} ${n===8?'（默认）':''}</option>`).join('')}</select></label><label class="field">限速（KiB/s；0 不限速）<input id="dlLimit" type="number" min="0" max="1048576" value="0" required></label></div>
    <p class="field-hint" id="dlEditHint">只显示已启动节点。不会自动启动节点，也不会在代理故障时切换为直连。自定义代理使用 HTTP，可填 http://用户名:密码@主机:端口。</p>
    <label class="dl-later"><input type="checkbox" id="dlLater">仅添加，稍后下载</label><p id="dlFormError" class="form-result hidden" role="alert"></p>
    <div class="dialog-footer"><span>支持 HTTP / HTTPS / FTP</span><button class="button primary" id="dlSave" type="submit">开始下载</button></div>
  </form></dialog>`);

  function rows() { return data.tasks.filter(task => (filter==='all'||(filter==='active'?['active','waiting'].includes(task.status):task.status===filter)) && `${task.name} ${task.path} ${task.url}`.toLowerCase().includes(query)); }
  const contextMenu=document.createElement('div');contextMenu.className='dl-context-menu hidden';contextMenu.setAttribute('role','menu');contextMenu.setAttribute('aria-label',text('下载任务右键菜单'));document.body.append(contextMenu);
  let menuTaskIDs=[];
  new MutationObserver(()=>document.body.classList.toggle('downloads-page',!dl('view-downloads').classList.contains('hidden'))).observe(dl('view-downloads'),{attributes:true,attributeFilter:['class']});
  function closeMenu(){contextMenu.classList.add('hidden')}
  function showMenu(id,x,y){
    if(id&&!selected.has(id)){selected=new Set([id]);draw()}
    menuTaskIDs=id?[...selected]:[];
    const tasks=data.tasks.filter(task=>menuTaskIDs.includes(task.id));
    const allowed=MirroxyDownloadMenu.allowed(tasks);
    const button=(verb,label,enabled=true)=>`<button type="button" role="menuitem" data-dl-menu="${verb}" ${enabled?'':'disabled'}>${esc(text(label))}</button>`;
    contextMenu.innerHTML=id?`<div class="dl-menu-caption">${tasks.length} ${text('个任务')}</div>${button('open-file','打开文件',allowed.openFile)}${button('open-folder','打开所在文件夹',allowed.openFolder)}<div class="dl-menu-separator"></div>${button('resume','继续',allowed.resume)}${button('pause','暂停',allowed.pause)}${button('reconnect','断点重连',allowed.reconnect)}${button('retry','重试',allowed.retry)}${button('edit','编辑',allowed.edit)}<div class="dl-menu-separator"></div>${button('copyURL','复制下载链接',allowed.copyURL)}${button('copyPath','复制保存路径',allowed.copyPath)}<div class="dl-menu-separator"></div>${button('remove','删除记录',allowed.remove)}<small>${text('保留已下载文件和断点数据')}</small>`:`${button('new','新建下载')}${button('selectAll','全选')}${button('pauseAll','全部暂停')}${button('refresh','刷新')}`;
    contextMenu.classList.remove('hidden');
    contextMenu.style.maxHeight=Math.max(80,window.innerHeight-16)+'px';
    const point=MirroxyDownloadMenu.position(x,y,contextMenu.offsetWidth,contextMenu.offsetHeight,window.innerWidth,window.innerHeight);
    contextMenu.style.left=point.left+'px';contextMenu.style.top=point.top+'px';
    contextMenu.querySelector('button:not(:disabled)')?.focus();
  }
  dl('view-downloads').addEventListener('contextmenu',event=>{if(event.target.closest('input:not([type="checkbox"]),textarea,select'))return;event.preventDefault();const row=event.target.closest('[data-dl-id]');showMenu(row?.dataset.dlId,event.clientX,event.clientY)});
  contextMenu.addEventListener('click',async event=>{const button=event.target.closest('[data-dl-menu]');if(!button||button.disabled)return;const verb=button.dataset.dlMenu;const ids=[...menuTaskIDs];const task=data.tasks.find(task=>task.id===ids[0]);closeMenu();try{
    if(verb==='new'){open();return}
    if(verb==='selectAll'){selected=new Set(rows().map(task=>task.id));draw();return}
    if(verb==='pauseAll'){await action(data.tasks.filter(task=>['active','waiting'].includes(task.status)).map(task=>task.id),'pause');return}
    if(verb==='refresh'){await load();return}
    if(verb==='edit'){if(task)open(task);return}
    if(verb==='copyURL'||verb==='copyPath'){if(task){await copyText(verb==='copyURL'?task.url:task.path||task.dir);toast(text('已复制'))}return}
    await action(ids,verb);
  }catch(error){toast(error.message,true)}});
  contextMenu.addEventListener('keydown',event=>{const items=[...contextMenu.querySelectorAll('button:not(:disabled)')];const index=items.indexOf(document.activeElement);if(event.key==='Escape'){closeMenu();return}if(['ArrowDown','ArrowUp','Home','End'].includes(event.key)){event.preventDefault();const next=event.key==='Home'?0:event.key==='End'?items.length-1:(index+(event.key==='ArrowDown'?1:-1)+items.length)%items.length;items[next]?.focus()}});
  document.addEventListener('pointerdown',event=>{if(!contextMenu.contains(event.target))closeMenu()});document.addEventListener('keydown',event=>{if(event.key==='Escape')closeMenu()});window.addEventListener('resize',closeMenu);window.addEventListener('scroll',event=>{if(!contextMenu.contains(event.target))closeMenu()},true);
  function filename(task) { if(task.path)return task.path.split(/[\\/]/).pop();if(task.name)return task.name;try{return decodeURIComponent(new URL(task.url).pathname.split('/').pop())||task.url}catch{return task.url} }
  function proxyLabel(task) { if(task.proxyMode==='node')return state?.nodes.find(n=>n.id===task.nodeId)?.name||text('节点已移除');if(task.proxyMode==='custom'){try{return new URL(task.proxyUrl).host}catch{return text('自定义代理')}}return text('直连'); }
  function eta(task) { const speed=Number(task.downloadSpeed),remaining=Number(task.totalLength)-Number(task.completedLength);if(task.status!=='active'||speed<=0||remaining<=0)return '—';const sec=Math.ceil(remaining/speed);return sec<60?sec+' s':sec<3600?Math.ceil(sec/60)+' min':(sec/3600).toFixed(1)+' h'; }
  function draw() {
    selected = new Set([...selected].filter(id=>data.tasks.some(task=>task.id===id)));
    const list=rows();
    dl('dlTasks').innerHTML=list.map(task=>{const percent=MirroxyDownloadProgress.percent(task.completedLength,task.totalLength);return `<tr data-dl-id="${esc(task.id)}" class="${selected.has(task.id)?'selected':''}" tabindex="0"><td><input type="checkbox" data-dl-check="${esc(task.id)}" ${selected.has(task.id)?'checked':''} aria-label="选择 ${esc(filename(task))}"></td><td class="dl-file"><span class="dl-file-icon">${task.status==='complete'?'✓':'↓'}</span><span title="${esc(task.url)}">${esc(filename(task))}</span></td><td>${size(task.totalLength)}</td><td><div class="dl-progress-label"><span class="dl-status ${esc(task.status)}">${esc(text(labels[task.status]||task.status))}</span><span>${percent.toFixed(1)}%</span></div><progress class="dl-progress ${esc(task.status)}" value="${percent.toFixed(2)}" max="100" aria-label="${percent.toFixed(1)}%"></progress></td><td>${size(task.downloadSpeed)}/s</td><td>${eta(task)}</td><td>${esc(task.connectionsUsed||'0')} / ${task.connections}</td><td class="dl-proxy" title="${esc(proxyLabel(task))}">${esc(proxyLabel(task))}</td><td><div class="dl-row-actions">${task.status!=='complete'?`<button class="button small ${['active','waiting'].includes(task.status)?'':'primary'}" data-dl-action="${['active','waiting'].includes(task.status)?'pause':task.status==='error'?'retry':'resume'}" data-id="${esc(task.id)}">${text(['active','waiting'].includes(task.status)?'暂停':task.status==='error'?'重试':'继续')}</button>`:''}<button class="icon-button" data-dl-action="edit" data-id="${esc(task.id)}" aria-label="编辑 ${esc(filename(task))}" title="${text('编辑')}">✎</button></div></td></tr>`}).join('');
    dl('dlEmpty').classList.toggle('hidden',list.length>0);
    for(const task of list){if(task.stalled){const status=dl('dlTasks').querySelector(`[data-dl-id="${CSS.escape(task.id)}"] .dl-status`);if(status){status.textContent=text('暂无速度，可断点重连');status.classList.add('stalled')}}}
    dl('dlEmpty').querySelector('h3').textContent=text(data.tasks.length?'没有匹配的任务':'添加你的第一个下载');
    dl('dlEmpty').querySelector('p').textContent=text(data.tasks.length?'试试其他状态或搜索词。':'粘贴链接，选择代理和连接数，即可开始下载。');
    dl('downloadNavCount').textContent=data.tasks.filter(task=>['active','waiting'].includes(task.status)).length;
    dl('dlTotals').textContent=data.tasks.length+' '+text('个任务')+' · '+data.tasks.filter(task=>task.status==='active').length+' '+text('下载中');
    dl('dlSpeed').textContent='↓ '+size(data.tasks.reduce((sum,task)=>sum+Number(task.downloadSpeed||0),0))+'/s';
    dl('dlEngine').textContent=text(!data.available?'aria2 未安装':data.running?'aria2 运行中':'aria2 待命');
    dl('dlEngine').classList.toggle('running',data.running);
    dl('dlInstall').classList.toggle('hidden',!!data.available);
    dl('dlSelection').textContent=selected.size?text('已选')+' '+selected.size+' '+text('项'):text('未选择任务');
    dl('dlSelectAll').checked=list.length>0&&list.every(task=>selected.has(task.id));
    dl('dlSelectAll').indeterminate=list.some(task=>selected.has(task.id))&&!dl('dlSelectAll').checked;
    // Background refresh must not disable actions on the user's selection.
    document.querySelectorAll('[data-dl-bulk]').forEach(button=>button.disabled=!selected.size);
    dl('dlEdit').disabled=selected.size!==1;
    const task=data.tasks.find(task=>selected.has(task.id));
    dl('dlDetails').classList.toggle('hidden',!task);
    if(task)dl('dlDetails').innerHTML=`<div><strong>${esc(filename(task))}</strong><span>${esc(text(labels[task.status]||task.status))}</span></div><dl><dt>${text('保存位置')}</dt><dd>${esc(task.path||task.dir)}</dd><dt>${text('下载地址')}</dt><dd>${esc(task.url)}</dd><dt>${text('已下载')}</dt><dd>${size(task.completedLength)} / ${size(task.totalLength)} · ${text('代理')} ${esc(proxyLabel(task))}</dd>${task.error?`<dt>${text('错误')}</dt><dd class="dl-red">${esc(task.error)}</dd>`:''}</dl>`;
    if(task?.stalled)dl('dlDetails').insertAdjacentHTML('beforeend',`<p class="dl-stall-hint">${text('已连续 45 秒没有下载进度，可保留断点重连。')}<button class="button small" data-dl-reconnect="${esc(task.id)}">${text('断点重连')}</button></p>`);
    if(task)dl('dlDetails').insertAdjacentHTML('beforeend',`<div class="dl-detail-actions"><button class="button small" data-dl-open="open-file" data-id="${esc(task.id)}" ${task.status==='complete'&&task.path?'':'disabled'}>${text('打开文件')}</button><button class="button small" data-dl-open="open-folder" data-id="${esc(task.id)}">${text('打开所在文件夹')}</button></div>`);
  }
  async function load() { if(loading||dl('view-downloads').classList.contains('hidden')||!state?.localAccess)return;loading=true;try{data=await apiGet('downloads');dl('dlError').classList.add('hidden');draw()}catch(error){dl('dlError').textContent=error.message;dl('dlError').classList.remove('hidden')}finally{loading=false;draw()} }
  function updateNodes() {const id=dl('dlNode').value;const nodes=(state?.nodes||[]).filter(node=>node.status==='running'||node.id===id);dl('dlNode').innerHTML='<option value="">'+text('选择已启动的节点')+'</option>'+nodes.map(node=>`<option value="${esc(node.id)}" ${node.status!=='running'?'disabled':''}>${esc(node.name)} · 127.0.0.1:${node.port}${node.status!=='running'?' · '+text('未启动'):''}</option>`).join('');dl('dlNode').value=id;}
  function proxyFields() {dl('dlNodeField').classList.toggle('hidden',dl('dlProxyMode').value!=='node');dl('dlProxyField').classList.toggle('hidden',dl('dlProxyMode').value!=='custom');dl('dlProxy').required=dl('dlProxyMode').value==='custom';dl('dlNode').required=dl('dlProxyMode').value==='node';}
  function open(task) {editing=task||null;dl('dlTitle').textContent=text(task?'编辑下载任务':'新建下载');dl('dlSave').textContent=text(task?'保存修改':'开始下载');dl('dlURL').value=task?.url||'';dl('dlName').value=task?.name||'';dl('dlDir').value=task?.dir||data.defaultDir;dl('dlName').readOnly=!!task;dl('dlDir').readOnly=!!task;dl('dlProxyMode').value=task?.proxyMode||'direct';updateNodes();dl('dlNode').value=task?.nodeId||'';dl('dlProxy').value=task?.proxyUrl||'';dl('dlConnections').value=task?.connections||8;dl('dlLimit').value=task?.limitKiB||0;dl('dlLater').checked=false;dl('dlLater').closest('label').classList.toggle('hidden',!!task);dl('dlFormError').classList.add('hidden');dl('dlEditHint').textContent=text(task?'编辑任务前请先暂停。文件名和目录保持不变；更换链接时必须指向相同文件，避免损坏断点数据。保存后继续保持暂停，点击「继续」开始。':'只显示已启动节点。不会自动启动节点，也不会在代理故障时切换为直连。自定义代理使用 HTTP，可填 http://用户名:密码@主机:端口。');proxyFields();dl('dlDialog').showModal();dl('dlURL').focus();}
  document.body.insertAdjacentHTML('beforeend',`<dialog id="dlDeleteDialog" aria-labelledby="dlDeleteTitle"><form id="dlDeleteForm"><div class="dialog-heading"><div><h2 id="dlDeleteTitle">删除下载任务</h2><p id="dlDeleteSummary"></p></div><button type="button" class="icon-button" id="dlDeleteClose" aria-label="关闭删除弹窗">×</button></div><ul id="dlDeleteList" class="dl-delete-list"></ul><label class="dl-delete-option"><input type="checkbox" id="dlDeleteFiles"><span>同时删除下载文件和断点数据<small>移入保存目录的 .Mirroxy-trash 回收区，可手动恢复。</small></span></label><p class="field-hint">默认只删除任务记录，保留所有已下载文件。勾选时只处理该任务关联的文件，不删除目录。</p><p id="dlDeleteWarning" class="dl-delete-warning hidden">文件将从原保存位置移入回收区。</p><div class="dialog-footer"><button type="button" class="button" id="dlDeleteCancel">取消</button><button type="submit" class="button dl-danger" id="dlDeleteConfirm">删除记录</button></div></form></dialog>`);
  let deleteResolve=null;
  function confirmDelete(ids){dl('dlDeleteFiles').checked=false;dl('dlDeleteWarning').classList.add('hidden');dl('dlDeleteConfirm').textContent=text('删除记录');dl('dlDeleteSummary').textContent=text('即将删除')+' '+ids.length+' '+text('个下载任务');dl('dlDeleteList').innerHTML=data.tasks.filter(task=>ids.includes(task.id)).map(task=>`<li>${esc(filename(task))}</li>`).join('');return new Promise(resolve=>{deleteResolve=resolve;dl('dlDeleteDialog').showModal();dl('dlDeleteCancel').focus()})}
  dl('dlDeleteFiles').onchange=()=>{const checked=dl('dlDeleteFiles').checked;dl('dlDeleteWarning').classList.toggle('hidden',!checked);dl('dlDeleteConfirm').textContent=text(checked?'删除记录及文件':'删除记录')};
  dl('dlDeleteCancel').onclick=dl('dlDeleteClose').onclick=()=>dl('dlDeleteDialog').close();
  dl('dlDeleteDialog').addEventListener('close',()=>{if(deleteResolve){const resolve=deleteResolve;deleteResolve=null;resolve(null)}});
  dl('dlDeleteForm').onsubmit=event=>{event.preventDefault();const resolve=deleteResolve;deleteResolve=null;const files=dl('dlDeleteFiles').checked;dl('dlDeleteDialog').close();resolve?.(files)};
  async function action(ids,verb) {if(!ids.length)return;let deleteFiles=false;if(verb==='remove'){const answer=await confirmDelete(ids);if(answer===null)return;deleteFiles=answer}const failures=[];for(const id of ids){const task=data.tasks.find(t=>t.id===id);if(!task)continue;if(verb==='pause'&&!['active','waiting'].includes(task.status))continue;if(['resume','retry'].includes(verb)&&task.status==='complete')continue;try{await api('downloads/action',{id,action:verb,deleteFiles})}catch(error){failures.push(error.message)}}if(failures.length)toast([...new Set(failures)].join('\n'),true);await load();}
  nav.onclick=()=>{view('downloads');activate()};
  function activate() {dl('pageName').textContent=text('下载管理');load();}
  dl('dlAdd').onclick=dl('dlEmptyAdd').onclick=()=>open();
  dl('dlCancel').onclick=()=>dl('dlDialog').close();dl('dlProxyMode').onchange=proxyFields;
  dl('dlEdit').onclick=()=>open(data.tasks.find(task=>selected.has(task.id)));
  dl('dlRefresh').onclick=load;
  dl('dlDetails').addEventListener('click',event=>{const button=event.target.closest('[data-dl-open],[data-dl-reconnect]');if(!button||button.disabled)return;action([button.dataset.dlReconnect||button.dataset.id],button.dataset.dlReconnect?'reconnect':button.dataset.dlOpen)});
  dl('dlInstall').onclick=async()=>{const button=dl('dlInstall');button.disabled=true;button.textContent=text('正在下载并校验引擎…');try{await api('downloads/install',{});await load()}catch(error){toast(error.message,true)}finally{button.disabled=false;button.innerHTML='<span>↓</span>'+text('安装 aria2 引擎')}};
  dl('dlPauseAll').onclick=()=>action(data.tasks.filter(task=>['active','waiting'].includes(task.status)).map(task=>task.id),'pause');
  document.querySelectorAll('[data-dl-bulk]').forEach(button=>button.onclick=()=>action([...selected],button.dataset.dlBulk));
  dl('dlSearch').oninput=()=>{query=dl('dlSearch').value.trim().toLowerCase();draw()};dl('dlFilter').onchange=()=>{filter=dl('dlFilter').value;draw()};
  dl('dlSelectAll').onchange=()=>{rows().forEach(task=>dl('dlSelectAll').checked?selected.add(task.id):selected.delete(task.id));draw()};
  dl('dlTasks').addEventListener('click',event=>{const actionButton=event.target.closest('[data-dl-action]');if(actionButton){const task=data.tasks.find(task=>task.id===actionButton.dataset.id);if(actionButton.dataset.dlAction==='edit')open(task);else action([task.id],actionButton.dataset.dlAction);return}const row=event.target.closest('[data-dl-id]');if(!row)return;if(event.target.matches('[data-dl-check]')){event.target.checked?selected.add(row.dataset.dlId):selected.delete(row.dataset.dlId)}else if(event.ctrlKey||event.metaKey){selected.has(row.dataset.dlId)?selected.delete(row.dataset.dlId):selected.add(row.dataset.dlId)}else{selected=new Set([row.dataset.dlId])}draw()});
  dl('dlTasks').addEventListener('dblclick',event=>{if(event.target.closest('button,input'))return;const row=event.target.closest('[data-dl-id]');if(row){const task=data.tasks.find(task=>task.id===row.dataset.dlId);if(task?.status==='complete')action([task.id],'open-file');else open(task)}});
  dl('dlTasks').addEventListener('keydown',event=>{const row=event.target.closest('[data-dl-id]');if(!row)return;if(event.key==='ContextMenu'||event.key==='F10'&&event.shiftKey){event.preventDefault();const rect=row.getBoundingClientRect();showMenu(row.dataset.dlId,rect.left+24,rect.top+16)}else if(event.key==='Enter'){selected=new Set([row.dataset.dlId]);draw()}});
  dl('dlForm').onsubmit=async event=>{event.preventDefault();const button=dl('dlSave');button.disabled=true;try{const spec={url:dl('dlURL').value.trim(),name:dl('dlName').value.trim(),dir:dl('dlDir').value.trim(),proxyMode:dl('dlProxyMode').value,nodeId:dl('dlNode').value,proxyUrl:dl('dlProxy').value.trim(),connections:Number(dl('dlConnections').value),limitKiB:Number(dl('dlLimit').value)};await api(editing?'downloads/action':'downloads/add',editing?{...spec,id:editing.id,action:'edit'}:{...spec,paused:dl('dlLater').checked});dl('dlDialog').close();await load()}catch(error){dl('dlFormError').textContent=error.message;dl('dlFormError').classList.remove('hidden')}finally{button.disabled=false}};
  window.MirroxyDownloads={activate,onState(next){nav.classList.toggle('hidden',!next.localAccess);if(dl('dlDialog').open)updateNodes();if(!dl('view-downloads').classList.contains('hidden'))dl('pageName').textContent=text('下载管理')}};
  timer=setInterval(load,1000);window.addEventListener('pagehide',()=>clearInterval(timer));
  window.addEventListener('MirroxyLanguageChanged',draw);
  if(state)window.MirroxyDownloads.onState(state);
})();
