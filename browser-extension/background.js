'use strict';

const DEFAULTS = {
  enabled: true,
  url: 'https://127.0.0.1:9950',
  connector: '',
  token: '',
  autoIntercept: true,
  skipHosts: ''
};
const LEGACY_DISCOVERY = 'http://127.0.0.1:15060';

async function settings() {
  return { ...DEFAULTS, ...(await chrome.storage.local.get(DEFAULTS)) };
}

function baseURL(url) {
  return String(url || '').trim().replace(/\/+$/, '');
}

function endpoint(cfg) {
  return baseURL(cfg.connector || cfg.url);
}

async function request(cfg, path, body) {
  const base = endpoint(cfg);
  if (!base) throw new Error('尚未连接 Mirroxy');
  const response = await fetch(base + '/api/downloads/connector/' + path, {
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

// Zero-configuration pairing: the legacy plain-HTTP port answers on loopback,
// so this works before the local CA is trusted and without copying a token.
async function discover() {
  const saved = await chrome.storage.local.get(['url', 'connector']);
  const candidates = [LEGACY_DISCOVERY + '/api/downloads/connector/auto'];
  if (saved.connector) candidates.push(baseURL(saved.connector) + '/api/downloads/connector/auto');
  if (saved.url) candidates.push(baseURL(saved.url) + '/api/downloads/connector/auto');
  for (const address of [...new Set(candidates)]) {
    try {
      const response = await fetch(address, { cache: 'no-store', signal: AbortSignal.timeout(1500) });
      if (!response.ok) continue;
      const data = await response.json();
      if (!data.token || !data.url || !data.connector) continue;
      await chrome.storage.local.set({ url: data.url, connector: data.connector, token: data.token });
      return { ...DEFAULTS, ...(await chrome.storage.local.get(DEFAULTS)) };
    } catch {}
  }
  return null;
}

async function ensureConnected(force = false) {
  const cfg = await settings();
  if (!force && cfg.token && cfg.connector) return cfg;
  return (await discover()) || cfg;
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

// Hand the browser cookie jar, user agent and referrer to the local engine so
// signed URLs and anti-hotlink CDNs behave like they do in the browser.
async function browserHeaders(url, referer) {
  const headers = { referer: referer || '', userAgent: navigator.userAgent, cookie: '' };
  if (!/^https?:/i.test(url)) return headers;
  try {
    const cookies = await chrome.cookies.getAll({ url });
    headers.cookie = cookies.map(cookie => cookie.name + '=' + cookie.value).join('; ');
  } catch {}
  return headers;
}

async function relay(url, name, referer) {
  let cfg = await ensureConnected();
  if (!cfg.token) {
    const error = new Error('未找到本机 Mirroxy，请确认 Mirroxy 正在运行');
    error.code = 'unconfigured';
    throw error;
  }
  const body = { url, name: name || '', headers: await browserHeaders(url, referer) };
  try {
    return await request(cfg, 'add', body);
  } catch (error) {
    // The panel port may have changed; re-pair once and retry.
    cfg = (await discover()) || cfg;
    if (!cfg.token) throw error;
    return request(cfg, 'add', body);
  }
}

async function cancelDownload(id) {
  try { await chrome.downloads.cancel(id); } catch {}
  try { await chrome.downloads.erase({ id }); } catch {}
}

chrome.downloads.onCreated.addListener(async item => {
  try {
    const cfg = await settings();
    if (!cfg.enabled || !cfg.autoIntercept) return;
    const url = item.finalUrl || item.url || '';
    if (!/^(https?|ftp):/i.test(url)) return;
    if (item.byExtensionId) return;
    if (isPanelURL(url, cfg.url)) return;
    if (skippedHost(url, cfg.skipHosts)) return;
    // Never race a download that already completed while the settings loaded.
    const [current] = await chrome.downloads.search({ id: item.id });
    if (!current || current.state !== 'in_progress') return;
    await relay(url, fileNameFor(current), current.referrer || '');
    await cancelDownload(item.id);
    notify('已交给 Mirroxy 下载', fileNameFor(current) || url);
  } catch (error) {
    if (error.code === 'unconfigured') {
      notifyThrottled('unconfigured', '未连接 Mirroxy', '请先启动 Mirroxy 后台；浏览器下载已保留。');
    } else {
      notifyThrottled(String(error.message || error), 'Mirroxy 未接收下载', (error.message || error) + '\n浏览器下载已保留。');
    }
  }
});

chrome.runtime.onInstalled.addListener(() => {
  discover().catch(() => {});
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({ id: 'mirroxy-link', title: '使用 Mirroxy 下载此链接', contexts: ['link'] });
    chrome.contextMenus.create({ id: 'mirroxy-page', title: '使用 Mirroxy 下载当前页面', contexts: ['page'] });
  });
});

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const url = info.menuItemId === 'mirroxy-link' ? info.linkUrl : info.pageUrl;
  if (!url || !/^(https?|ftp):/i.test(url)) return;
  try {
    await relay(url, '', tab?.url || '');
    notify('已交给 Mirroxy 下载', url);
  } catch (error) {
    notify('Mirroxy 未接收下载', error.message || String(error));
  }
});

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  (async () => {
    if (message?.type === 'connect') {
      const cfg = await ensureConnected(true);
      if (!cfg.token) throw new Error('未找到本机 Mirroxy');
      return request(cfg, 'ping', {});
    }
    if (message?.type === 'ping') {
      let cfg = await ensureConnected();
      if (!cfg.token) throw new Error('未找到本机 Mirroxy');
      try {
        return await request(cfg, 'ping', {});
      } catch (error) {
        cfg = (await discover()) || cfg;
        if (!cfg.token) throw error;
        return request(cfg, 'ping', {});
      }
    }
    if (message?.type === 'settings') {
      const cfg = await ensureConnected();
      return cfg;
    }
    if (message?.type === 'relay') {
      return relay(message.url, message.name || '', message.referer || '');
    }
    throw new Error('unknown message');
  })().then(result => sendResponse({ ok: true, result }), error => sendResponse({ ok: false, error: error.message || String(error) }));
  return true;
});

// Best-effort pairing on browser startup.
discover().catch(() => {});
