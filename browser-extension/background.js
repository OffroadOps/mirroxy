'use strict';

const DEFAULTS = {
  enabled: true,
  url: 'https://127.0.0.1:9950',
  token: '',
  autoIntercept: true,
  skipHosts: ''
};

async function settings() {
  return { ...DEFAULTS, ...(await chrome.storage.local.get(DEFAULTS)) };
}

function baseURL(url) {
  return String(url || '').trim().replace(/\/+$/, '');
}

async function request(cfg, path, body) {
  const response = await fetch(baseURL(cfg.url) + '/api/downloads/connector/' + path, {
    method: body === undefined ? 'GET' : 'POST',
    cache: 'no-store',
    headers: {
      'X-Mirroxy-Download-Token': cfg.token,
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' })
    },
    body: body === undefined ? undefined : JSON.stringify(body)
  });
  let result = {};
  try { result = await response.json(); } catch {}
  if (!response.ok || result.error) {
    throw new Error(result.error || ('HTTP ' + response.status));
  }
  return result;
}

function fileNameFor(item) {
  const source = item.filename || item.finalUrl || item.url || '';
  try {
    const path = new URL(source).pathname;
    const name = decodeURIComponent(path.split('/').pop() || '');
    if (name) return name;
  } catch {}
  const parts = String(source).split(/[\\/]/);
  return parts[parts.length - 1] || '';
}

function skippedHost(url, skipHosts) {
  let host = '';
  try { host = new URL(url).hostname.toLowerCase(); } catch { return false; }
  return String(skipHosts || '')
    .split(/[\s,;]+/)
    .map(v => v.trim().toLowerCase())
    .filter(Boolean)
    .some(rule => host === rule || host.endsWith('.' + rule));
}

function notify(title, message) {
  try {
    chrome.notifications.create('', {
      type: 'basic',
      iconUrl: 'icons/icon128.png',
      title,
      message: String(message || '').slice(0, 500)
    }, () => void chrome.runtime.lastError);
  } catch {}
}

const lastNotice = new Map();
function notifyThrottled(key, title, message) {
  const now = Date.now();
  if (now - (lastNotice.get(key) || 0) < 30000) return;
  lastNotice.set(key, now);
  notify(title, message);
}

function isPanelURL(url, panelURL) {
  try {
    const a = new URL(url);
    const b = new URL(baseURL(panelURL));
    const local = value => value === '127.0.0.1' || value === 'localhost';
    return local(a.hostname) && local(b.hostname) && a.port === b.port;
  } catch { return false; }
}

async function relay(url, name) {
  const cfg = await settings();
  if (!cfg.token) {
    const error = new Error('尚未粘贴 Mirroxy 连接配置');
    error.code = 'unconfigured';
    throw error;
  }
  return request(cfg, 'add', { url, name });
}

async function cancelDownload(id) {
  try { await chrome.downloads.cancel(id); } catch {}
  try { await chrome.downloads.erase({ id }); } catch {}
}

chrome.downloads.onCreated.addListener(async item => {
  try {
    const cfg = await settings();
    if (!cfg.enabled || !cfg.autoIntercept || !cfg.token) return;
    const url = item.finalUrl || item.url || '';
    if (!/^(https?|ftp):/i.test(url)) return;
    if (item.byExtensionId) return;
    // The panel itself serves the extension zip; never intercept that download.
    if (isPanelURL(url, cfg.url)) return;
    if (skippedHost(url, cfg.skipHosts)) return;
    // Never race a download that already completed while the settings loaded.
    const [current] = await chrome.downloads.search({ id: item.id });
    if (!current || current.state !== 'in_progress') return;
    await relay(url, fileNameFor(current));
    await cancelDownload(item.id);
    notify('已交给 Mirroxy 下载', fileNameFor(current) || url);
  } catch (error) {
    if (error.code === 'unconfigured') {
      notifyThrottled('unconfigured', 'Mirroxy 下载接力尚未配置', '打开扩展选项，粘贴 Mirroxy 下载管理页生成的连接配置。浏览器下载已保留。');
    } else {
      notifyThrottled(String(error.message || error), 'Mirroxy 未接收下载', (error.message || error) + '\n浏览器下载已保留。');
    }
  }
});

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({ id: 'mirroxy-link', title: '使用 Mirroxy 下载此链接', contexts: ['link'] });
    chrome.contextMenus.create({ id: 'mirroxy-page', title: '使用 Mirroxy 下载当前页面', contexts: ['page'] });
  });
});

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const url = info.menuItemId === 'mirroxy-link' ? info.linkUrl : info.pageUrl;
  if (!url || !/^(https?|ftp):/i.test(url)) return;
  try {
    await relay(url, '');
    notify('已交给 Mirroxy 下载', url);
  } catch (error) {
    notify('Mirroxy 未接收下载', error.message || String(error));
  }
});

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  (async () => {
    const cfg = await settings();
    if (message?.type === 'ping') {
      return request(cfg, 'ping', {});
    }
    if (message?.type === 'relay') {
      return request(cfg, 'add', { url: message.url, name: message.name || '' });
    }
    if (message?.type === 'settings') {
      return cfg;
    }
    throw new Error('unknown message');
  })().then(result => sendResponse({ ok: true, result }), error => sendResponse({ ok: false, error: error.message || String(error) }));
  return true;
});
