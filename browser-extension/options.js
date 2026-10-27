'use strict';

const $ = id => document.getElementById(id);

function send(message) {
  return new Promise(resolve => chrome.runtime.sendMessage(message, response => resolve(response || { ok: false, error: '扩展后台未响应' })));
}

function validLocalURL(value) {
  try {
    const url = new URL(value);
    return (url.protocol === 'https:' || url.protocol === 'http:') &&
      (url.hostname === '127.0.0.1' || url.hostname === 'localhost') && url.port &&
      url.pathname === '/' && !url.search && !url.hash;
  } catch { return false; }
}

let loadedURL = '';
let savedConnector = '';
let importedConnector = '';
let imported = false;

async function load() {
  const response = await send({ type: 'settings' });
  if (!response.ok) {
    $('status').textContent = '自动连接失败：' + response.error;
    $('status').className = 'status bad';
    return;
  }
  const cfg = response.result;
  loadedURL = cfg.url || '';
  savedConnector = cfg.connector || '';
  $('url').value = loadedURL;
  $('token').value = cfg.token || '';
  $('enabled').checked = !!cfg.enabled;
  $('autoIntercept').checked = !!cfg.autoIntercept;
  $('skipHosts').value = cfg.skipHosts || '';
  if (cfg.token) {
    $('status').textContent = '已自动连接本机 Mirroxy';
    $('status').className = 'status ok';
    $('panelInfo').textContent = '面板：' + cfg.url + (cfg.connector ? ' · 接力地址：' + cfg.connector : '');
    const ping = await send({ type: 'ping' });
    if (!ping.ok) {
      $('status').textContent = '连接需要刷新：' + ping.error;
      $('status').className = 'status bad';
    }
  } else {
    $('status').textContent = '未找到本机 Mirroxy，请先启动 Mirroxy 后台';
    $('status').className = 'status bad';
    $('panelInfo').textContent = '';
  }
}

async function save(show = true) {
  const url = $('url').value.trim().replace(/\/+$/, '') || loadedURL;
  const token = $('token').value.trim();
  if (url && !validLocalURL(url)) {
    $('saveStatus').textContent = '地址必须是本机 http(s)://127.0.0.1:端口 或 localhost';
    $('saveStatus').className = 'status bad';
    return false;
  }
  const connector = imported ? importedConnector : (url === loadedURL ? savedConnector : '');
  await chrome.storage.local.set({
    url,
    connector,
    token,
    enabled: $('enabled').checked,
    autoIntercept: $('autoIntercept').checked,
    skipHosts: $('skipHosts').value
  });
  loadedURL = url;
  savedConnector = connector;
  imported = false;
  if (show) {
    $('saveStatus').textContent = '已保存';
    $('saveStatus').className = 'status ok';
  }
  return true;
}

$('connect').onclick = async () => {
  $('status').textContent = '正在查找本机 Mirroxy…';
  $('status').className = 'status';
  const response = await send({ type: 'connect' });
  if (response.ok) {
    await load();
    $('status').textContent = '已自动连接 Mirroxy ' + (response.result.version || '');
    $('status').className = 'status ok';
  } else {
    $('status').textContent = '自动连接失败：' + response.error + '。请确认 Mirroxy 正在运行，或使用下方手动配置。';
    $('status').className = 'status bad';
  }
};

$('importConfig').onclick = () => {
  try {
    const data = JSON.parse($('configJson').value);
    if (data.url) $('url').value = String(data.url);
    if (data.token) $('token').value = String(data.token);
    importedConnector = data.connector ? String(data.connector) : '';
    imported = true;
    $('configJson').value = '';
    $('manualStatus').textContent = '已导入，请点击保存';
    $('manualStatus').className = 'status ok';
  } catch {
    $('manualStatus').textContent = '配置 JSON 无效';
    $('manualStatus').className = 'status bad';
  }
};

$('save').onclick = () => save(true);

$('test').onclick = async () => {
  if (!(await save(false))) return;
  $('manualStatus').textContent = '正在连接…';
  $('manualStatus').className = 'status';
  const response = await send({ type: 'ping' });
  if (response.ok) {
    $('manualStatus').textContent = '连接成功：Mirroxy ' + (response.result.version || '');
    $('manualStatus').className = 'status ok';
  } else {
    $('manualStatus').textContent = '连接失败：' + response.error;
    $('manualStatus').className = 'status bad';
  }
};

load();
