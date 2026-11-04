import { app, BrowserWindow, ipcMain, Menu, session } from 'electron';
import path from 'node:path';
import { Daemon, type DaemonStatus } from './daemon';
import { isLocalUrl, openExternal } from './security';
import { createTray } from './tray';

const daemon = new Daemon();
let mainWindow: BrowserWindow | null = null;
let downloadsWindow: BrowserWindow | null = null;
let quitting = false;
let panelOrigin = '';
const overlayHeights = new WeakMap<BrowserWindow, number>();

// Must be set before the app is ready, otherwise Chromium uses the system
// proxy and the daemon sees the panel request as forwarded (and rejects it).
app.commandLine.appendSwitch('no-proxy-server');

const MAIN_CHROME_CSS = `
.topbar{padding-right:150px;-webkit-app-region:drag;user-select:none}
.topbar :is(a,button,input){-webkit-app-region:no-drag}
.sidebar{-webkit-app-region:drag}
.sidebar :is(a,button,select,input,label){-webkit-app-region:no-drag}
`;

const DOWNLOADS_CHROME_CSS = `
.dm-menubar{height:38px;padding-right:150px;-webkit-app-region:drag;user-select:none}
.dm-menubar :is(button,input,label){-webkit-app-region:no-drag}
`;

interface WindowOptions {
  width: number;
  height: number;
  minWidth: number;
  minHeight: number;
  title: string;
  hideOnClose: boolean;
  css: string;
  overlayHeight: number;
}

function overlay(theme: 'light' | 'dark', height: number): Electron.TitleBarOverlay {
  return theme === 'dark'
    ? { color: '#15181d', symbolColor: '#e7e9ec', height }
    : { color: '#ffffff', symbolColor: '#41474f', height };
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

function createWindow(url: string, options: WindowOptions): BrowserWindow {
  const window = new BrowserWindow({
    width: options.width,
    height: options.height,
    minWidth: options.minWidth,
    minHeight: options.minHeight,
    show: false,
    center: true,
    title: options.title,
    backgroundColor: '#f6f7f8',
    autoHideMenuBar: true,
    ...(process.platform === 'win32'
      ? { titleBarStyle: 'hidden' as const, titleBarOverlay: overlay('light', options.overlayHeight) }
      : process.platform === 'darwin'
        ? { titleBarStyle: 'hiddenInset' as const }
        : {}),
    webPreferences: windowPreferences(),
  });

  overlayHeights.set(window, options.overlayHeight);

  window.once('ready-to-show', () => {
    window.show();
    window.focus();
  });
  window.on('close', (event) => {
    if (options.hideOnClose && !quitting) {
      event.preventDefault();
      window.hide();
    }
  });
  window.on('closed', () => {
    if (mainWindow === window) mainWindow = null;
    if (downloadsWindow === window) downloadsWindow = null;
  });

  window.webContents.on('did-finish-load', () => {
    void window.webContents.insertCSS(options.css);
  });

  window.webContents.setWindowOpenHandler(({ url: target }) => {
    if (isLocalUrl(target)) {
      if (new URL(target).pathname === '/downloads') openDownloadsWindow();
      else openPanelWindow(target);
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

function openPanelWindow(url: string): void {
  createWindow(url, {
    width: 1200,
    height: 760,
    minWidth: 900,
    minHeight: 560,
    title: 'Mirroxy',
    hideOnClose: true,
    css: MAIN_CHROME_CSS,
    overlayHeight: 56,
  });
}

function openDownloadsWindow(): void {
  if (!panelOrigin) return;
  if (downloadsWindow && !downloadsWindow.isDestroyed()) {
    downloadsWindow.show();
    downloadsWindow.focus();
    return;
  }
  downloadsWindow = createWindow(`${panelOrigin}/downloads`, {
    width: 1040,
    height: 660,
    minWidth: 780,
    minHeight: 520,
    title: '下载管理',
    hideOnClose: false,
    css: DOWNLOADS_CHROME_CSS,
    overlayHeight: 38,
  });
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
  mainWindow = createWindow(`data:text/html;charset=utf-8,${encodeURIComponent(html)}`, {
    width: 760,
    height: 420,
    minWidth: 560,
    minHeight: 320,
    title: 'Mirroxy',
    hideOnClose: true,
    css: '',
    overlayHeight: 0,
  });
}

async function bootstrap(): Promise<void> {
  app.setAppUserModelId('ai.mirroxy.desktop');

  await session.defaultSession.setProxy({ mode: 'direct' });
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
  ipcMain.on('mx:window:theme', (event, theme: string) => {
    if (process.platform !== 'win32') return;
    const window = BrowserWindow.fromWebContents(event.sender);
    if (!window) return;
    const height = overlayHeights.get(window);
    if (!height) return;
    window.setTitleBarOverlay(overlay(theme === 'dark' ? 'dark' : 'light', height));
  });

  app.on('before-quit', () => {
    quitting = true;
  });
  app.on('window-all-closed', () => {
    // The tray keeps the UI alive; use the tray menu to exit.
  });

  const status = await daemon.start();
  panelOrigin = new URL(status.url).origin;
  mainWindow = createWindow(status.url, {
    width: 1200,
    height: 760,
    minWidth: 900,
    minHeight: 560,
    title: 'Mirroxy',
    hideOnClose: true,
    css: MAIN_CHROME_CSS,
    overlayHeight: 56,
  });

  const iconPath = path.join(app.getAppPath(), 'resources', 'tray.png');
  createTray({
    iconPath,
    label: `${status.version || 'unknown'} · ${status.nodes} 个节点`,
    show: showMainWindow,
    openDownloads: openDownloadsWindow,
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

const gotLock = app.requestSingleInstanceLock();
if (!gotLock) {
  app.quit();
} else {
  app.on('second-instance', () => showMainWindow());
  app.whenReady().then(bootstrap).catch((error) => showErrorPage(String(error)));
}
