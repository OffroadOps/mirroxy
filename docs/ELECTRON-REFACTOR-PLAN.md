# Mirroxy Electron 桌面客户端重构设计文档

> 状态：设计稿 v1（2026-11-04）
> 基线：源码 0.9.0（`3286c3c`），Go 守护进程 + 内嵌 WebUI（`web/`）+ Windows Wails 下载窗口（`desktop/`）+ Kotlin Android（`android/`）
> 关系：本文取代 [`FLUTTER-MIGRATION-PLAN.md`](FLUTTER-MIGRATION-PLAN.md) 中的桌面端路线；Android 不在 Electron 范围内，见 §15。

---

## 目录

1. [背景与目标](#1-背景与目标)
2. [硬性约束（来自 AGENTS.md）](#2-硬性约束来自-agentsmd)
3. [为什么选 Electron](#3-为什么选-electron)
4. [总体架构](#4-总体架构)
5. [进程生命周期](#5-进程生命周期)
6. [通信设计：渲染进程 ↔ 主进程 ↔ 守护进程](#6-通信设计渲染进程--主进程--守护进程)
7. [Go 守护进程改造](#7-go-守护进程改造)
8. [技术选型](#8-技术选型)
9. [工程目录结构](#9-工程目录结构)
10. [Electron 主进程设计](#10-electron-主进程设计)
11. [前端（渲染进程）设计](#11-前端渲染进程设计)
12. [功能迁移清单](#12-功能迁移清单)
13. [安全设计](#13-安全设计)
14. [打包、安装与在线更新](#14-打包安装与在线更新)
15. [Android 与无界面服务器](#15-android-与无界面服务器)
16. [测试策略](#16-测试策略)
17. [CI/CD](#17-cicd)
18. [性能预算](#18-性能预算)
19. [分阶段实施计划](#19-分阶段实施计划)
20. [风险与对策](#20-风险与对策)
21. [待决策事项](#21-待决策事项)
22. [附录](#22-附录)

---

## 1. 背景与目标

### 1.1 现状

| 组成 | 位置 | 说明 |
|---|---|---|
| Go 主程序 | 仓库根 `*.go`（约 2 万行，含测试） | 节点、订阅、核心进程、系统代理、路由、流量、下载（aria2）、在线更新、远程管理、AI API；同时提供 `127.0.0.1:15060` 的 Web 面板 |
| WebUI | `web/`（原生 JS，`app.js` 480 行压缩风格 + 若干模块） | 通过 `go:embed` 内嵌，系统浏览器打开；5 s 轮询 `/api/state`，1 s 轮询 `/api/traffic` |
| Windows 托盘 | `tray_windows.go`（Win32 API 直接调用） | 系统代理节点子菜单、路由模式、打开面板、下载管理、UAC 运行、停止/运行代理、流量、更新、退出 |
| 下载窗口 | `desktop/`（Wails v2 + WebView2，独立 Go 模块） | 仅 Windows；通过受限桥接调用 `/api/downloads*` |
| 浏览器扩展 | `browser-extension/` | 调用 `/api/downloads/connector/*` 接力下载 |
| 更新助手 | `cmd/upgrade/`、`self_update*.go` | `Mirroxy-update-<os>-<arch>.zip`，白名单仅含 `Mirroxy[.exe]`、`upgrade[.exe]`、`manifest.json`、`bin/resources/Mirroxy_set_*.bin`、`licenses/*` |
| 安装 | `packaging/Mirroxy.iss`（Inno Setup）、`install.sh`、`install-macos.sh` | |

### 1.2 现有 WebUI 的结构性问题

1. **依赖浏览器**：面板是一个浏览器标签页，容易被关掉、被浏览器扩展干扰；首次需要信任本地证书（`panel_cert*.go`）。
2. **端口与 Host 校验负担**：面板端口冲突（`installer_port*.go`）、Host/Origin/Fetch-Metadata 校验、`X-App-Token` 都是为"浏览器访问本机端口"这一模型付出的成本。
3. **原生能力缺失**：文件夹选择要靠后端目录浏览接口模拟；拖放、系统通知、全局快捷键、多窗口、开机自启都做不好。
4. **托盘只有 Windows**：macOS / Linux 桌面没有托盘，日常"一步切换"路径无法跨平台。
5. **两套界面技术**：WebUI（浏览器） + Wails（WebView2）重复实现窗口与桥接。
6. **轮询**：状态 5 s、流量 1 s 轮询，后台标签页节流后数据滞后。
7. **前端代码可维护性**：`app.js` 单文件、无类型、无组件化，测试只覆盖少量纯函数（`packaging/test-*.cjs`）。

### 1.3 目标

- **G1** 用一个 Electron 桌面客户端统一 Windows x86_64 / macOS ARM64 / Linux AMD64 / Linux ARM64 的窗口、托盘、下载管理窗口。
- **G2** Go 守护进程继续作为唯一的状态拥有者与代理引擎，**不重写**代理、订阅、路由、流量、下载、更新逻辑。
- **G3** 前端重写为 TypeScript + React 组件化代码，**同一套前端**既运行在 Electron 渲染进程，也编译成远程 Web 面板供无界面 Linux 服务器使用。
- **G4** 状态推送化：新增事件流，取消前端轮询。
- **G5** 托盘三平台化，"双击节点 / 托盘点选即启动并接管系统代理"在三平台一致。
- **G6** 保持在线更新、数据保留、单节点独立核心等全部现有承诺。

### 1.4 非目标

- 不把 Go 编译成 Node 原生模块或 WASM 嵌入 Electron（理由见 §4.3）。
- 不在 Electron 中实现任何代理、系统代理、TUN、进程扫描逻辑——这些仍在 Go。
- 不引入任何 Node 原生模块（`.node`），避免四平台交叉编译与 ABI 问题。
- 不做 Android 版 Electron（Electron 不支持移动端）。
- 迁移期间不给旧 WebUI 加新功能，只修严重缺陷。

---

## 2. 硬性约束（来自 AGENTS.md）

以下条目是验收红线，每个阶段的出口检查都要逐条确认：

| # | 约束 | 在本设计中的落点 |
|---|---|---|
| C1 | 每个节点独立核心进程；起停一个节点不重启、不中断其它节点 | Go 不变；E2E 用例 §16.4-E3 |
| C2 | 一步切换：双击节点行 / 托盘点选即按需启动并接管系统代理；顶部开关、路由模式即时生效，无"应用"按钮 | §11.5、§10.4；E2E 用例 E1/E2 |
| C3 | 默认"不改变系统代理"；与 v2rayN（`127.0.0.1:10808`）共存 | Go 不变；Electron 自身网络请求强制直连（§10.7） |
| C4 | 自定义节点名称在订阅刷新后保留，改名不重启核心 | Go 不变；E2E 用例 E4 |
| C5 | 每次发布生成四平台 `Mirroxy-update-*.zip`，保留核心与用户数据 | §14.4 |
| C6 | 程序更新会短暂重启，不宣称零中断；Linux 原 PID exec，Windows 独立助手 | §14.4、§5.5 |
| C7 | 发布物不含 data、节点、订阅、密钥、日志、凭据 | §14.6 白名单 + `verify-release.py` 扩展 |
| C8 | 没有实机验证的平台不得宣称已通过 | §16.6 验证矩阵 |
| C9 | 每次新增功能与修复记录到 `web/changelog.json` | §19 每阶段出口；迁移后路径见 §22.3 |
| C10 | 源码必须提交 Git，不留未提交状态 | §17.3 |
| C11 | 二进制发布到 GitHub Releases，同版本不得替换字节 | §14.5 |
| C12 | 唯一源码目录 `D:\Mirroxy`，不依赖 `E:\scrcpyproxy` | §9 |
| C13 | 功能验证使用隔离测试目录，不改真实节点、端口与名称 | §16.2 |

---

## 3. 为什么选 Electron

### 3.1 与其它方案对比

| 维度 | 现状 WebUI | Wails（扩展现有 `desktop/`） | Flutter（前一版计划） | **Electron** |
|---|---|---|---|---|
| 前端代码复用 | — | 可复用 Web 技术 | 全部重写为 Dart | **可复用 Web 技术、i18n、CSS 主题；与浏览器扩展同语言** |
| 远程 Web 面板 | 有 | 需另行维护 | 需另行维护或放弃 | **同一套 React 代码编译为 Web 版** |
| 渲染一致性 | 依赖用户浏览器 | Win: WebView2 / mac: WKWebView / Linux: WebKitGTK，三套引擎 | 自绘，一致 | **自带 Chromium，一致** |
| 托盘 / 多窗口 | 无 / 无 | 托盘需 v3（仍在演进） | 依赖第三方插件 | **官方 API，成熟** |
| Linux ARM64 | — | 需 WebKitGTK 依赖 | 需 arm64 runner 构建 | **官方预编译 arm64，无原生模块时可在 x64 打包** |
| 包体 | 最小 | 小（~15 MB） | 中（+25–40 MB） | **大（安装包 ~90–110 MB，解压 ~250–300 MB）** |
| 常驻内存 | 浏览器标签 | 低 | 中 | **高（窗口打开 150–250 MB）**，靠"隐藏即销毁窗口"缓解（§18） |
| 团队成本 | — | Go + Web | 需学 Dart / Flutter | **Web 技术栈，AI 辅助成熟** |

### 3.2 结论

选 Electron 的核心理由：**一套 TypeScript 前端同时覆盖桌面客户端、远程 Web 面板、下载窗口**，且托盘 / 多窗口 / 原生对话框 / 通知等能力官方齐备。代价是包体与内存，通过"无原生模块""窗口隐藏后销毁渲染进程""守护进程独立运行"控制在可接受范围。

---

## 4. 总体架构

### 4.1 进程模型

```
┌─────────────────────────────── Electron 应用 (Mirroxy UI) ───────────────────────────────┐
│                                                                                          │
│  ┌──────────────────────┐  contextBridge  ┌───────────────────────────────────────────┐  │
│  │ 渲染进程：主窗口        │ ◀────────────▶ │ 主进程 (Node.js)                            │  │
│  │ React + TS (sandbox)  │   ipcRenderer   │  · 窗口管理 / 托盘 / 菜单 / 快捷键 / 通知     │  │
│  └──────────────────────┘                  │  · DaemonClient：HTTP + SSE（单连接复用）     │  │
│  ┌──────────────────────┐                  │  · 守护进程拉起 / 健康检查 / 重连              │  │
│  │ 渲染进程：下载窗口      │ ◀────────────▶ │  · 原生能力：文件对话框、剪贴板、拖放、自启    │  │
│  └──────────────────────┘                  │  · 更新协调（配合 Go 更新助手）                │  │
│                                             └───────────────────┬───────────────────────┘  │
└─────────────────────────────────────────────────────────────────┼──────────────────────────┘
                                                                  │ 127.0.0.1:15060（P1）
                                                                  │ 命名管道 / UDS（P2，§6.6）
                                                                  │ Bearer 令牌（令牌文件，§7.3）
                                                                  ▼
┌──────────────────────────── Go 守护进程 Mirroxy[.exe] ─────────────────────────────────┐
│  节点 · 订阅 · 系统代理 · 路由 · 流量 · 健康 · 下载(aria2) · 应用分流(TUN 助手) · 更新    │
│  /api/*（现有） /api/v1/*（AI API，现有） /api/v1/events（新增 SSE）                    │
│  远程面板（可选 HTTPS + 账号密码）：服务 React Web 构建产物                              │
└────────┬─────────────────────────┬───────────────────────────┬────────────────────────┘
         │                         │                           │
         ▼                         ▼                           ▼
  每节点独立核心进程           aria2c 下载进程             app-router 助手（UAC/TUN）
  (xray / sing-box)

  浏览器扩展 ──── /api/downloads/connector/*（不变）────▶ Go 守护进程
  远程浏览器 ──── HTTPS 远程面板（同一套 React 代码的 Web 构建）────▶ Go 守护进程
```

### 4.2 职责划分

| 层 | 拥有 | 不拥有 |
|---|---|---|
| Go 守护进程 | 所有业务状态与持久化（`data/Mirroxy.db`）、核心进程、系统代理、更新 | 任何窗口、托盘（P3 起，见 §7.5） |
| Electron 主进程 | 窗口、托盘、原生能力、与守护进程的唯一连接、UI 偏好（窗口尺寸、列宽） | 业务数据；不直接起停核心；不写 `data/` 下业务文件 |
| 渲染进程 | 展示与交互 | Node 能力、文件系统、网络（全部经 preload 暴露的受限 API） |

### 4.3 为什么不把 Go 嵌进 Electron

1. **代理必须在界面关闭后继续运行**；Linux 服务器无图形界面也要运行守护进程。
2. **在线更新**依赖 Linux 原 PID exec 与 Windows 独立助手；Go 嵌入 UI 进程会迫使 UI 与引擎同时重启。
3. 嵌入需要 cgo + Node-API 原生模块，四平台交叉编译和 Electron ABI 升级成本高，违背"无原生模块"原则。
4. 浏览器扩展、AI API、远程面板都已基于守护进程 HTTP 接口，拆开最省改动。

---

## 5. 进程生命周期

### 5.1 启动顺序

```
用户双击 Mirroxy 图标（快捷方式指向 Electron 可执行文件）
  │
  ├─ app.requestSingleInstanceLock()  失败 → 通知已有实例显示主窗口，退出
  │
  ├─ DaemonSupervisor.ensure()
  │    ├─ 读取令牌文件 <data>/ui-session.json（§7.3）
  │    ├─ GET /api/v1/ping（带令牌，超时 800 ms）
  │    │    ├─ 成功且版本兼容 → 复用现有守护进程
  │    │    ├─ 成功但 apiVersion 不兼容 → 提示"后台版本不匹配"，提供"重启后台"
  │    │    └─ 失败 → 启动 <安装目录>/Mirroxy[.exe] --daemon --no-open
  │    │              轮询 ping（最多 15 s，指数退避 100 ms→1 s），
  │    │              期间显示启动画面；超时显示诊断页（端口占用、日志路径）
  │    └─ 订阅 /api/v1/events
  │
  ├─ 创建托盘
  └─ 若非 --hidden（开机自启时传入），创建主窗口
```

- 开机自启只登记 **Electron 可执行文件 + `--hidden`**；由 UI 负责拉起守护进程。Linux 服务器场景继续使用 `mirroxy.service`（只运行守护进程，不涉及 Electron）。
- 若用户只想要后台、不要界面（例如 Windows 精简模式），保留"直接运行 `Mirroxy.exe`"的路径：P3 前 Go 托盘仍可用；P3 后 `Mirroxy.exe` 无 `--daemon` 参数启动时会尝试拉起 UI（§7.5）。

### 5.2 关闭窗口

- 关闭主窗口 = 隐藏到托盘。默认**销毁渲染进程**（`win.destroy()`），再次打开时重建（目标 < 400 ms，§18），以节省 100+ MB 内存。设置项"关闭窗口时保留界面（更快重开）"可改为 `hide()`。
- 首次关闭时弹一次性提示："Mirroxy 仍在托盘中运行，代理不受影响"。

### 5.3 退出

托盘菜单提供两个明确的退出项：

| 菜单 | 行为 |
|---|---|
| 退出界面 | 只退出 Electron；守护进程与代理继续运行（下次打开界面直接复用） |
| 退出 Mirroxy | 调用 `POST /api/shutdown`（守护进程现有逻辑：保存状态、恢复系统代理、停止核心），等待守护进程退出后退出 Electron |

macOS 的 `Cmd+Q` 映射为"退出界面"，并在首次使用时说明。

### 5.4 守护进程崩溃 / 断线

- 主进程 SSE 断开后进入重连：1 s、2 s、4 s…上限 10 s；渲染进程顶部显示"后台连接中断，正在重连"横幅，所有写操作按钮禁用。
- 连续 3 次 ping 失败且守护进程是由本 UI 拉起的 → 提示"后台已退出"，提供"重新启动后台"与"查看日志"。不自动无限重启，避免崩溃循环。
- 重连成功后主进程拉取一次完整 `/api/state` 快照，再继续增量事件（§6.4）。

### 5.5 程序更新期间

见 §14.4。要点：守护进程广播 `update.restarting` 事件 → UI 显示"程序更新中，代理会短暂重启"→ Windows 上 UI 必须主动退出以释放文件锁 → 更新助手完成替换后重新拉起 UI（若更新前 UI 在运行）。

---

## 6. 通信设计：渲染进程 ↔ 主进程 ↔ 守护进程

### 6.1 分层

```
React 组件
   │  useQuery / useMutation（TanStack Query）
   ▼
@mirroxy/api  ── Transport 接口 ──┬── ElectronTransport：window.mirroxy.invoke(...)  → preload → ipcMain → DaemonClient → HTTP
                                  └── HttpTransport：fetch('/api/...')（远程 Web 面板，浏览器内）
```

前端业务代码只依赖 `Transport` 接口，不感知运行在 Electron 还是浏览器。

```ts
// packages/api/src/transport.ts
export interface Transport {
  request<K extends Endpoint>(endpoint: K, body: RequestOf<K>): Promise<ResponseOf<K>>;
  subscribe(handler: (event: DaemonEvent) => void): () => void;   // 返回取消函数
  readonly kind: 'electron' | 'web';
  readonly capabilities: HostCapabilities;                         // 原生对话框、托盘等是否可用
}
```

### 6.2 端点注册表（单一事实来源）

所有可调用端点集中在 `packages/api/src/endpoints.ts`，用 zod 声明请求 / 响应结构。它同时用于：

1. 渲染进程类型推导；
2. 主进程 IPC 白名单与入参校验（未登记的端点一律拒绝）；
3. 生成 `api/openapi.yaml` 的对照测试（Go 端契约测试读取同一份 JSON Schema 导出，§16.3）。

```ts
export const endpoints = {
  'state':               { method: 'GET',  path: '/api/state',              req: z.void(),               res: StateSchema },
  'traffic':             { method: 'GET',  path: '/api/traffic',            req: z.void(),               res: TrafficSchema },
  'proxy.set':           { method: 'POST', path: '/api/proxy',              req: z.object({ action: z.literal('set'), id: NodeId }), res: OkSchema },
  'proxy.clear':         { method: 'POST', path: '/api/proxy',              req: z.object({ action: z.literal('clear'), id: NodeId.optional() }), res: OkSchema },
  'proxy.restore':       { method: 'POST', path: '/api/proxy',              req: z.object({ action: z.literal('restore') }), res: OkSchema },
  'node.action':         { method: 'POST', path: '/api/node',               req: NodeActionSchema,       res: OkSchema },
  'node.rename':         { method: 'POST', path: '/api/node/rename',        req: z.object({ id: NodeId, name: z.string().min(1).max(128) }), res: OkSchema },
  // …完整列表见 §22.1
} as const;
```

### 6.3 IPC 通道

只开放 4 个通道，全部在 preload 中封装，渲染进程拿不到 `ipcRenderer` 本身：

| 通道 | 方向 | 用途 |
|---|---|---|
| `mx:invoke` | 渲染 → 主（`invoke`） | `{ endpoint, body }`，主进程查表、校验、转发到守护进程 |
| `mx:event` | 主 → 渲染（`send`） | 守护进程事件扇出 |
| `mx:host` | 渲染 → 主（`invoke`） | 原生能力：`pickFolder`、`pickFile`、`openPath`、`showItemInFolder`、`copyText`、`readClipboard`、`notify`、`setAutostart`、`openWindow('downloads')`、`getHostInfo` |
| `mx:prefs` | 渲染 → 主（`invoke`） | UI 偏好读写（窗口、列宽、分组排序记忆），存 Electron `userData` |

```ts
// apps/desktop/src/preload/index.ts
contextBridge.exposeInMainWorld('mirroxy', {
  invoke: (endpoint: string, body?: unknown) => ipcRenderer.invoke('mx:invoke', { endpoint, body }),
  host:   (op: HostOp, arg?: unknown)        => ipcRenderer.invoke('mx:host', { op, arg }),
  prefs:  { get: (k: string) => ipcRenderer.invoke('mx:prefs', { op: 'get', k }),
            set: (k: string, v: unknown) => ipcRenderer.invoke('mx:prefs', { op: 'set', k, v }) },
  onEvent: (cb: (e: unknown) => void) => {
    const listener = (_: unknown, e: unknown) => cb(e);
    ipcRenderer.on('mx:event', listener);
    return () => ipcRenderer.off('mx:event', listener);
  },
});
```

主进程侧每个 handler 必须：校验 `event.senderFrame.url` 属于本应用（`app://mirroxy/`）、按注册表校验 body、限制 body 大小（16 KiB，下载添加 64 KiB）、统一把守护进程的 `{error}` 映射为 `MirroxyError { code, message }`。

### 6.4 事件流（新增 `/api/v1/events`）

替代 5 s / 1 s 轮询。使用 SSE（`text/event-stream`），原因：单向推送足够、Go 标准库即可实现、浏览器 Web 版原生支持 `EventSource`、穿透 HTTP 反代简单。

| 事件 `type` | 负载 | 频率 / 触发 |
|---|---|---|
| `hello` | `{ apiVersion, version, seq }` | 连接建立 |
| `state.snapshot` | 完整 state（与 `/api/state` 相同，去掉 `token`） | 连接建立后一次；`seq` 跳号时客户端主动重拉 |
| `nodes.changed` | `{ nodes: NodeView[] }` 或增量 `{ upsert, remove }` | 导入、改名、起停、排序、订阅刷新 |
| `node.status` | `{ id, status, latency?, health? }` | 核心状态变化、检测结果 |
| `traffic` | `{ total, perNode: {id: {up, down, upRate, downRate, conns}} }` | 1 s；**仅当有订阅者时计算** |
| `proxy.changed` | `{ mode, nodeId }` | 系统代理接管 / 清除 / 交还 |
| `routing.changed` | `{ mode }` | 路由模式 |
| `subscriptions.changed` | 订阅列表视图（不含 URL） | |
| `settings.changed` | `{ key, value }` | 任一设置保存 |
| `downloads` | 下载任务快照 / 进度 | 下载窗口打开时 1 s |
| `log.line` | `{ engine, id, line }` | 仅当客户端显式 `subscribeLog` 时 |
| `update.progress` / `update.restarting` | `{ stage, percent }` | 程序 / 核心 / Geo 更新 |
| `appRouting.changed` | 应用分流状态 | |

规则：
- 每条事件带单调递增 `seq`；客户端发现跳号即重拉 `state.snapshot`。
- 事件负载与 `/api/state` 遵守同一隐私规则：**不含节点密码、订阅 URL、内部应用端口、认证密钥**（扩展 `api_privacy_test.go` 覆盖事件流）。
- 心跳：每 15 s 发送注释行 `: ping`，客户端 45 s 未收到任何字节视为断线。
- 主进程只保持**一条** SSE 连接，向所有窗口扇出；渲染进程按需过滤（下载窗口只关心 `downloads`）。

### 6.5 主进程到守护进程的 HTTP 客户端

- 使用 Node 内置 `http`（不是 `fetch`），以便显式 `agent` 与将来切换 `socketPath`；**禁用一切代理**（Mirroxy 自己会改系统代理，绝不能让 UI 请求绕回节点）。
- 固定目标 `127.0.0.1:<panelPort>`，拒绝重定向，响应体上限 8 MiB（与 `desktop/bridge.go` 一致）。
- 请求头：`Authorization: Bearer <uiToken>`、`X-App-Token`（兼容旧中间件，直至 §7.4 完成）、`Host: 127.0.0.1:<port>`。
- 超时：读 10 s；`subscription/refresh`、`core/update`、`geofiles/update`、`app-update/apply`、`test` 等长操作 120 s，并通过事件流上报进度。

### 6.6 传输层演进（P2，可选）

| 阶段 | 传输 | 说明 |
|---|---|---|
| P1 | 回环 TCP `127.0.0.1:15060` + Bearer 令牌 | Go 改动最小，与浏览器扩展共用端口 |
| P2 | Unix domain socket（macOS/Linux：`<data>/mirroxy.sock`，0600）/ Windows 命名管道 `\\.\pipe\mirroxy-<SID>`（仅当前用户 ACL） | Node `http.request({ socketPath })` 原生支持两者；Go 端 Windows 需引入 `github.com/Microsoft/go-winio`。完成后 Electron 不再依赖面板端口，端口冲突类问题对桌面用户消失 |

P2 只在 P1 稳定后启动，且回环 TCP 保留给浏览器扩展与 AI API。

---

## 7. Go 守护进程改造

原则：**增量、向后兼容**。迁移期间旧 WebUI 必须一直可用（直至 Phase 7 删除）。

### 7.1 新增 `--daemon` 模式

- `--daemon`：不打开浏览器（隐含 `--no-open`），不创建 Go 托盘（P3 起），向 stdout 输出一行 JSON `{"ready":true,"port":15060,"pid":1234}` 便于 UI 诊断。
- 现有 `-ready`、`-shutdown-existing`、`-relaunch-wait` 保持不变，UI 可复用 `-ready` 做探测。

### 7.2 新增 / 调整接口

| 接口 | 类型 | 说明 |
|---|---|---|
| `GET /api/v1/ping` | 新增 | `{ version, apiVersion, pid, platform, startedAt }`；免令牌但仅回环 |
| `GET /api/v1/events` | 新增 | SSE，§6.4 |
| `POST /api/v1/log/subscribe` | 新增 | 打开/关闭某个节点或引擎的日志推送 |
| `GET /api/downloads/*` | 现有 | 下载窗口改走 Electron 后保持不变 |
| `GET /api/app-routing/browse` | 现有 | 桌面端改用原生对话框；**接口保留给远程面板** |
| `/api/state` | 现有 | 新增字段 `apiVersion`；桌面客户端拿到的 state 不再依赖其中的 `token` |

`apiVersion` 从 `1` 开始，只在破坏性变更时递增；UI 声明支持范围 `[min, max]`，不兼容时进入 §5.1 的"版本不匹配"分支（典型场景：用户只更新了 UI 或只更新了后台）。

### 7.3 UI 会话令牌

当前写操作依赖 `/api/state` 返回的 `token`（任何本机进程都能读到，与浏览器同源模型绑定）。桌面端改为：

1. 守护进程启动时生成 32 字节随机令牌，写入 `<data>/ui-session.json`：`{ "token": "...", "port": 15060, "pid": 1234, "apiVersion": 1 }`。
   - Windows：文件 ACL 仅当前用户 + SYSTEM（复用 `storage_key_windows.go` 中的权限逻辑思路）；
   - Unix：`0600`，目录 `0700`。
2. 中间件新增：`Authorization: Bearer` 与该令牌常量时间比较通过时，视为本机已认证客户端，**豁免 Host/Origin/Fetch-Metadata 浏览器专用校验**（这些校验对非浏览器客户端无意义），但仍要求请求来自回环。
3. 守护进程退出时删除文件；每次启动轮换令牌。
4. `ui-session.json` 加入发布与备份排除列表，**绝不进入更新包或发布包**。

### 7.4 中间件调整（`remote.go` 中的 `securityMiddleware`）

```
请求进入
 ├─ 浏览器扩展 connector 路由 → 现有逻辑（不变）
 ├─ 带 Bearer 且令牌匹配 且 来自回环 → local=true，跳过 Host/Origin/X-App-Token 校验
 ├─ 回环浏览器请求（旧 WebUI，过渡期）→ 现有逻辑（不变）
 └─ 远程 → Basic 认证 + 现有限制（不变）
```

新增测试：伪造 Bearer、非回环带 Bearer、过期令牌、令牌文件权限（Windows ACL / Unix mode）。

### 7.5 托盘归属迁移

| 阶段 | Windows 托盘 | macOS / Linux 托盘 |
|---|---|---|
| Phase 1–2 | Go `tray_windows.go`（不变） | 无 |
| Phase 3 起 | Electron 托盘；Go 在 `--daemon` 下不创建托盘 | Electron 托盘 |
| Phase 7 | 删除 `tray_windows.go` 中的菜单逻辑，仅保留"无 UI 安装时的最小托盘"或完全删除（§21 待决） | — |

托盘中"UAC 运行"（以管理员重启守护进程，为应用分流 TUN 助手服务）迁移到 Electron 托盘时，仍由 Go 端 `restart_windows.go` 完成提权重启，Electron 只调用接口。

### 7.6 前端产物嵌入（远程面板）

- Go 通过 `go:embed` 嵌入 `client/apps/web/dist`（替代 `web/`）。
- 为保证 `go build ./...`、`go test ./...` 在未构建前端时仍可通过：嵌入目录中提交一个 `placeholder/index.html`；正式构建脚本先执行前端构建再编译 Go，`verify-release.py` 校验嵌入的不是占位页。
- 远程面板沿用现有安全头（CSP、`X-Frame-Options: DENY` 等）；React 构建产物不使用内联脚本，满足 `script-src 'self'`。

### 7.7 不改动的部分

`core.go`、`deploy.go`、`routing.go`、`import.go`、`health.go`、`traffic.go`、`downloads*.go`、`app_router*.go`、`self_update*.go` 的业务逻辑保持不变；只在这些模块的状态变化处调用新的事件总线 `a.events.Publish(...)`（新增 `events.go`，带订阅者计数，无订阅者时零开销）。

---

## 8. 技术选型

| 领域 | 选型 | 理由 / 备注 |
|---|---|---|
| Electron | 启动时最新 stable 大版本，锁定 `exact` 版本；每季度跟进一次 | Electron 约 8 周一个大版本，仅支持最近 3 个；落后会失去安全补丁 |
| Node（构建） | 当前 LTS，写入 `.nvmrc` / `package.json#engines` | |
| 包管理 | pnpm workspace，`pnpm-lock.yaml` 提交 | 多包共享依赖、锁文件可复现 |
| 构建 | electron-vite（主进程 / preload / 渲染进程三目标） + Vite（Web 版） | 一套配置覆盖三目标与 HMR |
| 打包 | electron-builder，产物 `dir` 目标 → 再由现有 Inno Setup / tar.gz / dmg 脚本包装 | 保留 `packaging/Mirroxy.iss` 的升级与数据保留逻辑 |
| 语言 | TypeScript `strict`，`noUncheckedIndexedAccess` | |
| UI 框架 | React 19 | 生态最大；TanStack 系列成熟（§21 可改 Vue 3，但需尽早定） |
| 数据层 | TanStack Query（请求 / 缓存 / 乐观更新） + 事件流写入 Query 缓存 | 事件到达直接 `setQueryData`，组件无感 |
| 本地 UI 状态 | Zustand | 选中节点、分组、对话框开关等 |
| 路由 | React Router（`createHashRouter`；Web 版同样使用 hash，避免 Go 端改路由） | |
| 表格 | TanStack Table + TanStack Virtual | 千行节点表虚拟滚动、列宽拖拽、排序、多选 |
| 组件基础 | Radix UI Primitives（对话框、菜单、右键菜单、Tooltip、Select、Switch、Tabs） | 无样式、可访问性好，样式沿用自有设计令牌 |
| 样式 | CSS Modules + CSS 变量（从 `web/theme.css` 迁移令牌） | 不引入 Tailwind，避免与现有令牌体系重复 |
| 表单 / 校验 | React Hook Form + zod（与端点注册表共用 schema） | 节点编辑表单字段多 |
| 国际化 | i18next + react-i18next；词条从 `web/i18n.js` 机器迁移 | 中文为源语言，保留"文本即 key"的现有风格或改为语义 key（§21） |
| 图表 | uPlot | 流量曲线，体积小、性能高 |
| 二维码 | `qrcode`（生成）；导入时识别二维码图片用 `jsQR`（纯 JS） | 无原生依赖 |
| 日志（主进程） | `electron-log`，写到系统日志目录，**不写入凭据** | |
| 测试 | Vitest + Testing Library；Playwright（`_electron.launch`）E2E；Go 契约测试 | §16 |
| 代码规范 | ESLint（typescript-eslint、react-hooks）、Prettier、`tsc --noEmit` | |
| 安全加固 | `@electron/fuses`、ASAR 完整性校验 | §13 |

**明确禁止**：任何 Node 原生模块（`*.node`）、`@electron/remote`、`nodeIntegration: true`、`webviewTag`、在渲染进程使用 `eval` / 内联脚本。

---

## 9. 工程目录结构

```
D:\Mirroxy
├─ *.go                          Go 守护进程（保留）
├─ events.go                     新增：事件总线 + SSE
├─ ui_session.go                 新增：UI 会话令牌文件
├─ api/
│  └─ openapi.yaml               接口契约（由 Go 端 AI API openapi.json 扩展而来）
├─ client/                       新增：前端 monorepo（pnpm workspace）
│  ├─ package.json
│  ├─ pnpm-workspace.yaml
│  ├─ tsconfig.base.json
│  ├─ apps/
│  │  ├─ desktop/                Electron 应用
│  │  │  ├─ electron.vite.config.ts
│  │  │  ├─ electron-builder.yml
│  │  │  ├─ src/main/            主进程
│  │  │  │  ├─ index.ts          入口、单实例、生命周期
│  │  │  │  ├─ daemon/           DaemonSupervisor、DaemonClient、EventStream
│  │  │  │  ├─ windows/          mainWindow.ts、downloadsWindow.ts、windowState.ts
│  │  │  │  ├─ tray/             tray.ts、menuBuilder.ts（三平台差异）
│  │  │  │  ├─ ipc/              invoke.ts（端点白名单）、host.ts、prefs.ts
│  │  │  │  ├─ protocol.ts       app:// 协议、CSP
│  │  │  │  ├─ security.ts       导航/新窗口/权限拦截
│  │  │  │  ├─ shortcuts.ts      全局快捷键
│  │  │  │  ├─ autostart.ts      开机自启（Win/mac 用 setLoginItemSettings，Linux 写 .desktop）
│  │  │  │  └─ update.ts         与 Go 更新流程协作
│  │  │  ├─ src/preload/index.ts
│  │  │  ├─ src/renderer/        两个 HTML 入口：main.html、downloads.html（只挂载 packages/ui）
│  │  │  ├─ resources/           图标（由 docs/brand 生成）
│  │  │  └─ e2e/                 Playwright Electron 用例
│  │  └─ web/                    远程 Web 面板构建目标（Vite），产物被 Go embed
│  └─ packages/
│     ├─ api/                    endpoints.ts、schemas、Transport、ElectronTransport、HttpTransport、事件类型
│     ├─ ui/                     React 应用本体
│     │  ├─ src/app/             App、路由、Providers、主题、布局（侧栏 + 状态栏）
│     │  ├─ src/features/        nodes / subscriptions / import / routing-bar / app-routing /
│     │  │                       downloads / settings / cores / geofiles / remote-api /
│     │  │                       updates / changelog / logs
│     │  ├─ src/components/      设计系统组件（Button、Switch、Segmented、Dialog、Toast、Table…）
│     │  ├─ src/hooks/           useHost（原生能力降级）、useShortcuts、useEvent
│     │  └─ src/styles/          tokens.css（来自 web/theme.css）、全局样式
│     └─ i18n/                   zh-CN.json、en.json、迁移脚本
├─ web/                          旧 WebUI（Phase 7 删除）
├─ desktop/                      Wails 下载窗口（Phase 5 后删除）
├─ browser-extension/            不变
├─ android/                      不变（§15）
├─ cmd/upgrade/                  更新助手（扩展整目录替换，§14.4）
└─ packaging/                    发布脚本（扩展 Electron 构建步骤）
```

Electron 的 `userData` / 缓存目录放在系统用户目录（Windows `%APPDATA%\Mirroxy UI`、macOS `~/Library/Application Support/Mirroxy UI`、Linux `~/.config/Mirroxy UI`），**不放在安装目录、不进入发行包与更新包**；其中只存窗口状态与 UI 偏好，不存业务数据。

---

## 10. Electron 主进程设计

### 10.1 模块清单

| 模块 | 职责 | 关键点 |
|---|---|---|
| `index.ts` | 启动编排 | 单实例锁；`second-instance` 时显示并聚焦主窗口；处理 `--hidden`；`app.on('window-all-closed')` 不退出 |
| `DaemonSupervisor` | 定位、拉起、探测、监督守护进程 | 可执行文件路径：打包后为 `process.resourcesPath/../`（与 Electron 同级的 `Mirroxy[.exe]`），开发时读环境变量 `MIRROXY_DAEMON`；子进程 `detached: true` + `unref()`，使 UI 退出不影响守护进程；Windows 用 `windowsHide: true` |
| `DaemonClient` | HTTP 请求 | §6.5 |
| `EventStream` | SSE 解析、重连、seq 校验、扇出 | 手写 SSE 解析（约 80 行），避免依赖 |
| `windows/*` | 窗口创建与状态记忆 | 位置/尺寸/最大化记忆；多显示器越界纠正；`ready-to-show` 后再显示避免白屏 |
| `tray/*` | 托盘与菜单 | §10.4 |
| `ipc/*` | 三个 handler | §6.3 |
| `protocol.ts` | `app://mirroxy/` 自定义协议 | 从 asar 内读静态文件；统一加 CSP 头；拒绝路径穿越 |
| `security.ts` | 安全拦截 | §13 |

### 10.2 窗口

| 窗口 | 尺寸 | 说明 |
|---|---|---|
| 主窗口 | 默认 1200×760，最小 900×560 | 侧栏 + 内容 + 底部状态栏；Windows 11 使用 `backgroundMaterial: 'mica'`，macOS `vibrancy: 'sidebar'` + `titleBarStyle: 'hiddenInset'`，Linux 使用标准标题栏 |
| 下载管理窗口 | 默认 960×600 | 替代 `desktop/`（Wails）；托盘"下载管理"或主窗口侧栏打开；已打开则聚焦；关闭不影响下载 |
| 关于 / 更新日志 | 主窗口内路由 | 不单独开窗 |

两个窗口加载同一 `packages/ui`，通过入口 HTML 决定挂载 `<MainApp/>` 或 `<DownloadsApp/>`。

### 10.3 原生能力映射

| 能力 | 现状 | Electron 实现 |
|---|---|---|
| 选择 v2rayN 目录 / 扫描游戏目录 / 下载目录 | 后端目录浏览接口或手输路径 | `dialog.showOpenDialog({ properties: ['openDirectory'] })` |
| 打开已下载文件 / 所在目录 | `downloads_open*.go` | 维持 Go 实现（路径校验已在 Go 端），或 `shell.showItemInFolder`（需先经 Go 校验路径属于任务） |
| 剪贴板导入（Ctrl+V） | 浏览器 Clipboard API | `clipboard.readText()`，仅在用户按键时读取 |
| 拖放导入 | 无 | 渲染进程接收 `drop`，通过 `webUtils.getPathForFile` 取路径，主进程读取 ≤1 MiB 文本后调用 `import` |
| 二维码图片导入 | 无 | 拖入/粘贴图片 → jsQR 识别 → `import` |
| 系统通知 | 无 | `new Notification()`：下载完成、程序更新可用、系统代理节点失效、订阅刷新失败 |
| 全局快捷键 | 无 | `globalShortcut`，默认不注册；设置里可配置"切换接管系统代理""显示/隐藏窗口" |
| 开机自启 | 安装器/服务 | Win/mac：`app.setLoginItemSettings({ openAtLogin, args: ['--hidden'] })`；Linux：写 `~/.config/autostart/mirroxy.desktop` |
| 深链接 `mirroxy://` | 无 | `app.setAsDefaultProtocolClient('mirroxy')`；仅支持 `mirroxy://import?url=<订阅或分享链接>`，**必须弹出确认对话框展示将导入的内容**，绝不静默导入（§13） |
| 外部链接 | 浏览器新标签 | `shell.openExternal`，仅允许 `https:` 且白名单域名（GitHub 发布页等） |

### 10.4 托盘设计（三平台）

菜单结构对齐现有 `tray_windows.go`，并补齐跨平台：

```
系统代理  ▸   ✓ 节点 A（运行中）
              节点 B
              …（按面板排序；>30 个时显示前 30 个 + "更多…" 打开主窗口）
（当前：节点 A · 接管中）            ← 灰显信息行
路由模式  ▸   ✓ 绕过大陆 / 黑名单 / 全局
接管系统代理  ☑                     ← 新增：直接切换 接管/不改变
──────────
打开 Mirroxy
下载管理
以管理员身份重启后台（仅 Windows）
停止全部代理 / 运行代理
──────────
↑ 12.3 KB/s · 1.2 GB               ← 灰显，每次打开菜单时刷新
↓ 456 KB/s · 8.9 GB
v0.10.0 · 有新版本 0.10.1 ▸ 立即更新
──────────
退出界面（代理继续运行）
退出 Mirroxy
```

- 点选节点 = `proxy.set`（守护进程按需启动该节点并接管系统代理）——满足 C2。
- 菜单在每次打开前根据最新 Query 缓存重建（主进程也维护一份 state 镜像，由事件流更新）。
- 图标：Windows `.ico`；macOS 模板图（`Template.png`，自动适配深浅色菜单栏）；Linux 22/24/32 px PNG。可选状态角标：接管中 / 未接管 / 后台离线三种图标。
- Windows 左键单击显示主窗口，右键菜单；macOS 点击即菜单；Linux 依赖 StatusNotifierItem/AppIndicator（GNOME 需扩展，§20）。
- Tooltip：`Mirroxy · 节点 A · ↑12 KB/s ↓456 KB/s`（Windows 限 127 字符，截断）。

### 10.5 应用菜单与快捷键

- macOS 必须有应用菜单（编辑菜单提供复制粘贴）；Windows/Linux 隐藏菜单栏（`autoHideMenuBar`），快捷键在渲染进程处理。
- 保留 0.9.0 已有的节点表快捷键：回车切为系统代理、空格启动/停止、Ctrl+R 测速、F2 重命名、Ctrl+E 编辑、Ctrl+C 复制分享链接、Delete 删除、方向键移动、Ctrl+F 搜索、Ctrl+A 全选、Ctrl+V 导入（macOS 对应 Cmd）。
- 禁用 Electron 默认的 `Ctrl+R` 刷新与 `Ctrl+Shift+I` 开发者工具（生产构建），避免与"测速"冲突。

### 10.6 UI 偏好存储

`mx:prefs` 使用一个 JSON 文件（原子写：写临时文件后 rename），键白名单：`window.main`、`window.downloads`、`nodeTable.columns`、`nodeTable.sort`、`nodeTable.group`、`closeBehavior`、`firstCloseHintShown`、`language`、`theme`。语言与主题以守护进程设置为准时（若需与远程面板同步），则走 `/api/settings`，偏好文件只做本机覆盖（§21）。

### 10.7 网络隔离

- `app.commandLine.appendSwitch('no-proxy-server')` 并在 `session.defaultSession.setProxy({ mode: 'direct' })`：渲染进程本身几乎不发网络请求（只加载 `app://`），但必须确保不因系统代理被 Mirroxy 自己接管而形成回环。
- `session.webRequest.onBeforeRequest` 拦截渲染进程所有非 `app:` / `devtools:` 请求（更新日志、图标全部打包在本地）。

---

## 11. 前端（渲染进程）设计

### 11.1 页面与布局

沿用 0.9.0 的信息架构（v2rayN 式），只做组件化与交互加固：

```
┌──────────┬───────────────────────────────────────────────────────────────┐
│ Mirroxy  │ [全部] [运行中] [订阅A●] [订阅B] [手动添加] [＋]      🔍  测速  导入 │ ← 分组标签栏
│          ├───────────────────────────────────────────────────────────────┤
│ ▦ 节点面板│ 名称        协议   延迟   本地端口  连接  实时速度   累计流量  │
│ ◫ 应用代理│ ▶ 节点 A   VLESS  82ms   5060     12   ↑1K ↓40K   1.2 GB   │ ← 虚拟化表格
│ ↓ 下载管理│   节点 B   Trojan 超时   5061      0   —          300 MB   │
│ ↻ 订阅管理│   …                                                          │
│ ⚙ 全局设置│                                                              │
│          │                                                               │
│ 语言 主题 ├───────────────────────────────────────────────────────────────┤
│          │ 系统代理 [接管|清除|不改变]  路由 [绕过大陆|黑名单|全局]   节点A · 3 运行 · ↑↓ · 连接 │ ← 状态栏
└──────────┴───────────────────────────────────────────────────────────────┘
```

页面（路由）：`/nodes`、`/apps`（应用分流，仅 Windows 且 `platform.appRouting`）、`/subscriptions`、`/settings`、`/settings/cores`、`/settings/geofiles`、`/settings/remote`（AI API / 远程访问）、`/changelog`；下载窗口独立入口。

### 11.2 状态管理

```
DaemonEvent ──▶ eventRouter ──▶ queryClient.setQueryData(['state'], merge)
                             └─▶ queryClient.setQueryData(['traffic'], …)
用户操作 ──▶ useMutation(endpoint) ──▶ 成功：等待事件流更新（或乐观更新）
                                   └─▶ 失败：回滚 + Toast（错误文案 i18n）
```

- `['state']` 是唯一的服务端状态源；节点、订阅、设置都是它的 selector，避免多份缓存不一致。
- 流量单独一个 query key，1 s 更新时只重渲染速度/流量单元格（表格单元格订阅细粒度 selector，`React.memo` + 稳定行 key）。
- 乐观更新只用于：改名、路由模式、系统代理模式、设置开关；其余等待事件确认。

### 11.3 设计系统

- 设计令牌从 `web/theme.css` 迁移：颜色（亮/暗）、字号、间距、圆角、阴影、动效时长；暗色跟随系统（`nativeTheme.shouldUseDarkColors`）或手动。
- 组件：Button、IconButton、Switch、Segmented（状态栏分段按钮）、Tabs、Dialog、ConfirmDialog、ContextMenu、DropdownMenu、Tooltip、Toast、Field/Input/Textarea/Select、Badge（出口策略小标签、订阅红点）、ProgressBar、EmptyState、DataTable、StatusBar、Banner（后台离线、版本不匹配、证书提示——桌面端不再需要证书提示）。
- 字体：系统字体栈，中文回退 `Microsoft YaHei UI` / `PingFang SC` / `Noto Sans CJK SC`；Linux 发行包不内置字体，文档提示安装 Noto CJK。
- 可访问性：所有交互可键盘完成；对话框焦点陷阱（Radix 提供）；颜色对比度 ≥ 4.5:1。

### 11.4 节点表（最核心组件）

| 需求 | 实现 |
|---|---|
| 1000+ 行流畅 | TanStack Virtual，行高固定 32 px，overscan 10 |
| 列 | 名称、协议、延迟、本地端口、连接、实时速度、累计流量、（可选）出口策略、所属订阅 |
| 排序 | 列头点击；排序方式与分组记忆到 prefs；"自动排序 / 移到底部"等走 `node_order.go` 的现有接口 |
| 选择 | 单击、Ctrl/Shift 多选、Ctrl+A；选中集合存 Zustand，事件更新节点列表时按 id 保持 |
| 双击 | `proxy.set`（按需启动并接管系统代理）；行内显示"切换中…"直到 `proxy.changed` 事件到达 |
| 右键菜单 | 设为系统代理、启动/停止、测速、重命名、编辑、分享（链接/二维码）、复制分享链接、出口策略 ▸、指定应用、日志、移到底部、删除；多选时显示批量可用项 |
| 批量测速 | 并发 4（与现状一致），失败的"忙"错误重试，进度条在工具栏；完成后 Toast 汇总 |
| 延迟着色 | <150 ms 绿、<400 ms 黄、更高橙、超时红、不可用灰（区分"不可用"与"超时"，参见 91c3caf） |
| 分享 | 分享链接只在用户显式点击"分享/复制"时请求 `node/share`，不缓存到 Query |

### 11.5 状态栏与一步切换（C2）

- 系统代理分段按钮：`接管 | 清除 | 不改变`，点击即调用 `proxy.set / proxy.clear / proxy.restore`，**无"应用"按钮**。
  - "接管"时选取节点的规则与现状 `app.js` 一致：当前系统代理节点 → 光标节点 → 选中节点 → 第一个运行中节点；都没有时提示"先双击一个节点"。
- 路由分段按钮：`绕过大陆 | 黑名单 | 全局`，点击即 `routing`，Toast"相关节点的连接会短暂重连"。
- 平台不支持系统代理（`platform.systemProxy === false`，例如 Linux 服务器远程面板）时隐藏系统代理控件，并显示平台限制说明。

### 11.6 宿主能力降级

`useHost()` 返回当前宿主可用能力。Web 版（远程面板）中：

| 能力 | Electron | Web（远程面板） |
|---|---|---|
| 选择目录 | 原生对话框 | 回退到现有后端目录浏览接口 |
| 通知 | 系统通知 | 页面内 Toast |
| 托盘、自启、快捷键 | 有 | 隐藏相关设置 |
| 下载管理 | 独立窗口 | 新标签页路由 `/#/downloads` |
| 修改全局设置/远程访问 | 允许 | 按现有规则：非服务器平台的远程请求禁止修改 `/api/remote`、`/api/settings` |

### 11.7 国际化

- 用脚本把 `web/i18n.js` 中的词条导出为 `packages/i18n/zh-CN.json` 与 `en.json`；CI 检查两份 key 集合一致、无未翻译项。
- 语言默认跟随系统（Electron `app.getLocale()`；Web 版跟随浏览器），可在侧栏切换。
- 更新日志页读取打包内的 `changelog.json` / `changelog.en.json`。

---

## 12. 功能迁移清单

每一项必须达到与 0.9.0 WebUI **行为等价**（含错误提示、平台限制说明）后才能勾选。"验收"列是 E2E 或手工检查点。

### 12.1 节点面板

| 功能 | 现有来源 | 后端接口 | 验收 |
|---|---|---|---|
| 分组标签（全部/运行中/各订阅/手动） | `node-list.js` | `state` | 订阅刷新失败显示红点与原因 |
| 节点表、排序、多选、快捷键 | `node-table.js`、`node-list.js` | `state`、`traffic` | §11.4 全部 |
| 双击 / 回车切换系统代理 | `app.js#switchNode` | `proxy {set}` | E1 |
| 启动 / 停止 / 批量 | `app.js#batchNode` | `node {start/stop}` | E3 |
| 测速 / 批量测速 | `app.js` | `test` | 区分不可用与超时 |
| 改名 | `renameDialog` | `node/rename` | E4：不重启核心 |
| 编辑节点 | `node-edit.js` | `node/spec`、`node/edit` | 字段校验、警告显示 |
| 分享链接 / 二维码 / 批量复制 | `nodeExportDialog` | `node/share` | 链接仅显式获取 |
| 出口策略（每节点） | 右键菜单 | `node {route}` | 与全局不同时显示小标签 |
| 移到底部 / 自动排序 / 重新分配端口 | `node_order.go` | `v1/nodes/*` 或 `node` | |
| 核心日志 | `logDialog` | `log`（迁移后 + `log.line` 事件） | 实时尾随、复制 |
| 删除（单个/批量） | 右键 | `node {delete}` | 不影响其它节点 |
| Ctrl+V 导入 / 粘贴订阅地址自动打开添加订阅 | `app.js` | `import`、`subscription/add` | |

### 12.2 导入

| 功能 | 后端接口 | 备注 |
|---|---|---|
| 从 v2rayN 导入（仅 Windows） | `import {path}` | 目录用原生对话框选择 |
| 分享链接（多行） | `import {text}` | |
| 订阅地址 | `subscription/add` → `subscription/refresh` | |
| 拖放文件 / 二维码图片 / 深链接 | `import {text}` | 新增；深链接必须确认 |

### 12.3 订阅管理

表格（名称、域名、节点数、最后更新、状态）、更新 / 全部更新、编辑（名称、替换地址）、删除（保留节点 / 连同节点）、点名称跳转分组；接口：`subscription/add|edit|delete|refresh`。订阅 URL 永不在界面完整显示。

### 12.4 状态栏

系统代理三态、路由三态、当前节点、运行数量、总速度、连接数（§11.5）。

### 12.5 游戏 / 应用代理（Windows）

| 功能 | 接口 |
|---|---|
| 默认应用节点、兼容模式、配置列表 | `app-routing/config` |
| 添加应用：运行中进程选择、目录浏览/扫描 exe | `GET /api/processes`、`GET /api/app-routing/browse`、`GET /api/app-routing/scan`（桌面端目录改原生对话框，扫描仍在 Go） |
| 节点"指定应用"对话框 | 同上 |
| 开启/停止（含加速器冲突检测确认） | `app-routing/conflicts`、`app-routing/start|stop` |
| 能力检测 | `node/capabilities` |
| 平台限制说明（非 WFP kill-switch） | 文案保留，见 `PROJECT_SUMMARY.md` §5 |

### 12.6 下载管理（替代 `desktop/` Wails 客户端）

任务列表、添加（URL、目录、代理方式 direct/node/custom、连接数、限速）、暂停/继续/删除（可删文件）、回收站（`downloads_trash.go`）、打开文件/目录、下载设置（默认目录、代理、连接数、限速）、aria2 安装状态、浏览器扩展接力设置入口；接口：`downloads`、`downloads/add|action|settings|install`；进度改为 `downloads` 事件。验收：关闭窗口下载继续；任务文件缺失可删除（参见 ab771ed）。

### 12.7 全局设置

逐项即时保存（现有 `markSetting` 行为：保存中 / 已保存 / 失败），包括：语言、主题、开机自启（新增，由 Electron 处理）、关闭窗口行为（新增）、订阅自动更新间隔、健康检测、端口起点、浏览器接力等现有 `settings.go` 全部键。

### 12.8 核心 / Geo / 程序更新 / 更新日志

| 功能 | 接口 |
|---|---|
| 核心版本、检查、更新 | `core/check`、`core/update` + `update.progress` |
| Geo 文件状态、检查、更新单个/全部 | `geofiles/check`、`geofiles/update` |
| 程序更新检查与应用 | `app-update/check`、`app-update/apply` + `update.restarting` |
| 更新日志 | 本地 `changelog*.json` |

### 12.9 AI API / 远程访问

远程开关、端口、用户名、密码设置、证书/私钥路径、HTTPS/HTTP、错误显示；AI API 指南与 `openapi.json` 查看；接口：`remote`、`v1/guide`、`v1/openapi.json`。

### 12.10 托盘（§10.4）与其它

加速器避让提示（`accelerator_guard*.go`）、"全部停止"（`stop-all`）、关于页（版本、许可证 `licenses/`、第三方组件声明）。

---

## 13. 安全设计

### 13.1 Electron 加固清单（全部为必须项）

| 项 | 设置 |
|---|---|
| `contextIsolation` | `true` |
| `sandbox` | `true`（preload 也在沙箱中，只用 `contextBridge` / `ipcRenderer`） |
| `nodeIntegration` / `nodeIntegrationInWorker` / `nodeIntegrationInSubFrames` | `false` |
| `webSecurity` | `true`；`allowRunningInsecureContent: false` |
| `webviewTag` | `false` |
| 内容加载 | 只加载 `app://mirroxy/*`；不加载任何 `http(s)://` 页面 |
| CSP | `default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'`（协议处理器统一下发） |
| 导航拦截 | `will-navigate`、`will-redirect` 一律阻止；`setWindowOpenHandler` 返回 `deny`，外链交给 `shell.openExternal` 白名单 |
| 权限请求 | `session.setPermissionRequestHandler` 全部拒绝（除 `clipboard-sanitized-write`、`notifications`） |
| DevTools | 生产构建禁用（`devTools: false`），可通过环境变量 `MIRROXY_DEVTOOLS=1` 在开发构建中开启 |
| Fuses | `RunAsNode=false`、`EnableNodeOptionsEnvironmentVariable=false`、`EnableNodeCliInspectArguments=false`、`EnableCookieEncryption=true`、`EnableEmbeddedAsarIntegrityValidation=true`、`OnlyLoadAppFromAsar=true`、`GrantFileProtocolExtraPrivileges=false` |
| IPC | 4 个固定通道；校验 `senderFrame`；zod 校验入参；端点白名单；长度上限 |
| 依赖 | `pnpm audit` 进 CI；锁文件提交；依赖升级走 PR；主进程依赖尽量为零（只用 `electron-log`、`zod`） |

### 13.2 数据与凭据

- 渲染进程永不接触 UI 会话令牌：令牌只在主进程内存中。
- 节点密码、订阅 URL 只在用户显式"分享 / 编辑 / 导出"时出现；不写入 Electron 日志、不进入 Query 持久化（不启用 Query 持久化插件）。
- Electron `userData` 中不保存任何业务数据；崩溃报告（若启用）默认关闭，开启前需用户同意且不上传。
- 深链接与拖放导入的内容必须先在确认对话框中展示（订阅地址只显示域名），由用户确认后才调用 `import`。

### 13.3 守护进程侧

- Bearer 令牌只接受回环连接；令牌文件权限测试纳入 `go test`。
- SSE 只对已认证的本机客户端或已认证的远程会话开放；远程 SSE 同样受 Basic 认证与失败次数限制。
- 远程面板继续禁止修改本机敏感设置（`/api/remote`、`/api/settings` 在非服务器平台）。

### 13.4 威胁模型摘要

| 威胁 | 缓解 |
|---|---|
| 本机恶意网页访问 15060 端口 | 现有 Host/Origin/Fetch-Metadata/X-App-Token 校验保留；桌面端走 Bearer |
| 本机其它用户读取令牌 | 令牌文件 ACL/0600；P2 管道/UDS 仅当前用户 |
| 渲染进程 XSS（节点名称含 HTML） | React 默认转义；禁止 `dangerouslySetInnerHTML`（ESLint 规则）；CSP 无内联脚本 |
| 篡改安装目录中的 asar | ASAR 完整性校验 + 代码签名（Windows Authenticode、macOS 公证） |
| 利用 Electron 作为 Node 运行任意脚本 | `RunAsNode` fuse 关闭 |
| 深链接诱导导入恶意订阅 | 强制确认对话框，展示来源与内容 |

---

## 14. 打包、安装与在线更新

### 14.1 产物布局（安装后）

```
<安装目录>/
├─ Mirroxy.exe                 Go 守护进程（不变，仍为后台主体）
├─ upgrade.exe                 更新助手（扩展整目录替换）
├─ bin/                        核心与资源（不变）
├─ licenses/                   许可证（追加 Electron/Chromium 许可证）
├─ ui/                         Electron 应用目录（electron-builder 的 dir 产物）
│  ├─ Mirroxy UI.exe           Electron 可执行（Windows；macOS 为 Mirroxy.app 内部，见下）
│  ├─ resources/app.asar
│  ├─ locales/ …               Chromium 运行时文件
│  └─ LICENSES.chromium.html
├─ manifest.json
└─ data/                       用户数据（不在任何发布物中）
```

- 快捷方式、开始菜单、开机自启指向 `ui/Mirroxy UI.exe`；`Mirroxy.exe` 作为后台由 UI 拉起。
- **macOS**：以 `Mirroxy.app` 为整体分发，Go 守护进程与 `bin/` 放在 `Mirroxy.app/Contents/Resources/daemon/`；用户数据放在 `~/Library/Application Support/Mirroxy/data`（与现有 `install-macos.sh` 的数据位置对齐，迁移脚本见 §14.7）。需要 Developer ID 签名 + 公证；守护进程与核心也必须签名，否则 Gatekeeper 拦截。
- **Linux**：tar.gz 解压到 `/opt/mirroxy` 或用户目录，附 `mirroxy.desktop`、图标、`install.sh` 扩展；可选 AppImage（仅 UI + 守护进程，便携）。无界面服务器继续使用不含 `ui/` 的服务器包（§15.2）。

### 14.2 构建流程（`packaging/build-release.py` 扩展）

```
1. pnpm install --frozen-lockfile
2. pnpm -C client typecheck && pnpm -C client test
3. pnpm -C client build:web          → client/apps/web/dist（供 Go embed）
4. go build（四平台，现有步骤）       → Mirroxy[.exe]、upgrade[.exe]
5. pnpm -C client build:desktop      → electron-vite 构建
6. electron-builder --dir --<os> --<arch>
      Windows x64 / macOS arm64 / Linux x64 / Linux arm64
7. 组装目录：守护进程 + bin + licenses + ui/
8. 签名：Windows（signtool，若有证书）、macOS（codesign + notarytool + stapler）
9. 打包：Windows Inno Setup（Mirroxy.iss 增加 ui\* 文件段与快捷方式）
         macOS .dmg（含 .app）+ 在线更新 zip
         Linux tar.gz（桌面包、服务器包两种）
10. 生成在线更新包 Mirroxy-update-<os>-<arch>.zip（§14.4）
11. SHA256SUMS、verify-release.py（白名单、无 data/凭据、嵌入的不是占位页、asar 完整性）
```

Linux arm64：项目不使用原生模块，electron-builder 可在 x64 主机打包 arm64；但**冒烟测试必须在 arm64 runner（`ubuntu-24.04-arm`）上启动一次**，否则不宣称可用（C8）。

### 14.3 包体预估

| 平台 | 当前完整包 | Electron 版完整包（预估） |
|---|---|---|
| Windows x64（安装器） | 约 40–60 MB（含核心） | +85–100 MB |
| macOS arm64（dmg） | — | +95–110 MB |
| Linux x64/arm64（桌面 tar.gz） | — | +90–105 MB |
| 在线更新包 | 小（守护进程 + 资源） | 含 UI 时 +85–100 MB；仅后台更新时不变（§14.4） |

### 14.4 在线更新设计（C5、C6）

**问题**：现有更新白名单只允许 `Mirroxy[.exe]`、`upgrade[.exe]`、`manifest.json`、`bin/resources/Mirroxy_set_*.bin`、`licenses/*`；Electron 是一个几百个文件的目录，且 Windows 上运行中的文件会被锁定。

**方案：manifest 分组件，UI 目录整体原子替换**

```json
// Mirroxy-update-windows-amd64.zip 内的 manifest.json
{
  "version": "0.10.1",
  "minUpdaterVersion": "0.10.0",
  "components": {
    "daemon": { "files": ["Mirroxy.exe", "upgrade.exe"], "sha256": { "Mirroxy.exe": "…" } },
    "ui":     { "dir": "ui", "electron": "x.y.z", "archive": "ui.zip", "sha256": "…" }
  }
}
```

1. **白名单扩展**：`allowedUpdateFile` 增加 `ui.zip`（UI 目录作为内层 zip，便于整体校验与原子替换）；内层 zip 解压时同样执行 `filepath.IsLocal`、无反斜杠、仅普通文件、无重复、总大小上限（512 MiB）、文件数上限（2000）校验。
2. **UI 组件可选**：若该版本 UI 未变，`components.ui` 省略，更新包大小与现在一致；版本号一致时 UI 可单独热更？——**不做**：守护进程与 UI 始终同版本发布，避免 `apiVersion` 组合爆炸。UI 未变时仅表示"沿用旧 ui 目录"。
3. **Windows 流程**：
   1. UI 触发 `app-update/apply` → 守护进程下载、校验 → 广播 `update.restarting` 并在 `<data>/update-plan.json` 记录 `uiWasRunning: true`。
   2. UI 收到事件：显示"程序更新中，代理会短暂重启"，3 s 后主动退出（释放 `ui\` 文件锁）。
   3. `upgrade.exe` 等待守护进程与 UI 进程（plan 中记录的 PID）退出 → 备份 `ui\` 为 `ui.bak\`、守护进程为 `.bak` → 解压新 `ui\` 到 `ui.new\` → 两次 rename 原子切换 → 启动守护进程 `--resume-update`。
   4. 守护进程就绪后若 `uiWasRunning`，以 `--hidden=false` 拉起 UI；UI 连上后显示"已更新到 0.10.1"。
   5. 任一步失败：回滚 `ui.bak` 与守护进程（复用现有回滚逻辑），并在日志与下次打开 UI 时提示。
4. **Linux 流程**：守护进程继续原 PID exec（systemd 监管不变）；`ui/` 目录用 `ui.new` → rename 替换（运行中进程持有旧 inode，不受影响）；UI 收到 `update.restarting` 后自行重启（`app.relaunch(); app.exit()`）。
5. **macOS 流程**：替换整个 `Mirroxy.app` 会破坏签名验证的连续性，采用"下载新 .app 到临时目录 → 校验签名（`codesign --verify`）→ 守护进程退出 → 助手替换 `.app` → 重新打开"。macOS 助手由 `upgrade`（Go）实现，与 Windows 共用逻辑。
6. **过渡版本**：0.9.x 的旧更新器不认识 `ui.zip`，因此 **首个 Electron 版本（暂定 0.10.0）只能通过完整安装包安装**；发布说明明确写出。从 0.10.0 起在线更新覆盖 UI。旧更新器遇到含 `ui.zip` 的包应拒绝更新并提示"请下载完整安装包"（当前 `allowedUpdateFile` 会拒绝未知文件，行为满足要求，需测试确认）。
7. **不使用 electron-updater / Squirrel**：它们与"Go 守护进程 + 独立助手 + 数据保留 + GitHub Releases 不可变字节"的现有体系冲突，并会引入第二套更新源。

### 14.5 发布

- `packaging/publish-release.py` 上传新增产物到 GitHub Releases；同一版本已存在且字节不同 → 失败（C11）。
- 发布说明模板新增"各平台实机验证状态"表（§16.6）。

### 14.6 发布物白名单

完整包只允许：守护进程、更新助手、`bin/`（核心与资源）、`licenses/`、`ui/`（electron-builder 产物）、安装脚本、使用说明、`manifest.json`、`SHA256SUMS`。`verify-release.py` 增加断言：不含 `data/`、`*.db`、`*.key`、`ui-session.json`、`*.log`、`.env`、`*.pem`（证书私钥）、源码映射 `*.map`（生产构建关闭 sourcemap 或单独保存不发布）。

### 14.7 从 0.9.x 迁移

| 项 | 处理 |
|---|---|
| `data/`（数据库、密钥、流量） | 原地保留，守护进程版本迁移逻辑不变；安装器不触碰 |
| 面板端口设置 | 保留（浏览器扩展与远程面板仍使用） |
| 浏览器中的旧面板书签 | 0.10.x 过渡期仍可访问旧 WebUI（设置中可关闭）；Phase 7 后访问本机面板地址显示"请使用 Mirroxy 桌面客户端"并提供"打开客户端"按钮（通过 `mirroxy://open`） |
| `Mirroxy-downloads.exe`（Wails） | 安装器在 0.10.0 删除，托盘改为打开 Electron 下载窗口 |
| 开机自启 | 安装器把旧的"启动 Mirroxy.exe"自启项替换为"启动 UI --hidden"；保留用户原有开关状态 |
| macOS 数据位置 | 若现有安装把 data 放在安装目录，首次启动由守护进程迁移到 Application Support（先复制、校验、再标记旧目录，不删除） |

---

## 15. Android 与无界面服务器

### 15.1 Android

Electron 不支持移动端。Android 继续使用现有 Kotlin + Compose + libXray 工程（`android/`），独立发版、独立 QA（`docs/ANDROID-QA.md`）。可共享的仅有：设计令牌（颜色、圆角数值）、i18n 词条（可由 `packages/i18n` 导出为 Android `strings.xml` 的脚本，可选）、更新日志格式。前一版 Flutter 计划中的"Android 合流"在本方案中**取消**。

### 15.2 Linux 无界面服务器

- 服务器包不含 `ui/`，只含守护进程、核心、`mirroxy.service`、`mirroxyctl`（现状不变）。
- 远程管理继续使用 HTTPS 远程面板，前端换成 `client/apps/web` 的 React 构建产物——**与桌面客户端完全同一套代码**。
- 桌面客户端可选"连接远程后台"（Phase 6 之后的增强）：主进程以 HTTPS + Basic 认证连接远程守护进程，复用全部界面；远程模式下隐藏本机专属功能（系统代理、应用分流、托盘点选切换）。

---

## 16. 测试策略

### 16.1 测试金字塔

| 层 | 工具 | 覆盖 |
|---|---|---|
| Go 单元 / 集成 | `go test ./...`（现有，含 `-race`） | 新增：事件总线、SSE、令牌文件、中间件 Bearer 分支、更新包 `ui.zip` 校验、事件隐私 |
| 契约 | Go 测试读取 `client/packages/api` 导出的 JSON Schema，对每个端点的真实响应做 schema 校验 | 防止字段漂移 |
| 前端单元 | Vitest | 纯函数：排序、选择逻辑、速率格式化、延迟着色、i18n key 一致性；迁移现有 `packaging/test-*.cjs` 的用例 |
| 组件 | Vitest + Testing Library + jsdom | 节点表交互、对话框表单、状态栏 |
| 主进程 | Vitest（Node 环境） | EventStream 解析/重连、端点白名单、IPC 入参校验、路径校验 |
| E2E | Playwright `_electron.launch()` + 隔离守护进程 | §16.4 |
| 视觉回归 | Playwright 截图（亮/暗、中/英） | 主要页面 |

### 16.2 隔离测试环境（C13）

- E2E 启动守护进程时使用 `--dir <临时目录>` 与随机面板端口；临时目录内放测试用 `bin/`（真实核心或模拟核心）。
- 系统代理相关用例：默认运行在"模拟系统代理"模式（新增测试构建标签 `mirroxy_fakeproxy`，Go 端用内存实现替代 `windows.go` 系统代理调用），**绝不修改测试机的真实系统代理**；真实系统代理用例只在专用虚拟机手工执行。
- Electron 测试实例使用独立 `userData`（`--user-data-dir=<临时目录>`）。

### 16.3 契约测试示例

```
client/packages/api  ── pnpm export-schemas ──▶ api/schemas/*.json
go test ./... -run TestContract  ──▶ 启动隔离 App，逐个调用端点，校验响应 ⊆ schema，且无敏感字段
```

### 16.4 E2E 关键用例

| 编号 | 用例 | 对应约束 |
|---|---|---|
| E1 | 双击节点 → 节点启动 → 系统代理（模拟）指向该节点端口；状态栏显示接管 | C2 |
| E2 | 托盘点选另一节点 → 一步切换；路由模式点选即生效、无"应用"按钮 | C2 |
| E3 | 节点 A、B 运行，A 上保持一个长连接；停止 B → A 的核心 PID 不变、长连接未断 | C1 |
| E4 | 改名 → 核心 PID 不变；刷新订阅 → 自定义名称保留 | C4 |
| E5 | 关闭窗口 → 守护进程与核心仍运行；重开窗口状态正确 | §5.2 |
| E6 | 杀死守护进程 → UI 显示离线横幅 → 重新启动后台 → 恢复 | §5.4 |
| E7 | 下载窗口：添加任务、关闭窗口、任务继续、重开看到进度 | §12.6 |
| E8 | 深链接导入必须出现确认对话框 | §13.2 |
| E9 | 在线更新（本地伪造发布源）：节点、端口、累计流量保留；UI 自动回来 | C5、C6 |
| E10 | 1000 节点数据集下滚动、排序、批量测速不卡顿（记录帧时间） | §18 |

### 16.5 手工验收清单（每次发布）

- Windows：与 v2rayN（`127.0.0.1:10808`）共存，默认不改变系统代理；接管后退出恢复原设置。
- 安装器：全新安装、0.9.x → 0.10.0 升级（数据保留）、同版本重装无变化。
- 在线更新：本机先备份 → 应用 → 节点、端口、累计流量保留（AGENTS.md 要求）。
- 托盘三平台菜单项、点选切换、退出两种语义。
- 中英文、亮暗主题、125%/150% 缩放、多显示器。

### 16.6 平台验证矩阵（发布说明必须附带）

| 项 | Windows x64 | macOS arm64 | Linux x64 | Linux arm64 |
|---|---|---|---|---|
| CI 构建 | ☐ | ☐ | ☐ | ☐ |
| CI E2E（模拟代理） | ☐ | ☐ | ☐ | ☐（arm64 runner） |
| 实机：安装 / 启动 / 托盘 | ☐ | ☐ | ☐ | ☐ |
| 实机：系统代理接管 / 恢复 | ☐ | ☐ | ☐ | ☐ |
| 实机：在线更新保留数据 | ☐ | ☐ | ☐ | ☐ |

未勾选的实机项在发布说明中写"未实机验证"，不得宣称通过（C8）。

---

## 17. CI/CD

### 17.1 `.github/workflows/ci.yml` 扩展

```
jobs:
  go-test        (ubuntu, windows)    现有：build / vet / test；新增 -run TestContract
  frontend       (ubuntu)             pnpm install → lint → typecheck → vitest → build:web
  electron-e2e   (windows, ubuntu, macos-14)
                                      构建守护进程 + desktop → Playwright E2E（Linux 用 xvfb-run）
  electron-arm64 (ubuntu-24.04-arm)   打包 + 启动冒烟（--smoke-test：启动、连上守护进程、退出码 0）
  cross-compile  (ubuntu)             现有 Go 四平台交叉编译
  audit          (ubuntu)             pnpm audit --prod、go vet、许可证清单检查
```

### 17.2 发布工作流

手动触发 `release.yml`：四平台矩阵构建 → 签名（密钥来自 GitHub Secrets，仅在受保护分支 / tag 上可用）→ 产物上传为 workflow artifact → 本机或受控 runner 执行 `publish-release.py`（保持"同版本不可替换字节"的检查）。发布凭据不进入仓库与产物（C7）。

### 17.3 Git 规范

- 每个 Phase 一个功能分支（如 `feat/electron-phase1-daemon`），小步提交；改动前确认基线提交，改动后提交（C10）。
- 提交信息沿用现有中文 Conventional Commits 风格（`feat:` / `fix:` / `docs:`）。
- 每个面向用户的变化写入 `web/changelog.json` 与 `web/changelog.en.json`（C9）。

---

## 18. 性能预算

| 指标 | 目标 | 测量方法 |
|---|---|---|
| 冷启动到主窗口可交互（守护进程已运行） | ≤ 1.5 s（Windows 中端机） | E2E 打点：`app.ready` → 首屏 `state.snapshot` 渲染完成 |
| 冷启动（需拉起守护进程） | ≤ 3 s | 同上 |
| 从托盘重新打开窗口（窗口已销毁） | ≤ 400 ms | |
| 节点表 1000 行滚动 | 帧时间 p95 ≤ 16 ms | Playwright + Performance API |
| 流量 1 s 刷新的渲染开销 | ≤ 3 ms/帧 | React Profiler |
| 常驻内存：仅托盘（窗口销毁） | ≤ 90 MB（主进程 + GPU 进程） | 任务管理器 / `app.getAppMetrics()` |
| 常驻内存：主窗口打开 | ≤ 250 MB | 同上 |
| 守护进程 CPU（无 UI 订阅时） | 与 0.9.0 持平 | 事件总线无订阅者零开销 |
| 渲染进程 JS 包 | 首屏 ≤ 600 KB gzip | Vite 构建报告；路由级代码分割 |

缓解手段：窗口隐藏即销毁、`backgroundThrottling` 保持开启、隐藏时停止订阅 `traffic` 事件（主进程告知守护进程无订阅者）、禁用不需要的 Chromium 功能（拼写检查等）。

---

## 19. 分阶段实施计划

估算口径：1 名主力开发 + AI 辅助，单位为周。每阶段有出口条件，不满足不进入下一阶段；每阶段结束都要更新 `web/changelog.json`（若有用户可见变化）并提交。

### Phase 0 · 技术验证（1 周）

- 四平台最小 Electron 壳：窗口 + 托盘 + 通过主进程调用 `/api/state` + 订阅一个原型 SSE。
- 验证：Windows Mica、macOS 菜单栏模板图标、Linux（GNOME + AppIndicator 扩展 / KDE）托盘、Linux 中文输入法（fcitx5 / ibus）、arm64 runner 启动、Fuses + ASAR 完整性在 Windows/macOS 生效、窗口销毁重建耗时、内存基线。
- 验证 Windows 上 UI 退出后 `ui\` 目录可被 rename 替换（文件锁释放时序）。

**出口**：Spike 报告 `docs/ELECTRON-SPIKE.md`（含实测数据）；§21 中的待决项全部定稿。

### Phase 1 · 守护进程改造（2 周）

- `--daemon`、`/api/v1/ping`、`ui-session.json` 令牌、Bearer 中间件分支、事件总线 + `/api/v1/events`（全部事件类型）、`apiVersion`。
- 各业务模块在状态变化处发布事件；无订阅者零开销。
- 隐私测试覆盖事件流；令牌文件权限测试；契约测试框架。

**出口**：旧 WebUI 行为完全不变；`go test ./...`（含 `-race`）全绿；用 `curl` 可观察到全部事件类型。

### Phase 2 · 前端工程与设计系统（2 周）

- 建立 `client/` monorepo、ESLint/Prettier/TS 配置、`packages/api`（端点注册表、两种 Transport、事件类型）、`packages/i18n`（词条迁移脚本）、`packages/ui` 设计系统组件与布局骨架、Electron 主进程框架（单实例、DaemonSupervisor、IPC、协议、安全拦截、窗口状态、托盘骨架）。
- CI `frontend` 与 `electron-e2e`（冒烟级）job。

**出口**：Electron 壳四平台可启动，显示后台版本与节点数量；主题/语言切换可用；安全清单 §13.1 全部落实并有测试。

### Phase 3 · 日常核心路径（3–4 周）

- 节点面板全部功能（§12.1）、导入（§12.2）、订阅管理（§12.3）、状态栏（§12.4）、三平台托盘（§10.4）、Go 在 `--daemon` 下停用 Windows 托盘。
- E2E：E1–E6、E10。

**出口**：日常使用不再需要打开旧 WebUI；Windows 内部 alpha（作者本机：备份后安装，确认与 v2rayN 共存、节点与流量保留）。

### Phase 4 · 进阶功能（2–3 周）

- 全局设置、核心 / Geo / 程序更新界面、AI API / 远程访问、更新日志、日志实时尾随、应用分流（§12.5）、加速器避让提示、通知、开机自启、全局快捷键、拖放与二维码导入、深链接。

**出口**：§12 清单除下载管理外全部勾选；E8 通过。

### Phase 5 · 下载管理窗口（1–2 周）

- 用 Electron 下载窗口替代 `desktop/`（Wails）；托盘"下载管理"指向新窗口；浏览器扩展接力回归测试（`packaging/verify-extension-*.cjs`）。
- 删除 `desktop/` 模块与 `build.ps1` 中的 Wails 构建步骤（在过渡版本发布后）。

**出口**：E7 通过；§12.6 全部勾选。

### Phase 6 · 打包、安装与在线更新（2–3 周）

- electron-builder 配置、`build-release.py` / `Mirroxy.iss` / macOS / Linux 打包扩展、签名与公证、`upgrade` 整目录替换与回滚、`allowedUpdateFile` 扩展与 `ui.zip` 校验、`verify-release.py` 白名单扩展、从 0.9.x 迁移逻辑。
- Web 版（`client/apps/web`）接入 Go embed，远程面板切换为 React 版。

**出口**：CI 产出四平台完整包与四个 `Mirroxy-update-*.zip`；E9 通过；至少 Windows 完成实机"备份 → 完整包升级 0.9.x→0.10.0 → 在线更新 0.10.0→0.10.1（测试版本号）→ 节点、端口、累计流量保留"。

### Phase 7 · Beta 过渡与切换（1–2 周）

- 发布 0.10.0（过渡版）：默认使用 Electron 客户端，旧 WebUI 在设置中可临时开启；发布说明附平台验证矩阵。
- 收集问题、修复回归；下一版本删除 `web/` 旧代码、Go 端仅为旧 WebUI 服务的代码路径（保留浏览器扩展、AI API、远程面板所需接口）、`tray_windows.go` 菜单逻辑（按 §21 决议）。
- 更新 `AGENTS.md`（changelog 路径、日常路径描述）、`README*.md`、`CHANGELOG.md`、`docs/PROJECT_SUMMARY.md`；在 `FLUTTER-MIGRATION-PLAN.md` 顶部标注"已被本文取代"。

**出口**：旧 WebUI 删除；CI 全绿；发布说明标明各平台实机验证状态。

### 工期汇总

| 阶段 | 周 | 可并行 |
|---|---|---|
| 0 技术验证 | 1 | — |
| 1 守护进程改造 | 2 | 与 Phase 2 后半并行 |
| 2 前端工程与设计系统 | 2 | |
| 3 日常核心路径 | 3–4 | |
| 4 进阶功能 | 2–3 | 与 Phase 5 并行 |
| 5 下载管理窗口 | 1–2 | |
| 6 打包与在线更新 | 2–3 | 可从 Phase 4 开始做打包原型 |
| 7 Beta 与切换 | 1–2 | |
| **合计** | **约 14–19 周**（并行后约 12–15 周） | |

### 里程碑

| 里程碑 | 内容 |
|---|---|
| M1 | Phase 1 完成：事件流可用，旧 WebUI 不受影响 |
| M2 | Phase 3 完成：Windows 内部 alpha，日常路径全部可用 |
| M3 | Phase 6 完成：四平台发布物与在线更新就绪 |
| M4 | 0.10.0 过渡版发布 |
| M5 | 旧 WebUI 与 Wails 删除 |

---

## 20. 风险与对策

| 风险 | 概率 | 影响 | 对策 |
|---|---|---|---|
| 包体与内存显著增大，用户反感 | 高 | 中 | 窗口隐藏即销毁；发布说明说明原因；守护进程可独立运行；评估后可提供"仅后台"精简包 |
| Linux GNOME 默认不显示托盘 | 高 | 中 | 检测无托盘环境时关闭窗口不隐藏而是最小化；文档提示安装 AppIndicator 扩展；`.desktop` 快捷方式始终可用 |
| Windows 更新时 UI 目录文件锁 | 中 | 高 | 助手等待 UI PID 退出 + 重试 rename + 回滚；Phase 0 验证 |
| macOS 签名 / 公证需要开发者账号 | 中 | 高 | 提前申请；未签名版本在发布说明标注并给出手动放行步骤；不宣称已验证 |
| Electron 大版本快速迭代、旧版本停止支持 | 高 | 中 | 每季度升级一次；无原生模块使升级成本低；E2E 兜底 |
| 迁移期间双 UI 维护 | 中 | 中 | Phase 1 起冻结旧 WebUI 新功能 |
| 端点注册表与 Go 实现漂移 | 中 | 中 | 契约测试进 CI |
| 用户只更新了后台或只更新了 UI | 低 | 中 | `apiVersion` 协商 + 版本不匹配提示；两者始终同版本发布 |
| 系统代理被接管后 UI 请求回环到节点 | 中 | 高 | 主进程强制直连、渲染进程禁止外网请求（§10.7） |
| Chromium 安全漏洞 | 中 | 中 | 只加载本地内容 + 沙箱 + Fuses；按季度升级，重大 CVE 立即升级 |
| 远程面板换成 React 后行为回归 | 中 | 中 | Web 版跑同一套组件测试；Linux 服务器在 Phase 6 实机回归 |

---

## 21. 待决策事项

Phase 0 结束前需要定稿：

| # | 问题 | 建议 |
|---|---|---|
| D1 | UI 框架 React 还是 Vue 3 | **React**（TanStack Table/Virtual、Radix 生态最完整） |
| D2 | i18n key 保持"中文原文即 key"还是改为语义 key | 改为语义 key（`nodes.switchSuccess`），迁移脚本自动生成并保留中文为源语言 |
| D3 | Phase 7 后 Go 端 Windows 托盘是删除还是保留最小托盘 | 删除；"仅后台运行"场景用服务 / 开机自启 UI `--hidden` 覆盖 |
| D4 | 语言、主题存守护进程还是 UI 偏好 | 存守护进程设置（远程面板与桌面一致），UI 偏好只存窗口与表格布局 |
| D5 | 是否在 P2 实现命名管道 / UDS | 是，但排在 0.10.x 之后，不阻塞首发 |
| D6 | Windows 安装器继续 Inno Setup 还是改 NSIS | 继续 Inno Setup，保留现有升级、端口、数据保留逻辑 |
| D7 | Linux 是否提供 AppImage / deb | 首发只提供 tar.gz；AppImage 作为后续增强 |
| D8 | 是否提供"连接远程后台"的桌面模式 | 作为 Phase 7 之后的增强 |
| D9 | 首个 Electron 版本号 | 0.10.0（首个需完整包安装的版本） |

---

## 22. 附录

### 22.1 端点注册表初稿（来自 `api.go` / `ai_api.go` / `downloads.go`）

| 端点 key | 方法 | 路径 | 备注 |
|---|---|---|---|
| `state` | GET | `/api/state` | 快照；桌面端主要用事件流 |
| `traffic` | GET | `/api/traffic` | 被 `traffic` 事件取代 |
| `import` | POST | `/api/import` | `{text}` 或 `{path}` |
| `node.action` | POST | `/api/node` | `{id, action: start/stop/delete/…, route?}` |
| `node.rename` | POST | `/api/node/rename` | |
| `node.spec` | POST | `/api/node/spec` | 编辑表单数据（含凭据，仅编辑时） |
| `node.edit` | POST | `/api/node/edit` | |
| `node.share` | POST | `/api/node/share` | 含凭据，仅显式调用 |
| `node.capabilities` | POST | `/api/node/capabilities` | |
| `proxy` | POST | `/api/proxy` | `set / clear / restore / mode` |
| `routing` | POST | `/api/routing` | |
| `test` | POST | `/api/test` | |
| `stop-all` | POST | `/api/stop-all` | |
| `subscription.add/edit/delete/refresh` | POST | `/api/subscription/*` | |
| `settings` | POST | `/api/settings` | 单键保存 |
| `remote` | POST | `/api/remote` | |
| `log` | POST | `/api/log` | |
| `core.check/update` | POST | `/api/core/*` | |
| `geofiles.check/update` | POST | `/api/geofiles/*` | |
| `app-update.check/apply` | POST | `/api/app-update/*` | |
| `app-routing.config/start/stop` | POST | `/api/app-routing/*` | |
| `app-routing.conflicts` / `processes` / `app-routing.browse` / `app-routing.scan` | GET | `/api/app-routing/conflicts`、`/api/processes`、`/api/app-routing/browse?path=`、`/api/app-routing/scan?path=` | |
| `node.qr` | GET | `/api/node/qr` | 二维码图片 |
| `panel.cert-status` / `panel.trust-ca` / `panel.ca` | GET/POST | `/api/panel/*` | 本机浏览器面板证书信任；桌面端不需要，保留给过渡期旧 WebUI |
| `downloads.connector` | GET | `/api/downloads/connector` | 浏览器接力状态 |
| `downloads` / `downloads.add/action/settings/install` | GET/POST | `/api/downloads*` | |
| `shutdown` | POST | `/api/shutdown` | "退出 Mirroxy" |
| `v1.*` | GET/POST | `/api/v1/*` | AI API（status、nodes、traffic、subscriptions、nodes/{id}/start…），保持不变 |
| `v1.ping` / `v1.events` / `v1.log.subscribe` | GET/POST | 新增 | §7.2 |

Phase 1 开始时以代码为准逐一核对并补全请求/响应 schema。

### 22.2 旧文件去向

| 旧文件 | 去向 |
|---|---|
| `web/app.js`、`node-list.js`、`node-table.js`、`node-edit.js`、`download-menu.js`、`browser-relay.js` | 拆分到 `packages/ui/src/features/*`（Phase 7 删除原文件） |
| `web/dl-app.js`、`dl-model.js`、`downloads.html`、`dl-app.css` | `features/downloads` |
| `web/i18n.js` | `packages/i18n/*.json` |
| `web/theme.css`、`style.css`、`*.css` | `packages/ui/src/styles/tokens.css` + 组件 CSS Modules |
| `web/changelog*.json` | 过渡期不动；Phase 7 移到 `client/packages/ui/assets/`，同步修改 AGENTS.md 与 `render-changelog.py` |
| `web/icon.png`、`app.ico` | `apps/desktop/resources/` |
| `desktop/`（Wails） | Phase 5 后删除 |
| `tray_windows.go` | Phase 7 按 D3 处理 |
| `panel_cert*.go` | 远程 HTTPS 仍需要；本机浏览器面板证书信任逻辑在 Phase 7 评估删除 |
| `packaging/test-*.cjs` | 迁移为 Vitest 用例 |

### 22.3 文档与约定同步（Phase 7）

- `AGENTS.md`：日常路径表述改为"桌面客户端双击节点行或托盘点选"；更新日志路径；新增"Electron 不得引入原生模块、安全清单 §13.1 为必须项"。
- `README.md` / `README.en.md`：安装方式、系统要求（磁盘、内存）、平台限制。
- `docs/PROJECT_SUMMARY.md`：技术结构表增加 Electron 客户端层。

### 22.4 术语

| 术语 | 含义 |
|---|---|
| 守护进程 / 后台 | Go 编写的 `Mirroxy[.exe]` |
| UI / 客户端 | Electron 应用 |
| 主进程 / 渲染进程 / preload | Electron 进程模型中的三类代码 |
| 一步切换 | 双击节点行或托盘点选即按需启动并接管系统代理 |
| 事件流 | `/api/v1/events` SSE |
