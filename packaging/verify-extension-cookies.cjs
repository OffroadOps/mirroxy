'use strict';
// A/B check for the extension cookie permission.
//
// Chrome only returns cookies for URLs covered by a host permission. The old
// manifest listed only localhost, so chrome.cookies.getAll() failed for an
// external download site and the relayed download lost its sign-in state.
// The new manifest asks for <all_urls>.
//
// This loads the extension twice - once with each manifest - and compares
// what chrome.cookies can actually see for an external site, running the probe
// inside the extension service worker exactly like background.js does.
//
// Chrome 137 and later ignore --load-extension, so the extension is installed
// through the CDP Extensions.loadUnpacked command instead.
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const CHROME = process.env.CHROME_BIN || 'C:/Program Files/Google/Chrome/Application/chrome.exe';
const SOURCE = process.argv[2] || 'D:/Mirroxy/browser-extension';

const sleep = ms => new Promise(r => setTimeout(r, ms));

const OLD_HOSTS = [
  'https://127.0.0.1/*',
  'https://localhost/*',
  'http://127.0.0.1/*',
  'http://localhost/*'
];

const PROBE = `(async () => {
  const site = 'https://downloads.example.com/';
  let setResult = 'ok';
  try {
    await chrome.cookies.set({ url: site, name: 'sid', value: 'signed-in-token', path: '/' });
  } catch (e) { setResult = 'error: ' + e.message; }
  try {
    const cookies = await chrome.cookies.getAll({ url: site });
    return { setResult, readable: true, count: cookies.length,
             names: cookies.map(c => c.name), values: cookies.map(c => c.value) };
  } catch (e) {
    return { setResult, readable: false, error: e.message };
  }
})()`;

async function connect(url) {
  const ws = new WebSocket(url);
  await new Promise((resolve, reject) => {
    ws.onopen = resolve;
    ws.onerror = () => reject(new Error('websocket error'));
  });
  let id = 0;
  const pending = new Map();
  ws.onmessage = event => {
    const msg = JSON.parse(event.data);
    if (msg.id && pending.has(msg.id)) {
      const { resolve, reject } = pending.get(msg.id);
      pending.delete(msg.id);
      msg.error ? reject(new Error(JSON.stringify(msg.error))) : resolve(msg.result);
    }
  };
  return {
    send(method, params) {
      const myId = ++id;
      ws.send(JSON.stringify({ id: myId, method, params: params || {} }));
      return new Promise((resolve, reject) => pending.set(myId, { resolve, reject }));
    },
    close() { ws.close(); }
  };
}

// Writes a copy of the extension with the given host permissions.
function stageExtension(hostPermissions) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'mirroxy-ext-'));
  const target = path.join(dir, 'ext');
  fs.cpSync(SOURCE, target, { recursive: true });
  const manifestPath = path.join(target, 'manifest.json');
  const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
  manifest.host_permissions = hostPermissions;
  fs.writeFileSync(manifestPath, JSON.stringify(manifest, null, 2));
  return { dir, target };
}

async function runVariant(label, hostPermissions, port) {
  const { dir, target } = stageExtension(hostPermissions);
  const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'mirroxy-prof-'));
  const child = spawn(CHROME, [
    `--user-data-dir=${profile}`,
    `--remote-debugging-port=${port}`,
    '--headless=new',
    '--no-first-run',
    '--no-default-browser-check',
    '--disable-gpu',
    'about:blank'
  ], { stdio: 'ignore' });

  try {
    let version = null;
    for (let attempt = 0; attempt < 60 && !version; attempt++) {
      await sleep(500);
      try {
        version = await (await fetch(`http://127.0.0.1:${port}/json/version`)).json();
      } catch {}
    }
    if (!version) throw new Error('the browser never opened its debugging port');

    const browser = await connect(version.webSocketDebuggerUrl);
    const loaded = await browser.send('Extensions.loadUnpacked', { path: target });
    const id = loaded.id;
    browser.close();
    if (!id) throw new Error('Extensions.loadUnpacked returned no id');

    // The probe runs in the extension service worker, the same context that
    // background.js uses to collect cookies for a relayed download.
    let worker = null;
    for (let attempt = 0; attempt < 40 && !worker; attempt++) {
      await sleep(500);
      const list = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
      worker = list.find(t => t.type === 'service_worker' && t.url.includes(id));
    }
    if (!worker) throw new Error('the extension service worker never appeared');

    const cdp = await connect(worker.webSocketDebuggerUrl);
    await cdp.send('Runtime.enable');
    const result = await cdp.send('Runtime.evaluate', {
      expression: PROBE,
      awaitPromise: true,
      returnByValue: true
    });
    if (result.exceptionDetails) {
      throw new Error('probe failed: ' + JSON.stringify(result.exceptionDetails.exception?.description || result.exceptionDetails));
    }
    cdp.close();
    const value = result.result.value;
    console.log(label + ': ' + JSON.stringify(value));
    return value;
  } finally {
    child.kill();
    await sleep(800);
    try { fs.rmSync(dir, { recursive: true, force: true }); } catch {}
    try { fs.rmSync(profile, { recursive: true, force: true }); } catch {}
  }
}

async function main() {
  const before = await runVariant('old manifest (localhost only)', OLD_HOSTS, 19260);
  const after = await runVariant('new manifest', ['<all_urls>'], 19261);

  const sawCookie = value => value && value.readable && value.count > 0 && value.values.includes('signed-in-token');
  if (sawCookie(before)) {
    console.log('UNEXPECTED: the old manifest already read external cookies, the A/B comparison proves nothing');
    process.exitCode = 1;
    return;
  }
  if (!sawCookie(after)) {
    console.log('FAILED: the new manifest still cannot read external cookies');
    process.exitCode = 1;
    return;
  }
  console.log('PASS: only the new manifest can read the external site cookie');
}

main().catch(err => {
  console.error('FAILED: ' + err.message);
  process.exit(1);
});
