# Mirroxy Flutter 客户端开发计划

> 状态：规划稿（2026-11-02）。本文替代"继续扩展 WebUI"的路线，目标是用一套 Flutter 客户端覆盖 Windows / macOS / Linux 桌面与 Android，Go 后端保留为代理引擎。

## 1. 目标与边界

**目标**

- 放弃浏览器面板（`web/`）与 Wails 下载窗口（`desktop/`），统一为 Flutter 原生客户端。
- 解决 WebUI 的结构性限制：原生窗口与托盘、多窗口、文件拖放与对话框、全局快捷键、系统通知、无需面板端口与证书。
- 桌面四平台（Windows x86_64、macOS ARM64、Linux AMD64、Linux ARM64）与 Android 共用一套 UI 代码与设计系统。

**不变的约束（来自 AGENTS.md）**

- 每个节点独立核心进程；起停一个节点不重启、不中断其它节点。
- 一步切换：双击节点行 / 托盘点选即按需启动并接管系统代理；顶部开关与路由模式即时生效，无"应用"按钮。
- 默认"不改变系统代理"；自定义节点名称在订阅刷新后保留，改名不重启核心。
- 每次发布生成四平台 `Mirroxy-update-*.zip`，保留核心与用户数据；Linux 原 PID exec、Windows 独立助手。
- 发布物不含 data、节点、订阅、密钥、日志、凭据；没有实机验证的平台不得宣称通过。
- 每项改动记录到 `web/changelog.json`（迁移后改到新位置，见 §8）。

**不做**

- 不重写 Go 后端的代理、订阅、路由、流量、更新逻辑。
- 不把 Go 编译成 Flutter FFI 库（桌面端）——见 §2.2。

## 2. 总体架构

### 2.1 进程模型

```
┌──────────────────────────┐        本地 IPC（HTTP/JSON + 事件流）       ┌───────────────────────────┐
│ Flutter 客户端 (mirroxy-ui)│ ─────────────────────────────────────────▶ │ Go 守护进程 (mirroxy)       │
│  窗口 / 托盘 / 多窗口 / 通知 │ ◀───────────────── 事件推送 ─────────────── │  节点、订阅、系统代理、路由    │
└──────────────────────────┘                                            │  流量、下载、更新、远程        │
                                                                         └─────────┬─────────────────┘
        浏览器扩展 ────────── 现有 HTTP 接口（不变）──────────────────────────────▶ │
                                                                                   ▼
                                                                    每节点独立 xray / sing-box 核心进程
```

- **Go 守护进程**是唯一的状态拥有者。关闭 UI 不影响代理运行；Linux 无界面服务器只运行守护进程（systemd 监管不变）。
- **Flutter 客户端**是纯前端：不持久化业务数据，不直接操作核心进程，只调 API、订阅事件。
- **Android** 例外：后端仍是现有 Kotlin + libXray（`android/app/src/main/java/com/mirroxy/android/`），Flutter 通过 Pigeon 平台通道调用，见 §6。

### 2.2 为什么不用 FFI 把 Go 嵌进 Flutter

- 代理必须在 UI 关闭后继续运行，且 Linux ARM64 服务器场景没有图形界面。
- 在线更新依赖"Linux 原 PID exec / Windows 独立助手"，把后端嵌进 UI 进程会让 UI 与引擎必须同时重启。
- 现有 HTTP API、浏览器扩展、远程管理都已基于守护进程，拆开改动最小。

### 2.3 通信协议

| 项 | 方案 |
|---|---|
| 传输 | 桌面优先 Unix domain socket（macOS/Linux）与 Windows 命名管道；保留 `127.0.0.1` 回环 TCP 作为兼容（浏览器扩展、旧工具） |
| 请求 | 沿用 JSON over HTTP，统一收敛到 `/api/v1/*`（现有 `/api/*` 动作逐步别名到 v1） |
| 推送 | 新增 `/api/v1/events`（SSE），推送节点状态、流量（1s）、日志、下载进度、更新进度；替代 `web/app.js` 中 5s 轮询 `/api/state` 与 1s 轮询流量 |
| 鉴权 | 守护进程启动时在 data 目录写入会话令牌文件（仅当前用户可读：Windows ACL / Unix 0600），UI 读取后放入 `Authorization` 头；继续使用 `X-App-Token` 兼容 |
| 契约 | 编写 OpenAPI 3.1 规格（`api/openapi.yaml`），生成 Dart 模型（`freezed` + `json_serializable`）；Go 端加契约测试，保证字段名不漂移 |
| 远程 | 客户端可连接远程守护进程（TLS + 令牌，复用 `panel_cert.go` 与 `remote.go`），替代"在服务器上开 WebUI"的用法 |

## 3. 技术选型

| 领域 | 选型 | 说明 |
|---|---|---|
| Flutter | 最新 stable（锁定到具体版本，写入 `.fvmrc`） | 用 FVM 固定版本，CI 与本地一致 |
| 状态管理 | Riverpod 2（`riverpod_generator`） | 事件流天然映射为 `StreamProvider` |
| 路由 | `go_router` | 桌面侧边栏 + 移动底栏共用路由表 |
| 网络 | `dio` + 自定义 `HttpClientAdapter`（UDS / 命名管道） | SSE 用 `dio` 流式响应自行解析 |
| 窗口 | `window_manager` | 无边框、记住尺寸位置、最小化到托盘 |
| 托盘 | `tray_manager` | 三平台托盘 + 菜单；托盘直接列出节点供点选 |
| 多窗口 | `desktop_multi_window`（Spike 验证；不成熟则下载管理改为同窗口路由 + 可弹出） | 下载管理独立窗口 |
| 视觉 | Material 3 + 自有设计令牌；Windows 用 `flutter_acrylic` 提供 Mica/Acrylic，macOS 用 vibrancy | 不引入 Fluent/Cupertino 双套组件，保持一套 UI |
| 表格 | `two_dimensional_scrollables` 的 `TableView` 或自研虚拟化行列表 | 节点表需千行级流畅滚动、列宽拖拽、多选 |
| 图表 | `fl_chart` | 流量曲线 |
| 二维码 | `qr_flutter`（生成）、`mobile_scanner`（Android 扫码）、屏幕截图识别（桌面，Spike） | 分享与导入 |
| 拖放 | `desktop_drop` | 拖入订阅文件 / 配置 |
| 快捷键 | `hotkey_manager` | 全局切换节点 / 开关系统代理 |
| 通知 | `local_notifier`（桌面）、`flutter_local_notifications`（Android） | 下载完成、更新可用、节点失效 |
| 自启动 | `launch_at_startup` | 与守护进程自启分开设置 |
| 国际化 | Flutter `gen-l10n` + ARB | 从 `web/i18n.js` 与 `changelog.en.json` 迁移词条 |
| 平台通道 | Pigeon | Android 侧调用 Kotlin `Core` / `Services` / `NodeStore` |
| 测试 | `flutter_test`、`integration_test`、golden 测试（`alchemist`） | 关键页面做截图回归 |

## 4. 目录结构

```
D:\Mirroxy
├─ *.go                     Go 守护进程（现状保留）
├─ api/openapi.yaml         接口契约（新增）
├─ client/                  Flutter 工程（新增）
│  ├─ lib/
│  │  ├─ app/               入口、路由、主题、窗口与托盘
│  │  ├─ core/              API 客户端、事件流、传输适配器、令牌
│  │  ├─ data/              生成的模型与仓储（repositories）
│  │  ├─ features/          nodes / subscriptions / routing / app_routing /
│  │  │                     traffic / downloads / settings / updates / logs / remote
│  │  ├─ platform/          desktop/ 与 android/ 平台差异实现
│  │  └─ l10n/              ARB 文件
│  ├─ windows/ macos/ linux/ android/   Flutter 平台壳
│  └─ test/ integration_test/
├─ android/                 现有 Kotlin 工程（Phase 5 合入 client/android 后归档）
├─ web/  desktop/           迁移完成后删除（Phase 7）
└─ packaging/               发布脚本（Phase 6 改造）
```

## 5. 功能迁移清单

以现有 `web/` 页面与 `/api/*` 动作为准，逐项迁移。每项须达到与 WebUI 同等行为后才勾掉。

| 模块 | 现有来源 | Flutter 要点 |
|---|---|---|
| 节点列表 | `node-list.js`、`node-table.js` | 虚拟化表格、排序（`node_order.go`）、多选、右键菜单、**双击即启动并接管系统代理** |
| 节点编辑 / 改名 / 分享 | `node-edit.js`、`/api/node/edit`、`rename`、`share` | 改名不重启核心；分享二维码与链接；凭据只在显式导出时出现 |
| 导入 | `/api/import`、`nodes/import` | 剪贴板、文件拖放、二维码；导入预览 |
| 订阅 | `subscription/add/edit/delete/refresh` | 刷新后保留自定义名称；定时刷新状态 |
| 顶栏开关 | `system-proxy`、`routing`、`mode` | "接管系统代理"开关、路由模式即时生效，无"应用"按钮 |
| 联网检测 / 健康 | `/api/test`、`health.go` | 不可用 vs 超时区分（参考 91c3caf） |
| 流量 | `traffic.go`、`/api/v1/traffic` | 事件流推送；累计流量与实时曲线 |
| 应用分流 | `app-routing.css`、`app_router*.go`、`app_scan.go` | 进程列表 + 图标；仅在支持的平台显示，保留平台限制说明 |
| 下载管理 | `downloads.html`、`dl-*.js`、`downloads*.go` | 独立窗口（IDM 式）、回收站、浏览器接力设置；任务文件缺失自动清理 |
| 全局设置 | `settings.go`、`/api/settings` | 逐项即时保存（参考 805c2e2） |
| 核心 / Geo / 程序更新 | `core/*`、`geofiles/*`、`app-update/*` | 进度推送；程序更新提示"短暂重启" |
| 远程管理 | `remote.go`、`server_capability.go` | 连接远程守护进程、能力探测 |
| 日志 | `/api/log` | 实时尾随、等级过滤、复制 |
| 加速器避让 | `accelerator_guard*.go` | 状态提示与开关 |
| 托盘 | `tray_windows.go` | 三平台托盘；节点点选；开关系统代理；打开下载管理 |
| 更新日志 | `web/changelog*.json` | 关于页内渲染 |

## 6. Android 合流策略

现有 Android 是 Kotlin + Compose + libXray（单运行节点、HTTP/SOCKS 回环、导入/编辑/改名/删除、Keystore 加密、前台服务、VPN 与应用白名单）。

1. 在 `client/android` 内保留 Kotlin 业务层：把 `Core.kt`、`Importer.kt`、`NodeStore.kt`、`Services.kt` 原样迁入，删除 Compose UI。
2. 用 Pigeon 定义 `MirroxyHostApi`（节点 CRUD、启停、导入、VPN 授权、应用列表）与 `MirroxyFlutterApi`（状态、流量回调）。
3. Flutter 侧抽象 `MirroxyBackend` 接口：桌面实现走 HTTP/IPC，Android 实现走 Pigeon。UI 层只依赖接口。
4. 自适应布局：宽屏用侧边栏 + 表格，窄屏用底栏 + 卡片列表；同一组 feature 组件。
5. Android 待办（订阅 URL、流量统计、多节点并行、下载管理）在 Flutter 版中按桌面功能顺序补齐。
6. 验证沿用 `docs/ANDROID-BUILD-TEST.md` 与 `ANDROID-QA.md`；VPN 系统授权后路由测试须实机完成才可宣称通过。

## 7. 分阶段计划

工期按 1 名主力开发 + AI 辅助估算，单位为周。每阶段有明确出口条件，未达成不进入下一阶段。

### Phase 0 · 技术验证（1–2 周）

- 四平台各跑通：窗口 + 托盘 + 通过 UDS/命名管道调用守护进程 `/api/state` + 订阅 SSE。
- 在 GitHub Actions 上产出 Windows x64、macOS arm64（`macos-14`）、Linux x64、Linux arm64（`ubuntu-24.04-arm`）构建。Flutter 不支持从 x64 交叉编译 Linux arm64，必须用 arm64 runner。
- 验证 `desktop_multi_window` 稳定性、Windows Mica、Linux 中文输入法（fcitx/ibus）、1000 行节点表滚动帧率。
- 测量常驻内存与包体（预期 UI 进程 60–120 MB，包体 +25–40 MB）。

**出口**：Spike 报告写入 `docs/`，确认多窗口方案与传输层方案；任一平台不可行则在此回退决策。

### Phase 1 · 后端拆分与 API 契约（2 周）

- 增加守护模式：`mirroxy --daemon` 不启动 WebUI、不打开浏览器；现有托盘逻辑在 UI 接管后改为可选。
- 实现 UDS / 命名管道监听与令牌文件（含权限测试）。
- 收敛 `/api/v1`，新增 `/api/v1/events`（SSE），补齐所有 WebUI 用到的动作。
- 编写 `api/openapi.yaml`，Go 契约测试覆盖全部端点；保证列表/状态响应不含凭据（沿用 `api_privacy_test.go`）。

**出口**：WebUI 仍可正常使用（不破坏现状）；`go test ./...` 全绿；OpenAPI 生成的 Dart 客户端能跑通读写冒烟测试。

### Phase 2 · Flutter 工程骨架与设计系统（1–2 周）

- 建立 `client/`：FVM、Riverpod、go_router、dio 传输适配器、事件流、错误与重连（守护进程未启动时自动拉起或提示）。
- 设计系统：颜色 / 字体 / 间距 / 圆角令牌，亮暗主题，与 `docs/brand` 对齐；组件库（按钮、开关、表格、对话框、Toast、空状态）。
- 窗口与托盘外壳、侧边导航、i18n 骨架（中 / 英）、golden 测试基线。

**出口**：空壳客户端在四平台可安装运行，主题切换、托盘、窗口记忆可用。

### Phase 3 · 核心日常路径（3–4 周）

- 节点列表、顶栏开关、路由模式、双击切换、托盘点选切换。
- 订阅管理、导入、节点编辑 / 改名 / 分享、联网检测、实时流量。
- 集成测试：使用隔离测试目录启动守护进程，验证"一步切换"、改名不重启核心、起停一个节点不影响其它节点、订阅刷新保留名称。

**出口**：日常使用可完全不打开 WebUI；内部 alpha 版本（Windows 优先，因为主要用户在 Windows + v2rayN 共存场景）。

### Phase 4 · 进阶功能（2–3 周）

- 全局设置（逐项即时保存）、核心 / Geo / 程序更新、日志、远程管理、加速器避让、健康监测。
- 应用分流（含平台限制说明）。
- 下载管理独立窗口，替代 `desktop/` 的 Wails 客户端；浏览器扩展接力不变。

**出口**：§5 清单全部勾完；WebUI 功能无遗漏（逐页对照）。

### Phase 5 · Android 合流（3–4 周，可与 Phase 4 后半并行）

- 按 §6 迁移 Kotlin 业务层与 Pigeon 接口；自适应布局。
- 实机验证 VPN 授权、应用白名单、前台服务保活。

**出口**：Android 版功能不少于现 0.1.0-dev；实机 QA 记录更新到 `docs/ANDROID-QA.md`。

### Phase 6 · 打包、发布与在线更新（2 周）

- Flutter 桌面产物是目录（可执行文件 + 动态库 + `data/`），不是单文件：
  - 更新包 `Mirroxy-update-*.zip` 同时包含守护进程与 `client/` 目录；Windows 更新助手（`cmd/upgrade`）改为整目录替换；Linux 守护进程继续原 PID exec，UI 进程由守护进程通知后自行重启。
  - 保留核心与用户数据的规则不变；增加"旧版 → 新版"数据迁移测试。
- Windows 安装器（现有 Inno Setup）加入 UI 目录、开始菜单快捷方式；macOS `.app` 签名与公证；Linux 提供 tar.gz + `.desktop` + systemd 用户服务（仅守护进程）。
- `packaging/build-release.py` 与 `publish-release.py` 接入 Flutter 构建；SHA256SUMS 覆盖所有新文件；同一版本不得替换字节。

**出口**：四平台发布物在 CI 生成；至少 Windows 完成"本机备份 → 在线更新 → 节点、端口、累计流量保留"实机验证；其它平台按实际验证情况标注。

### Phase 7 · Beta 并行与切换（1–2 周）

- 发布一个同时带 WebUI 与 Flutter 客户端的过渡版本，默认打开 Flutter 客户端，WebUI 可在设置中临时启用。
- 收集问题、修复回归后，下一版本删除 `web/`、`desktop/` 与面板端口相关代码（保留浏览器扩展所需接口）。
- 更新 `AGENTS.md`、`README*.md`、`CHANGELOG.md`、`docs/PROJECT_SUMMARY.md`。

**出口**：WebUI 删除，CI 全绿，发布说明标注各平台实机验证状态。

### 工期汇总

| 阶段 | 周 |
|---|---|
| 0 技术验证 | 1–2 |
| 1 后端拆分与契约 | 2 |
| 2 骨架与设计系统 | 1–2 |
| 3 核心日常路径 | 3–4 |
| 4 进阶功能 | 2–3 |
| 5 Android 合流（部分并行） | 3–4 |
| 6 打包发布 | 2 |
| 7 Beta 与切换 | 1–2 |
| **合计** | **约 15–21 周** |

## 8. 工程规范

- **Git**：每个阶段在独立分支开发，按功能小步提交；不留未提交源码。
- **更新日志**：迁移期间仍写 `web/changelog.json`；Phase 7 移到 `client/assets/changelog.json` 并同步修改 AGENTS.md。
- **CI**：`go test ./...`、`flutter analyze`、`flutter test`、golden 测试、四平台构建；PR 必须全绿。
- **测试隔离**：所有自动化测试使用隔离 data 目录与随机端口，不读写真实节点、订阅和系统代理。
- **安全**：令牌文件权限测试；UI 日志不打印凭据；远程连接强制 TLS。
- **性能预算**：冷启动 < 1.5 s，节点表 1000 行滚动 60 fps，UI 进程隐藏到托盘后常驻内存 < 120 MB。

## 9. 风险与对策

| 风险 | 影响 | 对策 |
|---|---|---|
| 多窗口插件不成熟 | 下载管理无法独立窗口 | Phase 0 验证；不行则同窗口路由 + 可弹出的子窗口进程 |
| Linux ARM64 构建 | 只能在 arm64 机器构建 | 使用 GitHub `ubuntu-24.04-arm` runner；无实机则不宣称实机通过 |
| Linux 输入法 / 字体渲染 | 中文输入体验差 | Phase 0 在 fcitx5 / ibus 下测试；内置 Noto Sans CJK 回退字体 |
| 内存占用高于 WebUI | 常驻托盘时资源占用 | 隐藏窗口时暂停动画与图表；守护进程可在 UI 退出后独立运行 |
| 产物从单文件变为目录 | 更新与安装脚本需改造 | Phase 6 专项；升级助手整目录原子替换并保留回滚 |
| macOS 签名公证 | 需 Apple 开发者账号 | 提前申请；未签名版本在发布说明中标注 |
| 迁移期双 UI 维护 | 工作量翻倍 | Phase 1 起冻结 WebUI 新功能，只修严重缺陷 |

## 10. 下一步

1. 确认本计划后，建分支 `feat/flutter-spike` 开始 Phase 0。
2. Phase 0 首周交付：四平台 CI 构建 + Windows 上托盘与 `/api/state` 调用演示。
