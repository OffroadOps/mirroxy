import { contextBridge, ipcRenderer } from 'electron';

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
