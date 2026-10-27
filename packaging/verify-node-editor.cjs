'use strict';
// Checks the node editor in a real browser: for a SOCKS or HTTP node the
// password and username boxes must be visible, and for a VLESS node the
// username box must be hidden.
//
// The panel serves HTTPS with its own local CA, so the browser runs with
// --ignore-certificate-errors inside a throwaway profile.
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const CHROME = process.env.CHROME_BIN || 'C:/Program Files/Google/Chrome/Application/chrome.exe';
const PANEL = process.argv[2] || 'https://127.0.0.1:19980/';
const CDP_PORT = Number(process.env.CDP_PORT || 19280);

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

async function main() {
  const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'mirroxy-panel-'));
  const child = spawn(CHROME, [
    `--user-data-dir=${profile}`,
    `--remote-debugging-port=${CDP_PORT}`,
    '--headless=new',
    '--no-first-run',
    '--no-default-browser-check',
    '--disable-gpu',
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
    if (!version) throw new Error('the browser never opened its debugging port');

    const opened = await (await fetch(`http://127.0.0.1:${CDP_PORT}/json/new?${encodeURIComponent(PANEL)}`, { method: 'PUT' })).json();
    const cdp = await connect(opened.webSocketDebuggerUrl);
    await cdp.send('Runtime.enable');
    await sleep(2500);

    const evaluate = async expression => {
      const result = await cdp.send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
      if (result.exceptionDetails) {
        throw new Error('eval failed: ' + JSON.stringify(result.exceptionDetails.exception?.description || result.exceptionDetails));
      }
      return result.result.value;
    };

    // The panel needs the node list before the editor can be opened.
    await evaluate('(async () => { for (let i = 0; i < 40 && !(window.state && state.nodes.length); i++) await new Promise(r => setTimeout(r, 250)); return state.nodes.map(n => ({ id: n.id, name: n.name, protocol: n.protocol })); })()')
      .then(nodes => console.log('nodes: ' + JSON.stringify(nodes)));

    const inspect = async (protocol, label) => {
      const outcome = await evaluate(`(async () => {
        const node = state.nodes.find(n => n.protocol === '${protocol}');
        if (!node) return { error: 'no ${protocol} node' };
        await openEditNode(node.id);
        const dialog = document.getElementById('editNodeDialog');
        const passwordField = document.getElementById('editUserField');
        const usernameField = document.getElementById('editUsernameField');
        const result = {
          protocol: '${protocol}',
          dialogOpen: dialog.open,
          passwordVisible: !passwordField.classList.contains('hidden'),
          usernameVisible: !usernameField.classList.contains('hidden'),
          passwordLabel: document.getElementById('editPasswordLabel').textContent,
          usernameLabel: document.getElementById('editUsernameLabel').textContent,
          passwordValue: document.getElementById('editPassword').value,
          usernameValue: document.getElementById('editUsername').value
        };
        dialog.close();
        return result;
      })()`);
      console.log(label + ': ' + JSON.stringify(outcome));
      return outcome;
    };

    const socks = await inspect('socks', 'socks node');
    const http = await inspect('http', 'http node');
    const vless = await inspect('vless', 'vless node');

    cdp.close();

    const problems = [];
    for (const [label, outcome] of [['socks', socks], ['http', http]]) {
      if (!outcome.passwordVisible) problems.push(label + ': the password box is hidden');
      if (!outcome.usernameVisible) problems.push(label + ': the username box is hidden');
      if (!outcome.passwordValue) problems.push(label + ': the password box is empty');
      if (!outcome.usernameValue) problems.push(label + ': the username box is empty');
    }
    if (vless.passwordVisible === false) problems.push('vless: the password box should stay visible');
    if (vless.usernameVisible !== false) problems.push('vless: the username box should be hidden');

    if (problems.length) {
      console.log('FAILED: ' + problems.join('; '));
      process.exitCode = 1;
      return;
    }
    console.log('PASS: the editor shows the credentials for SOCKS and HTTP nodes');
  } finally {
    child.kill();
    await sleep(500);
    try { fs.rmSync(profile, { recursive: true, force: true }); } catch {}
  }
}

main().catch(err => {
  console.error('FAILED: ' + err.message);
  process.exit(1);
});
