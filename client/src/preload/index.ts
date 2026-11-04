import { contextBridge, ipcRenderer } from 'electron';

function currentTheme(): 'light' | 'dark' {
  return document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light';
}

function syncTitleBarTheme(): void {
  ipcRenderer.send('mx:window:theme', currentTheme());
}

window.addEventListener('DOMContentLoaded', () => {
  syncTitleBarTheme();
  new MutationObserver(syncTitleBarTheme).observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['data-theme'],
  });
});

contextBridge.exposeInMainWorld('mirroxy', {
  platform: process.platform,
  versions: {
    electron: process.versions.electron,
    chrome: process.versions.chrome,
    node: process.versions.node,
  },
  host: {
    status: () => ipcRenderer.invoke('mx:host:status'),
    openExternal: (url: string) => ipcRenderer.invoke('mx:host:openExternal', url),
  },
});
