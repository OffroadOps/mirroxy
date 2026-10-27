'use strict';

const $ = id => document.getElementById(id);

function send(message) {
  return new Promise(resolve => chrome.runtime.sendMessage(message, response => resolve(response || { ok: false, error: '扩展后台未响应' })));
}

async function refresh() {
  const response = await send({ type: 'settings' });
  if (!response.ok) return;
  const cfg = response.result;
  $('enabled').checked = !!cfg.enabled;
  $('autoIntercept').checked = !!cfg.autoIntercept;
  $('openPanel').dataset.url = cfg.url || '';
  if (!cfg.token) {
    $('status').textContent = '未配置：请在 Mirroxy 下载页复制连接配置';
    $('statusDot').className = 'dot warn';
    $('hint').classList.remove('hidden');
    return;
  }
  const ping = await send({ type: 'ping' });
  if (ping.ok) {
    $('status').textContent = '已连接 Mirroxy ' + (ping.result.version || '');
    $('statusDot').className = 'dot ok';
    $('hint').classList.add('hidden');
  } else {
    $('status').textContent = '无法连接：' + ping.error;
    $('statusDot').className = 'dot bad';
    $('hint').classList.remove('hidden');
  }
}

$('enabled').onchange = () => chrome.storage.local.set({ enabled: $('enabled').checked });
$('autoIntercept').onchange = () => chrome.storage.local.set({ autoIntercept: $('autoIntercept').checked });
$('options').onclick = () => chrome.runtime.openOptionsPage();
$('openPanel').onclick = () => {
  const url = $('openPanel').dataset.url;
  if (url) chrome.tabs.create({ url });
};
refresh();
