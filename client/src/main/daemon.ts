import { spawn, type ChildProcess } from 'node:child_process';
import fs from 'node:fs';
import https from 'node:https';
import net from 'node:net';
import path from 'node:path';
import { app } from 'electron';

const LOCAL_HOST = '127.0.0.1';

export interface DaemonState {
  version?: string;
  nodes?: unknown[];
  token?: string;
  platform?: { os?: string };
}

export interface DaemonStatus {
  url: string;
  ready: boolean;
  version: string;
  nodes: number;
  running: number;
}

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once('error', reject);
    server.listen(0, LOCAL_HOST, () => {
      const address = server.address();
      const port = typeof address === 'object' && address ? address.port : 0;
      server.close(() => (port ? resolve(port) : reject(new Error('无法分配面板端口'))));
    });
  });
}

function request(
  url: string,
  method: 'GET' | 'POST',
  body?: unknown,
  token?: string,
): Promise<{ status: number; text: string }> {
  return new Promise((resolve, reject) => {
    const payload = body === undefined ? undefined : JSON.stringify(body);
    const req = https.request(
      url,
      {
        method,
        rejectUnauthorized: false,
        timeout: 4000,
        headers: {
          ...(payload ? { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(payload) } : {}),
          ...(token ? { 'X-App-Token': token } : {}),
        },
      },
      (res) => {
        let text = '';
        res.setEncoding('utf8');
        res.on('data', (chunk) => (text += chunk));
        res.on('end', () => resolve({ status: res.statusCode ?? 0, text }));
      },
    );
    req.on('timeout', () => req.destroy(new Error('请求后台超时')));
    req.on('error', reject);
    if (payload) req.write(payload);
    req.end();
  });
}

async function waitForDaemon(url: string, timeoutMs = 25000): Promise<DaemonState> {
  const deadline = Date.now() + timeoutMs;
  let lastError = '';
  while (Date.now() < deadline) {
    try {
      const res = await request(`${url}/api/state`, 'GET');
      if (res.status === 200) return JSON.parse(res.text) as DaemonState;
    } catch (error) {
      lastError = error instanceof Error ? error.message : String(error);
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`后台没有在预期时间内就绪${lastError ? `：${lastError}` : ''}`);
}

function daemonCandidates(): string[] {
  const exe = process.platform === 'win32' ? '.exe' : '';
  const list: string[] = [];
  if (process.env.MIRROXY_DAEMON) list.push(process.env.MIRROXY_DAEMON);
  if (app.isPackaged) {
    list.push(path.join(process.resourcesPath, '..', `Mirroxy${exe}`));
    list.push(path.join(process.resourcesPath, `Mirroxy${exe}`));
  } else {
    const root = path.resolve(app.getAppPath(), '..');
    list.push(path.join(root, 'work', `mirroxy-dev${exe}`));
    list.push(path.join(root, `mirroxy${exe}`));
    list.push(path.join(root, 'bin', `mirroxy${exe}`));
  }
  return list;
}

function resolveDaemonBinary(): string {
  const found = daemonCandidates().find((candidate) => fs.existsSync(candidate));
  if (found) return found;
  throw new Error(
    `找不到 Mirroxy 后台程序。请先构建：\n` +
      `  go build -o work/mirroxy-dev.exe .\n` +
      `或用 MIRROXY_DAEMON 指定后台可执行文件路径。\n` +
      `已尝试：\n  ${daemonCandidates().join('\n  ')}`,
  );
}

function resolveDataDir(): string {
  if (process.env.MIRROXY_DAEMON_DIR) return path.resolve(process.env.MIRROXY_DAEMON_DIR);
  if (!app.isPackaged) return path.resolve(app.getAppPath(), '..');
  return app.getPath('userData');
}

export class Daemon {
  private child: ChildProcess | null = null;
  private state: DaemonState | null = null;
  url = '';
  private spawnError = '';

  async start(): Promise<DaemonStatus> {
    if (this.url && this.state) return this.status();
    const binary = resolveDaemonBinary();
    const dataDir = resolveDataDir();
    fs.mkdirSync(dataDir, { recursive: true });
    const port = await freePort();
    this.url = `https://${LOCAL_HOST}:${port}`;
    this.spawnError = '';

    const child = spawn(binary, ['--dir', dataDir, '--panel', String(port), '--no-open'], {
      detached: true,
      stdio: 'ignore',
      windowsHide: true,
    });
    child.on('error', (error) => {
      this.spawnError = error.message;
    });
    child.on('exit', (code) => {
      if (!this.state && code !== 0 && !this.spawnError) this.spawnError = `后台进程已退出（code ${code}）`;
    });
    child.unref();
    this.child = child;

    try {
      this.state = await waitForDaemon(this.url);
    } catch (error) {
      const detail = this.spawnError ? `${String(error)}（${this.spawnError}）` : String(error);
      throw new Error(detail);
    }
    return this.status();
  }

  async status(): Promise<DaemonStatus> {
    if (this.url) {
      try {
        const res = await request(`${this.url}/api/state`, 'GET');
        if (res.status === 200) this.state = JSON.parse(res.text) as DaemonState;
      } catch {
        // Keep the last known state while the daemon restarts.
      }
    }
    const nodes = Array.isArray(this.state?.nodes) ? this.state.nodes : [];
    const running = nodes.filter((node) => (node as { status?: string })?.status === 'running').length;
    return {
      url: this.url,
      ready: !!this.state,
      version: this.state?.version ?? '',
      nodes: nodes.length,
      running,
    };
  }

  async stop(): Promise<void> {
    if (!this.url) return;
    try {
      const res = await request(`${this.url}/api/state`, 'GET');
      const token = (JSON.parse(res.text) as DaemonState).token ?? '';
      await request(`${this.url}/api/shutdown`, 'POST', {}, token);
    } catch {
      // The daemon may already be gone.
    }
    this.state = null;
    this.child = null;
  }
}
