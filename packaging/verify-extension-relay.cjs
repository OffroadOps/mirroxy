'use strict';
// End-to-end check of the browser relay with the real extension in a real
// Chrome: the extension pairs itself with a running Mirroxy over the local
// 15060 port, hands over a download URL together with the cookies it read for
// that site, and Mirroxy is expected to accept the task.
//
// The download site is a local HTTP server that refuses the request unless the
// Cookie header carries the value the extension was able to read.
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const http = require('node:http');
const os = require('node:os');
const path = require('node:path');
const tls = require('node:tls');

// The panel serves HTTPS with its own local CA, so the fetch calls below have
// to trust that CA instead of the system store.
process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';

const CHROME = process.env.CHROME_BIN || 'C:/Program Files/Google/Chrome/Application/chrome.exe';
const SOURCE = process.argv[2] || 'D:/Mirroxy/browser-extension';
const PANEL_PORT = Number(process.env.PANEL_PORT || 19960);
const CDP_PORT = Number(process.env.CDP_PORT || 19270);

const sleep = ms => new Promise(r => setTimeout(r, ms));

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

// A download site that only serves the file to a request carrying the cookie.
function startDownloadSite() {
  const seen = { cookie: null, userAgent: null, referer: null, hits: 0 };
  const server = http.createServer((req, res) => {
    seen.hits++;
    seen.cookie = req.headers.cookie || null;
    seen.userAgent = req.headers['user-agent'] || null;
    seen.referer = req.headers.referer || null;
    if (!/sid=signed-in-token/.test(seen.cookie || '')) {
      res.writeHead(403, { 'Content-Type': 'text/plain' });
      res.end('missing cookie');
      return;
    }
    res.writeHead(200, { 'Content-Type': 'application/octet-stream' });
    res.end('file-content');
  });
  return new Promise(resolve => {
    server.listen(0, '127.0.0.1', () => {
      resolve({ server, port: server.address().port, seen });
    });
  });
}

async function main() {
  // A fresh Mirroxy run directory, so the real installation is untouched.
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'mirroxy-run-'));
  const binary = path.resolve(__dirname, '..', 'Mirroxy.exe');
  if (!fs.existsSync(binary)) {
    console.error('SKIPPED: build Mirroxy.exe first');
    process.exit(0);
  }
  // The download engine has to be in place before Mirroxy starts, otherwise
  // the first download attempt finds no aria2.
  const runtime = path.join(root, 'bin', 'runtime');
  fs.mkdirSync(runtime, { recursive: true });
  const downloader = path.resolve(__dirname, '..', 'bin', 'runtime', 'Mirroxy_downloader.exe');
  if (fs.existsSync(downloader)) fs.copyFileSync(downloader, path.join(runtime, 'Mirroxy_downloader.exe'));

  const app = spawn(binary, ['-dir', root, '-panel', String(PANEL_PORT), '-no-open'], { stdio: 'ignore' });

  try {
    // Wait for the panel to answer the connector info request.
    let paired = null;
    for (let attempt = 0; attempt < 60 && !paired; attempt++) {
      await sleep(500);
      try {
        const res = await fetch(`https://127.0.0.1:${PANEL_PORT}/api/downloads/connector`);
        if (res.ok) paired = await res.json();
      } catch {}
    }
    if (!paired) throw new Error('Mirroxy never answered the connector request');
    console.log('paired: ' + JSON.stringify({ url: paired.url, connector: paired.connector }));

    const site = await startDownloadSite();
    console.log('download site: http://127.0.0.1:' + site.port + '/file.bin');

    const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'mirroxy-prof-'));
    const child = spawn(CHROME, [
      `--user-data-dir=${profile}`,
      `--remote-debugging-port=${CDP_PORT}`,
      '--headless=new',
      '--no-first-run',
      '--no-default-browser-check',
      '--disable-gpu',
      // The panel uses its own local CA, which a fresh profile does not trust.
      '--ignore-certificate-errors',
      'about:blank'
    ], { stdio: 'ignore' });

    try {
      let version = null;
      for (let attempt = 0; attempt < 60 && !version; attempt++) {
        await sleep(500);
        try {
          version = await (await fetch(`http://127.0.0.1:${CDP_PORT}/json/version`)).json();
        } catch {}
      }
      const browser = await connect(version.webSocketDebuggerUrl);
      const loaded = await browser.send('Extensions.loadUnpacked', { path: SOURCE });
      browser.close();
      const extensionID = loaded.id;
      console.log('extension id: ' + extensionID);

      let worker = null;
      for (let attempt = 0; attempt < 40 && !worker; attempt++) {
        await sleep(500);
        const list = await (await fetch(`http://127.0.0.1:${CDP_PORT}/json/list`)).json();
        worker = list.find(t => t.type === 'service_worker' && t.url.includes(extensionID));
      }
      if (!worker) throw new Error('the extension service worker never appeared');

      const cdp = await connect(worker.webSocketDebuggerUrl);
      await cdp.send('Runtime.enable');

      const evaluate = async expression => {
        const result = await cdp.send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
        if (result.exceptionDetails) {
          throw new Error('eval failed: ' + JSON.stringify(result.exceptionDetails.exception?.description || result.exceptionDetails));
        }
        return result.result.value;
      };

      // Let the extension discover Mirroxy on its own, exactly like a user
      // who just installed it. The connector has to be this test instance, so
      // the panel address is used for both ends.
      await evaluate(`chrome.storage.local.set(${JSON.stringify({
        url: paired.url, connector: paired.url, token: paired.token
      })})`);
      const cfg = await evaluate('settings()');
      console.log('extension settings: ' + JSON.stringify({ url: cfg.url, connector: cfg.connector, hasToken: !!cfg.token }));
      if (!cfg.token) throw new Error('the extension did not pair');

      // Put a cookie in the jar for the download site.
      await evaluate(`(async () => {
        await chrome.cookies.set({ url: 'http://127.0.0.1:${site.port}/', name: 'sid', value: 'signed-in-token', path: '/' });
      })()`);

      // Relay the download the way the extension does.
      const relayed = await evaluate(`relay('http://127.0.0.1:${site.port}/file.bin', 'file.bin', 'http://127.0.0.1:${site.port}/page')`);
      console.log('relay result: ' + JSON.stringify(relayed));

      // The download engine performs the actual fetch; give it a moment.
      let hit = null;
      for (let attempt = 0; attempt < 40 && !hit; attempt++) {
        await sleep(500);
        if (site.seen.hits > 0) hit = site.seen;
      }
      console.log('download site saw: ' + JSON.stringify({
        hits: site.seen.hits, cookie: site.seen.cookie, userAgent: site.seen.userAgent, referer: site.seen.referer
      }));

      cdp.close();
      site.server.close();

      if (!hit) {
        console.log('FAILED: the relayed download never reached the download site');
        process.exitCode = 1;
        return;
      }
      if (!/sid=signed-in-token/.test(hit.cookie || '')) {
        console.log('FAILED: the download site did not receive the browser cookie');
        process.exitCode = 1;
        return;
      }
      console.log('PASS: the relayed download carried the browser cookie to the download site');
    } finally {
      child.kill();
      await sleep(500);
      try { fs.rmSync(profile, { recursive: true, force: true }); } catch {}
    }
  } finally {
    app.kill();
    await sleep(500);
    try { fs.rmSync(root, { recursive: true, force: true }); } catch {}
  }
}

main().catch(err => {
  console.error('FAILED: ' + err.message);
  process.exit(1);
});
