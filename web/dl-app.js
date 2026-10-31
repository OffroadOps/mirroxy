'use strict';
// Download window opened from the tray: a classic download-manager layout over
// the existing /api/downloads endpoints. The page polls the task list every
// second and the panel state (token, nodes) every few seconds.
(() => {
  const $ = id => document.getElementById(id);
  const M = MirroxyDownloadModel;
  const t = (key, params) => window.MirroxyI18n?.t(key, params) || key;
  const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const bytes = n => { n = Number(n) || 0; const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']; let i = 0; while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; } return (i ? n.toFixed(n >= 100 ? 0 : 1) : Math.round(n)) + ' ' + units[i]; };
  const icons = { compressed: 't-compressed', documents: 't-documents', music: 't-music', programs: 't-programs', video: 't-video', other: 't-other' };

  let app = null, token = '', data = { tasks: [], defaultDir: '', available: true, running: false };
  let node = 'all', sort = { key: 'created', dir: 'desc' }, query = '', selected = new Set(), cursor = '', anchor = '', visible = [], loading = false, infoID = '', lastRows = null, toastTimer;
  try { const saved = JSON.parse(localStorage.getItem('Mirroxy.downloadWindow') || '{}'); if (typeof saved.node === 'string') node = saved.node; if (saved.sort?.key) sort = saved.sort; } catch {}
  const save = () => { try { localStorage.setItem('Mirroxy.downloadWindow', JSON.stringify({ node, sort })); } catch {} };

  function toast(message, error = false) { clearTimeout(toastTimer); const el = $('dmToast'); el.textContent = message; el.className = 'dm-toast' + (error ? ' error' : ''); toastTimer = setTimeout(() => el.classList.add('hidden'), error ? 8000 : 3500); }
  async function request(path, body) {
    const response = await fetch('/api/' + path, body === undefined ? { cache: 'no-store' } : { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-App-Token': token }, body: JSON.stringify(body) });
    let result; try { result = await response.json(); } catch { throw Error(t(response.status === 403 ? '下载管理目前仅允许本机访问' : '后台响应异常，请刷新页面')); }
    if (!response.ok || result.error) throw Error(t(result.error || '操作未完成'));
    return result;
  }
  async function copyText(text) { if (navigator.clipboard?.writeText) return navigator.clipboard.writeText(text); const input = document.createElement('textarea'); input.value = text; document.body.append(input); input.select(); document.execCommand('copy'); input.remove(); }

  const byID = id => data.tasks.find(task => task.id === id);
  const selectedTasks = () => [...selected].map(byID).filter(Boolean);
  function proxyLabel(task) {
    if (task.proxyMode === 'node') return app?.nodes.find(n => n.id === task.nodeId)?.name || t('节点已移除');
    if (task.proxyMode === 'custom') { try { return new URL(task.proxyUrl).host; } catch { return t('自定义代理'); } }
    return t('直连');
  }
  const created = value => value ? String(value).replace('T', ' ').slice(0, 16) : '';

  // Rendering --------------------------------------------------------------
  function render() {
    selected = new Set([...selected].filter(byID));
    const counts = M.counts(data.tasks);
    document.querySelectorAll('[data-count]').forEach(el => { const n = counts[el.dataset.count] || 0; el.textContent = n ? n : ''; });
    document.querySelectorAll('.dm-node').forEach(b => b.classList.toggle('active', b.dataset.node === node));
    const list = M.sortTasks(M.filterTasks(data.tasks, node, query), sort);
    visible = list.map(task => task.id);
    $('dmHead').querySelectorAll('[data-sort]').forEach(th => th.setAttribute('aria-sort', th.dataset.sort === sort.key ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none'));
    // Rewrite the rows only when they changed, so a double-click is not split
    // across two different row elements by the one-second refresh.
    const html = list.map(row).join('');
    if (html !== lastRows) {
      lastRows = html; $('dmTasks').innerHTML = html;
      $('dmTasks').querySelectorAll('[data-progress]').forEach(bar => { bar.style.width = bar.dataset.progress + '%'; });
    }
    $('dmEmpty').classList.toggle('hidden', list.length > 0);
    $('dmEmptyText').textContent = t(data.tasks.length ? '这个分类里没有任务' : '还没有下载任务');
    const active = data.tasks.filter(task => task.status === 'active').length;
    $('dmTotals').textContent = t('{count} 个任务 · {active} 个下载中', { count: data.tasks.length, active });
    $('dmSelection').textContent = selected.size ? t('已选 {count} 项', { count: selected.size }) : '';
    $('dmSpeed').textContent = '↓ ' + bytes(data.tasks.reduce((sum, task) => sum + (Number(task.downloadSpeed) || 0), 0)) + '/s';
    const engine = $('dmEngine');
    engine.textContent = t(!data.available ? 'aria2 未安装' : data.running ? 'aria2 运行中' : 'aria2 待命');
    engine.className = 'dm-engine' + (!data.available ? ' missing' : data.running ? ' running' : '');
    $('dmBanner').classList.toggle('hidden', data.available !== false);
    $('dmBannerText').textContent = t('还没有安装 aria2 下载引擎，安装后才能开始下载。');
    const allowed = MirroxyDownloadMenu.allowed(selectedTasks());
    document.querySelector('[data-bulk="resume"]').disabled = !(allowed.resume || allowed.retry);
    document.querySelector('[data-bulk="pause"]').disabled = !allowed.pause;
    document.querySelector('[data-bulk="remove"]').disabled = !allowed.remove;
    document.querySelector('[data-bulk="reconnect"]').disabled = !allowed.reconnect;
    $('dmFolder').disabled = !allowed.openFolder;
    $('dmPauseAll').disabled = !data.tasks.some(task => ['active', 'waiting'].includes(task.status));
    $('dmClean').disabled = !data.tasks.some(task => task.status === 'complete');
    renderInfo();
  }
  function row(task) {
    const name = M.filename(task), p = M.percent(task), status = task.stalled && task.status === 'active' ? 'stalled' : task.status;
    const bar = ['active', 'paused', 'waiting'].includes(task.status) && p > 0 ? `<i data-progress="${p.toFixed(1)}"></i>` : '';
    const live = task.status === 'active';
    const classes = [selected.has(task.id) && 'selected', task.id === cursor && 'cursor'].filter(Boolean).join(' ');
    return `<tr data-id="${esc(task.id)}" class="${classes}" aria-selected="${selected.has(task.id)}" title="${esc(task.url)}"><td><span class="dm-name"><svg><use href="#${icons[M.category(name)]}"/></svg><span>${esc(name)}</span></span></td><td class="num">${Number(task.totalLength) ? bytes(task.totalLength) : '<span class="dm-muted">—</span>'}</td><td><span class="dm-st ${esc(status)}">${bar}<b>${esc(M.statusText(task, t))}</b></span></td><td>${esc(M.etaText(task))}</td><td class="num">${live ? bytes(task.downloadSpeed) + '/s' : ''}</td><td title="${esc(proxyLabel(task))}">${esc(proxyLabel(task))}</td><td>${esc(created(task.created))}</td><td title="${esc(task.path || task.dir)}">${esc(task.dir || '')}</td></tr>`;
  }
  function renderInfo() {
    if (!$('dmInfoDialog').open) return;
    const task = byID(infoID);
    if (!task) { $('dmInfoDialog').close(); return; }
    const p = M.percent(task);
    $('dmInfoTitle').textContent = M.statusText(task, t) + ' · ' + M.filename(task);
    const props = [['地址', task.url], ['状态', M.statusText(task, t)], ['文件大小', Number(task.totalLength) ? bytes(task.totalLength) : t('未知')], ['已下载', bytes(task.completedLength) + ' (' + p.toFixed(2) + '%)'], ['传输速率', task.status === 'active' ? bytes(task.downloadSpeed) + '/s' : '—'], ['剩余时间', M.etaText(task) || '—'], ['连接', (task.connectionsUsed || 0) + ' / ' + task.connections], ['代理', proxyLabel(task)], ['保存位置', task.path || task.dir], ['添加时间', created(task.created)]];
    $('dmInfoProps').innerHTML = props.map(([k, v]) => `<dt>${esc(t(k))}</dt><dd>${esc(v)}</dd>`).join('');
    $('dmInfoProgress').value = p;
    $('dmInfoError').classList.toggle('hidden', !task.error && !task.stalled);
    $('dmInfoError').textContent = task.error || (task.stalled ? t('已连续 45 秒没有下载进度，可保留断点重连。') : '');
    const running = ['active', 'waiting'].includes(task.status);
    $('dmInfoToggle').textContent = t(running ? '暂停' : task.status === 'error' || task.status === 'removed' ? '重试' : '继续');
    $('dmInfoToggle').disabled = task.status === 'complete';
    $('dmInfoReconnect').disabled = task.status === 'complete';
    $('dmInfoOpen').disabled = !(task.status === 'complete' && task.path);
  }

  // Data -------------------------------------------------------------------
  async function loadState() {
    try { app = await request('state'); token = app.token; if (!app.localAccess) { $('dmBanner').classList.remove('hidden'); $('dmBannerText').textContent = t('下载管理目前仅允许本机访问'); $('dmInstall').classList.add('hidden'); } } catch {}
  }
  async function loadTasks() {
    if (loading || document.hidden) return; loading = true;
    try { data = await request('downloads'); render(); } catch (error) { $('dmEngine').textContent = error.message; } finally { loading = false; }
  }

  // Actions ----------------------------------------------------------------
  let deleteResolve = null;
  function confirmDelete(tasks) {
    $('dmDeleteFiles').checked = false;
    $('dmDeleteSummary').textContent = t('删除 {count} 个下载任务？默认只删除任务记录，保留已下载的文件。', { count: tasks.length });
    $('dmDeleteList').innerHTML = tasks.map(task => `<li>${esc(M.filename(task))}</li>`).join('');
    return new Promise(resolve => { deleteResolve = resolve; $('dmDeleteDialog').showModal(); $('dmDeleteCancel').focus(); });
  }
  async function action(ids, verb) {
    let tasks = ids.map(byID).filter(Boolean); if (!tasks.length) return;
    let deleteFiles = false;
    if (verb === 'remove') { const answer = await confirmDelete(tasks); if (answer === null) return; deleteFiles = answer; }
    if (verb === 'resume') tasks = tasks.filter(task => task.status !== 'complete');
    if (verb === 'pause') tasks = tasks.filter(task => ['active', 'waiting'].includes(task.status));
    const failures = [];
    for (const task of tasks) {
      // One "continue" covers paused and failed tasks.
      const real = verb === 'resume' && ['error', 'removed'].includes(task.status) ? 'retry' : verb;
      try { await request('downloads/action', { id: task.id, action: real, deleteFiles }); } catch (error) { failures.push(error.message); }
    }
    if (failures.length) toast([...new Set(failures)].join('\n'), true);
    await loadTasks();
  }
  function openFolder(id) { action([id], 'open-folder'); }

  // Selection --------------------------------------------------------------
  function paint() { $('dmTasks').querySelectorAll('tr[data-id]').forEach(tr => { const on = selected.has(tr.dataset.id); tr.classList.toggle('selected', on); tr.setAttribute('aria-selected', on); tr.classList.toggle('cursor', tr.dataset.id === cursor); }); render(); }
  function select(id, event) {
    if (event?.shiftKey && anchor) { const a = visible.indexOf(anchor), b = visible.indexOf(id); selected = new Set(visible.slice(Math.min(a, b), Math.max(a, b) + 1)); }
    else if (event?.ctrlKey || event?.metaKey) { selected.has(id) ? selected.delete(id) : selected.add(id); anchor = id; }
    else { selected = new Set([id]); anchor = id; }
    cursor = id; paint();
  }
  function move(step, extend) {
    if (!visible.length) return;
    const index = visible.indexOf(cursor), next = visible[Math.max(0, Math.min(visible.length - 1, step === Infinity ? visible.length - 1 : step === -Infinity ? 0 : (index < 0 ? 0 : index + step)))];
    if (extend) { if (!anchor) anchor = cursor || next; const a = visible.indexOf(anchor), b = visible.indexOf(next); selected = new Set(visible.slice(Math.min(a, b), Math.max(a, b) + 1)); } else { selected = new Set([next]); anchor = next; }
    cursor = next; paint(); $('dmTasks').querySelector(`tr[data-id="${CSS.escape(next)}"]`)?.scrollIntoView({ block: 'nearest' });
  }

  // Popup menus (context menu and menu bar share one element) ---------------
  const popup = $('dmPopup');
  let popupVerbs = {};
  function showPopup(items, x, y, trigger) {
    popupVerbs = {};
    popup.innerHTML = items.map((item, i) => {
      if (item === '-') return '<hr>';
      popupVerbs[i] = item.run;
      return `<button type="button" role="${item.checked === undefined ? 'menuitem' : 'menuitemradio'}"${item.checked === undefined ? '' : ` aria-checked="${item.checked}"`} data-i="${i}" class="${item.danger ? 'danger' : ''}" ${item.enabled === false ? 'disabled' : ''}><span>${esc(t(item.label))}</span>${item.key ? `<kbd>${item.key}</kbd>` : ''}</button>`;
    }).join('');
    popup.classList.remove('hidden');
    const point = MirroxyDownloadMenu.position(x, y, popup.offsetWidth, popup.offsetHeight, innerWidth, innerHeight);
    popup.style.left = point.left + 'px'; popup.style.top = point.top + 'px';
    document.querySelectorAll('.dm-menu-trigger').forEach(b => b.classList.toggle('open', b === trigger));
    if (!trigger) popup.querySelector('button:not(:disabled)')?.focus();
  }
  function closePopup() { popup.classList.add('hidden'); document.querySelectorAll('.dm-menu-trigger').forEach(b => b.classList.remove('open')); }
  popup.addEventListener('click', e => { const b = e.target.closest('button[data-i]'); if (!b || b.disabled) return; const run = popupVerbs[b.dataset.i]; closePopup(); run?.(); });
  popup.addEventListener('keydown', e => { const items = [...popup.querySelectorAll('button:not(:disabled)')], i = items.indexOf(document.activeElement); if (e.key === 'Escape') { closePopup(); $('dmList').focus(); } else if (['ArrowDown', 'ArrowUp'].includes(e.key)) { e.preventDefault(); items[(i + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length]?.focus(); } });
  document.addEventListener('pointerdown', e => { if (!popup.contains(e.target) && !e.target.closest('.dm-menu-trigger')) closePopup(); });
  addEventListener('resize', closePopup); addEventListener('blur', closePopup);

  function taskMenu() {
    const tasks = selectedTasks(), allowed = MirroxyDownloadMenu.allowed(tasks), one = tasks[0], ids = tasks.map(task => task.id);
    return [
      { label: '打开文件', enabled: allowed.openFile, key: 'Enter', run: () => action(ids, 'open-file') },
      { label: '打开所在文件夹', enabled: allowed.openFolder, run: () => openFolder(one.id) },
      '-',
      { label: '继续', enabled: allowed.resume || allowed.retry, run: () => action(ids, 'resume') },
      { label: '暂停', enabled: allowed.pause, key: 'Space', run: () => action(ids, 'pause') },
      { label: '断点重连', enabled: allowed.reconnect, run: () => action(ids, 'reconnect') },
      '-',
      { label: '复制下载链接', enabled: allowed.copyURL, run: () => copyText(one.url).then(() => toast(t('已复制'))) },
      { label: '复制保存路径', enabled: allowed.copyPath, run: () => copyText(one.path || one.dir).then(() => toast(t('已复制'))) },
      '-',
      { label: '删除', enabled: allowed.remove, key: 'Del', danger: true, run: () => action(ids, 'remove') },
      '-',
      { label: '属性', enabled: tasks.length === 1, key: 'Alt+Enter', run: () => openInfo(one.id) },
    ];
  }
  const menus = {
    task: () => [
      { label: '新建任务', key: 'Ctrl+N', run: () => openAdd() },
      { label: '从剪贴板添加链接', key: 'Ctrl+V', run: addFromClipboard },
      '-',
      { label: '全部暂停', enabled: !$('dmPauseAll').disabled, run: pauseAll },
      { label: '删除已完成', enabled: !$('dmClean').disabled, run: cleanFinished },
      '-',
      { label: '关闭窗口', run: () => window.close() },
    ],
    download: taskMenu,
    view: () => {
      const dark = document.documentElement.dataset.theme === 'dark', lang = window.MirroxyI18n?.preference || 'auto';
      return [
        { label: '深色模式', checked: dark, run: () => setTheme(dark ? 'light' : 'dark') },
        '-',
        { label: '跟随系统语言', checked: lang === 'auto', run: () => setLanguage('auto') },
        { label: '简体中文', checked: lang === 'zh-CN', run: () => setLanguage('zh-CN') },
        { label: 'English', checked: lang === 'en', run: () => setLanguage('en') },
        '-',
        { label: '刷新', key: 'F5', run: () => { loadState(); loadTasks(); } },
      ];
    },
    help: () => [
      { label: '打开 Mirroxy 主面板', run: () => window.open('/', '_blank') },
      { label: '安装浏览器接力扩展', run: () => window.open('/#browser-relay', '_blank') },
    ],
  };
  function setTheme(value) { document.documentElement.dataset.theme = value; try { localStorage.setItem('Mirroxy.theme', value); } catch {} }
  function setLanguage(value) { try { localStorage.setItem('Mirroxy.language', value); } catch {} location.reload(); }
  document.querySelectorAll('.dm-menu-trigger').forEach(b => {
    b.addEventListener('click', () => { if (b.classList.contains('open')) { closePopup(); return; } const r = b.getBoundingClientRect(); showPopup(menus[b.dataset.menu](), r.left, r.bottom + 2, b); });
    b.addEventListener('mouseenter', () => { if (!popup.classList.contains('hidden') && document.querySelector('.dm-menu-trigger.open') && !b.classList.contains('open')) b.click(); });
  });

  // Dialogs ----------------------------------------------------------------
  function updateNodes() {
    const current = $('dmNode').value, running = (app?.nodes || []).filter(n => n.status === 'running');
    $('dmNode').innerHTML = `<option value="">${esc(t('选择已启动的节点'))}</option>` + running.map(n => `<option value="${esc(n.id)}">${esc(n.name)} · 127.0.0.1:${n.port}</option>`).join('');
    $('dmNode').value = running.some(n => n.id === current) ? current : '';
  }
  function proxyFields() { const mode = $('dmProxyMode').value; $('dmNodeField').classList.toggle('hidden', mode !== 'node'); $('dmProxyField').classList.toggle('hidden', mode !== 'custom'); $('dmNode').required = mode === 'node'; $('dmProxy').required = mode === 'custom'; }
  function openAdd(url = '') {
    $('dmURL').value = url; $('dmName').value = ''; $('dmDir').value = $('dmDir').value || data.defaultDir || '';
    updateNodes(); proxyFields(); $('dmAddError').classList.add('hidden'); $('dmAddDialog').showModal(); (url ? $('dmStart') : $('dmURL')).focus();
  }
  async function submitAdd(paused) {
    const form = $('dmAddForm'); if (!form.reportValidity()) return;
    const spec = { url: $('dmURL').value.trim(), name: $('dmName').value.trim(), dir: $('dmDir').value.trim(), proxyMode: $('dmProxyMode').value, nodeId: $('dmNode').value, proxyUrl: $('dmProxy').value.trim(), connections: Number($('dmConnections').value), limitKiB: 0 };
    $('dmStart').disabled = $('dmLater').disabled = true;
    try { await request('downloads/add', { ...spec, paused }); $('dmAddDialog').close(); toast(t(paused ? '任务已添加，稍后可点击「继续」开始' : '已开始下载')); await loadTasks(); }
    catch (error) { $('dmAddError').textContent = error.message; $('dmAddError').classList.remove('hidden'); }
    finally { $('dmStart').disabled = $('dmLater').disabled = false; }
  }
  async function addFromClipboard() {
    let text = '';
    try { text = await navigator.clipboard.readText(); } catch {}
    openAdd(M.pastedURL(text));
  }
  function openInfo(id) { if (!byID(id)) return; infoID = id; $('dmInfoDialog').showModal(); renderInfo(); $('dmInfoClose').focus(); }
  function pauseAll() { action(data.tasks.filter(task => ['active', 'waiting'].includes(task.status)).map(task => task.id), 'pause'); }
  async function cleanFinished() {
    const done = data.tasks.filter(task => task.status === 'complete');
    if (!done.length || !confirm(t('删除 {count} 个已完成任务的记录？已下载的文件会保留。', { count: done.length }))) return;
    for (const task of done) { try { await request('downloads/action', { id: task.id, action: 'remove', deleteFiles: false }); } catch (error) { toast(error.message, true); break; } }
    await loadTasks();
  }

  // Wiring -----------------------------------------------------------------
  $('dmAdd').onclick = () => openAdd();
  $('dmPauseAll').onclick = pauseAll;
  $('dmClean').onclick = cleanFinished;
  $('dmFolder').onclick = () => { const task = selectedTasks()[0]; if (task) openFolder(task.id); };
  $('dmPanel').onclick = () => window.open('/', '_blank');
  document.querySelectorAll('[data-bulk]').forEach(b => b.onclick = () => action([...selected], b.dataset.bulk));
  $('dmInstall').onclick = async () => { const b = $('dmInstall'); b.disabled = true; b.textContent = t('正在下载并校验引擎…'); try { await request('downloads/install', {}); await loadTasks(); } catch (error) { toast(error.message, true); } finally { b.disabled = false; b.textContent = t('安装 aria2 引擎'); } };
  document.querySelectorAll('.dm-node').forEach(b => b.onclick = () => { node = b.dataset.node; save(); render(); });
  $('dmHead').addEventListener('click', e => { const th = e.target.closest('[data-sort]'); if (!th) return; sort = M.nextSort(sort, th.dataset.sort); save(); render(); });
  $('dmSearch').addEventListener('input', () => { query = $('dmSearch').value; render(); });
  $('dmTasks').addEventListener('mousedown', e => { if (e.shiftKey) e.preventDefault(); });
  $('dmTasks').addEventListener('click', e => { const tr = e.target.closest('tr[data-id]'); if (tr) { select(tr.dataset.id, e); $('dmList').focus({ preventScroll: true }); } });
  $('dmTasks').addEventListener('dblclick', e => { const tr = e.target.closest('tr[data-id]'); if (!tr) return; const task = byID(tr.dataset.id); if (task?.status === 'complete' && task.path) action([task.id], 'open-file'); else openInfo(tr.dataset.id); });
  $('dmList').addEventListener('click', e => { if (!e.target.closest('tr[data-id]') && !e.target.closest('thead')) { selected.clear(); paint(); } });
  $('dmList').addEventListener('contextmenu', e => {
    e.preventDefault(); const tr = e.target.closest('tr[data-id]');
    if (tr) { if (!selected.has(tr.dataset.id)) select(tr.dataset.id); showPopup(taskMenu(), e.clientX, e.clientY); }
    else showPopup(menus.task().slice(0, 5), e.clientX, e.clientY);
  });
  document.addEventListener('keydown', e => {
    if (document.querySelector('dialog[open]') || !popup.classList.contains('hidden')) return;
    const key = e.key, lower = key.length === 1 ? key.toLowerCase() : key, mod = e.ctrlKey || e.metaKey;
    if (e.target.closest?.('input,select,textarea')) { if (key === 'Escape' && e.target === $('dmSearch')) { $('dmSearch').value = ''; query = ''; render(); $('dmList').focus(); } return; }
    if (mod && lower === 'n') { e.preventDefault(); openAdd(); return; }
    if (mod && lower === 'f') { e.preventDefault(); $('dmSearch').focus(); return; }
    if (mod && lower === 'a') { e.preventDefault(); selected = new Set(visible); paint(); return; }
    if (key === 'F5') { e.preventDefault(); loadState(); loadTasks(); return; }
    if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(key)) { e.preventDefault(); move({ ArrowDown: 1, ArrowUp: -1, Home: -Infinity, End: Infinity }[key], e.shiftKey); return; }
    if (e.target.closest?.('button') && (key === 'Enter' || key === ' ')) return;
    const tasks = selectedTasks(); if (!tasks.length) return;
    const current = byID(selected.has(cursor) ? cursor : tasks[0].id);
    if (key === 'Delete') { e.preventDefault(); action(tasks.map(task => task.id), 'remove'); }
    else if (key === 'Enter' && e.altKey) { e.preventDefault(); openInfo(current.id); }
    else if (key === 'Enter') { e.preventDefault(); if (current.status === 'complete' && current.path) action([current.id], 'open-file'); else openInfo(current.id); }
    else if (key === ' ') { e.preventDefault(); action(tasks.map(task => task.id), tasks.some(task => ['active', 'waiting'].includes(task.status)) ? 'pause' : 'resume'); }
    else if (key === 'ContextMenu' || (key === 'F10' && e.shiftKey)) { e.preventDefault(); const r = $('dmTasks').querySelector(`tr[data-id="${CSS.escape(current.id)}"]`)?.getBoundingClientRect(); if (r) showPopup(taskMenu(), r.left + 40, r.bottom); }
  });
  // Ctrl+V in the window starts a new download from a copied link.
  document.addEventListener('paste', e => {
    if (document.querySelector('dialog[open]') || e.target.closest?.('input,textarea')) return;
    const url = M.pastedURL(e.clipboardData?.getData('text') || ''); if (!url) return;
    e.preventDefault(); openAdd(url);
  });
  $('dmAddForm').onsubmit = e => { e.preventDefault(); submitAdd(false); };
  $('dmLater').onclick = () => submitAdd(true);
  $('dmAddCancel').onclick = () => $('dmAddDialog').close();
  $('dmProxyMode').onchange = proxyFields;
  $('dmInfoClose').onclick = () => $('dmInfoDialog').close();
  $('dmInfoToggle').onclick = () => { const task = byID(infoID); if (task) action([task.id], ['active', 'waiting'].includes(task.status) ? 'pause' : 'resume'); };
  $('dmInfoReconnect').onclick = () => action([infoID], 'reconnect');
  $('dmInfoOpen').onclick = () => action([infoID], 'open-file');
  $('dmInfoFolder').onclick = () => openFolder(infoID);
  $('dmDeleteFiles').onchange = () => { $('dmDeleteConfirm').textContent = t($('dmDeleteFiles').checked ? '删除记录及文件' : '删除'); };
  $('dmDeleteCancel').onclick = () => $('dmDeleteDialog').close();
  $('dmDeleteDialog').addEventListener('close', () => { if (deleteResolve) { const resolve = deleteResolve; deleteResolve = null; resolve(null); } $('dmDeleteConfirm').textContent = t('删除'); });
  $('dmDeleteForm').onsubmit = e => { e.preventDefault(); const resolve = deleteResolve, files = $('dmDeleteFiles').checked; deleteResolve = null; $('dmDeleteDialog').close(); resolve?.(files); };

  loadState().then(loadTasks);
  setInterval(loadTasks, 1000);
  setInterval(loadState, 5000);
  document.addEventListener('visibilitychange', () => { if (!document.hidden) loadTasks(); });
})();
