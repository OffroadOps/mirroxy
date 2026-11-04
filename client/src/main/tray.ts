import { Menu, Tray, nativeImage } from 'electron';
import type { Tray as TrayType } from 'electron';

export interface TrayOptions {
  iconPath: string;
  label: string;
  show: () => void;
  openDownloads: () => void;
  quitUi: () => void;
  quitAll: () => void;
}

export function createTray(options: TrayOptions): TrayType {
  const image = nativeImage.createFromPath(options.iconPath);
  const icon = process.platform === 'win32' ? image.resize({ width: 16, height: 16 }) : image;

  const tray = new Tray(icon);
  tray.setToolTip(`Mirroxy · ${options.label}`);
  tray.setContextMenu(
    Menu.buildFromTemplate([
      { label: '打开 Mirroxy', click: options.show },
      { label: '下载管理', click: options.openDownloads },
      { type: 'separator' },
      { label: '退出界面（代理继续运行）', click: options.quitUi },
      { label: '退出 Mirroxy', click: options.quitAll },
    ]),
  );
  tray.on('click', options.show);
  tray.on('double-click', options.show);
  return tray;
}
