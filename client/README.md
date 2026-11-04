# Mirroxy Electron 客户端

Phase 0 的最小可运行外壳：原生窗口 + 托盘，启动时由主进程拉起 Go 后台，并读取 `/api/state`。
界面暂时直接加载现有 Web 面板；React 渲染层按 `docs/ELECTRON-REFACTOR-PLAN.md` 的 Phase 2 接入。

## 运行

先在仓库根目录构建后台（Electron 默认查找 `work/mirroxy-dev.exe`）：

```powershell
go build -buildvcs=false -o work/mirroxy-dev.exe .
```

再启动客户端：

```powershell
cd client
npm install
npm start
```

## 环境变量

| 变量 | 作用 |
|---|---|
| `MIRROXY_DAEMON` | 指定 Go 后台可执行文件，打包后默认查找 Electron 同级的 `Mirroxy.exe` |
| `MIRROXY_DAEMON_DIR` | 指定后台运行目录（含 `data/`、`bin/`）；开发时默认是仓库根目录 |

## 当前行为

- 单实例；重复启动会显示已有窗口。
- 关闭窗口只隐藏到托盘，代理继续运行。
- 托盘菜单：打开 Mirroxy / 退出界面（代理继续运行）/ 退出 Mirroxy。
- 面板使用自签证书，主进程只对 `127.0.0.1`、`localhost` 放行证书错误。
- 渲染进程启用 `contextIsolation`、关闭 `nodeIntegration`、`sandbox: true`；外部链接仅允许 HTTPS 白名单域名。
- 面板端口由主进程动态选择一个空闲端口，不占用固定端口。

## 尚未实现（按计划推进）

- Go `--daemon`、SSE 事件流、`ui-session.json` 令牌（Phase 1）。
- React + TypeScript 渲染层与 IPC 端点白名单（Phase 2–4）。
- 下载管理窗口、electron-builder 打包、在线更新（Phase 5–6）。
