import { app, BrowserWindow, ipcMain, Menu, session } from 'electron';
import path from 'node:path';
import { Daemon, type DaemonStatus } from './daemon';
import { isLocalUrl, openExternal } from './security';
import { createTray } from './tray';

const daemon = new Daemon();
let mainWindow: BrowserWindow | null = null;
let quitting = false;
let panelOrigin = '';

const gotLock = app.requestSingleInstanceLock();
if (!gotLock) {
  app.quit();
} else {
  app.on('second-instance', () => showMainWindow());
  app.whenReady().then(bootstrap).catch((error) => showErrorPage(String(error)));
}

function windowPreferences(): Electron.WebPreferences {
  return {
    preload: path.join(__dirname, '../preload/index.js'),
    contextIsolation: true,
    nodeIntegration: false,
    sandbox: true,
    spellcheck: false,
    devTools: !app.isPackaged,
  };
}

function createWindow(url: string): BrowserWindow {
  const window = new BrowserWindow({
    width: 1200,
    height: 760,
    minWidth: 900,
    minHeight: 560,
    show: false,
    center: true,
    title: 'Mirroxy',
    backgroundColor: '#f6f7f8',
    autoHideMenuBar: true,
    webPreferences: windowPreferences(),
  });

  window.once('ready-to-show', () => window.show());
  window.on('close', (event) => {
    if (quitting) return;
    event.preventDefault();
    window.hide();
  });
  window.on('closed', () => {
    if (mainWindow === window) mainWindow = null;
  });

  window.webContents.setWindowOpenHandler(({ url: target }) => {
    if (isLocalUrl(target)) {
      createWindow(target);
      return { action: 'deny' };
    }
    void openExternal(target);
    return { action: 'deny' };
  });

  window.webContents.on('will-navigate', (event, target) => {
    if (!target.startsWith(panelOrigin)) {
      event.preventDefault();
      void openExternal(target);
    }
  });

  void window.loadURL(url);
  return window;
}

function showMainWindow(): void {
  if (!mainWindow) return;
  if (mainWindow.isMinimized()) mainWindow.restore();
  mainWindow.show();
  mainWindow.focus();
}

function showErrorPage(message: string): void {
  const escape = (value: string) =>
    value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  const html = `<!doctype html><html lang="zh-CN"><meta charset="utf-8">
  <body style="margin:0;height:100vh;display:grid;place-items:center;background:#f6f7f8;color:#171a1f;font:13px/1.6 'Segoe UI','Microsoft YaHei',sans-serif">
  <div style="max-width:640px;padding:28px 32px;background:#fff;border:1px solid #e6e8eb;border-radius:12px">
    <h1 style="margin:0 0 10px;font-size:18px">无法启动 Mirroxy 后台</h1>
    <p style="margin:0 0 14px;color:#5f6b77">Electron 壳已经启动，但没能拉起 Go 后台。</p>
    <pre style="white-space:pre-wrap;background:#fafbfb;border:1px solid #eceef1;border-radius:8px;padding:12px;font:12px/1.7 Consolas,monospace;color:#41474f">${escape(message)}</pre>
  </div></body></html>`;
  mainWindow = createWindow(`data:text/html;charset=utf-8,${encodeURIComponent(html)}`);
}

async function bootstrap(): Promise<void> {
  app.setAppUserModelId('ai.mirroxy.desktop');
  app.commandLine.appendSwitch('no-proxy-server');

  app.on('certificate-error', (event, _contents, url, _error, _certificate, callback) => {
    if (isLocalUrl(url)) {
      event.preventDefault();
      callback(true);
      return;
    }
    callback(false);
  });

  session.defaultSession.setPermissionRequestHandler((_contents, _permission, callback) => callback(false));

  if (process.platform === 'darwin') {
    Menu.setApplicationMenu(
      Menu.buildFromTemplate([{ role: 'appMenu' }, { role: 'editMenu' }, { role: 'viewMenu' }]),
    );
  } else {
    Menu.setApplicationMenu(null);
  }

  ipcMain.handle('mx:host:status', async (): Promise<DaemonStatus> => daemon.status());
  ipcMain.handle('mx:host:openExternal', (_event, url: string) => openExternal(String(url)));

  app.on('before-quit', () => {
    quitting = true;
  });
  app.on('window-all-closed', () => {
    // The tray keeps the UI alive; use the tray menu to exit.
  });

  const status = await daemon.start();
  panelOrigin = new URL(status.url).origin;
  mainWindow = createWindow(status.url);

  const iconPath = path.join(app.getAppPath(), 'resources', 'tray.png');
  createTray({
    iconPath,
    label: `${status.version || 'unknown'} · ${status.nodes} 个节点`,
    show: showMainWindow,
    quitUi: () => {
      quitting = true;
      app.quit();
    },
    quitAll: async () => {
      quitting = true;
      await daemon.stop();
      app.quit();
    },
  });
}
