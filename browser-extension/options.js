'use strict';

const $ = id => document.getElementById(id);
const DEFAULTS = { enabled: true, url: 'https://127.0.0.1:9950', token: '', autoIntercept: true, skipHosts: '' };

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

async function load() {
  const cfg = { ...DEFAULTS, ...(await chrome.storage.local.get(DEFAULTS)) };
  $('url').value = cfg.url;
  $('token').value = cfg.token;
  $('enabled').checked = !!cfg.enabled;
  $('autoIntercept').checked = !!cfg.autoIntercept;
  $('skipHosts').value = cfg.skipHosts || '';
}

async function save(show = true) {
  const value = $('url').value.trim().replace(/\/+$/, '');
  if (!validLocalURL(value)) {
    $('status').textContent = '地址必须是本机 http(s)://127.0.0.1:端口 或 localhost';
    $('status').className = 'status bad';
    return false;
  }
  await chrome.storage.local.set({
    url: value,
    token: $('token').value.trim(),
    enabled: $('enabled').checked,
    autoIntercept: $('autoIntercept').checked,
    skipHosts: $('skipHosts').value
  });
  if (show) {
    $('status').textContent = '已保存';
    $('status').className = 'status ok';
  }
  return true;
}

$('importConfig').onclick = async () => {
  try {
    const data = JSON.parse($('configJson').value);
    if (data.url) $('url').value = String(data.url);
    if (data.token) $('token').value = String(data.token);
    $('configJson').value = '';
    $('status').textContent = '已导入，请点击保存';
    $('status').className = 'status ok';
  } catch {
    $('status').textContent = '配置 JSON 无效';
    $('status').className = 'status bad';
  }
};

$('save').onclick = () => save(true);

$('test').onclick = async () => {
  if (!(await save(false))) return;
  $('status').textContent = '正在连接…';
  $('status').className = 'status';
  const response = await send({ type: 'ping' });
  if (response.ok) {
    $('status').textContent = '连接成功：Mirroxy ' + (response.result.version || '');
    $('status').className = 'status ok';
  } else {
    $('status').textContent = '连接失败：' + response.error;
    $('status').className = 'status bad';
  }
};

load();
