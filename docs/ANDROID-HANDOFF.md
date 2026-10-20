# Mirroxy Android 开发交接

更新时间：2026-10-13。桌面源码版本：0.7.4。

## 用户目标与工作边界

开发能在手机上独立运行的 Mirroxy Android 版，尽可能使用 Kotlin（kt）。不是仅遥控电脑的客户端，也不是把桌面 Web 面板套进 WebView。
Android 新代码放在 `D:\Mirroxy\android`，文档留在 `D:\Mirroxy\docs`；可以参考现有桌面源码，但不要改动正在运行的桌面程序、真实节点、端口、下载任务、系统代理或私有 data。
用户此前明确确认当前项目在 D 盘；现有 README/AGENTS.md 中的 E 盘工作区文字与实际环境有历史不一致，禁止因此迁移目录或读取退役的 scrcpyproxy 本地运行目录。公开 scrcpyproxy URL 和私有仓库名是兼容标识，不随意改名。
不读取、复制或打包真实订阅、节点密码、数据库密钥；跨设备配置迁移必须通过用户主动导出/导入，不直接复制 Windows DPAPI 密钥。

## 当前桌面版

- Go 1.26 单体后台 + 内嵌 HTML/CSS/JavaScript 面板；Windows 托盘，Linux systemd，macOS LaunchAgent。
- 每个节点固定回环端口，同时支持 HTTP/SOCKS5；可同时运行多个节点。支持分享链接、订阅、v2rayN 导入、改名、测速、健康检测、路由与流量统计。
- 网络核心 A/B 分别是独立的 Xray 与 sing-box。协议包括 VMess、VLESS/REALITY、Shadowsocks、Trojan、HTTP/SOCKS、Hysteria2、TUIC、AnyTLS；各协议实际支持取决于所选核心与配置。不能把 Windows/Linux 核心二进制直接当 Android 核心。
- 默认不改变系统代理；按节点配置路由，故障不会静默换线路或回退直连。Windows 应用代理基于单独的 TUN 助手和管理员授权。
- 本机 HTTP API `/api/v1/`，远程管理默认关闭，开启必须认证。下载管理目前仅本机面板开放。
- SQLite 状态按域 AES-GCM 加密；Windows 主密钥由 DPAPI 保护。节点自定义名称、固定端口、累计流量必须持久保存。
- aria2 1.37.0 下载管理：无分类侧栏，最多同时 30 个任务，每任务最多 16 个连接。用户明确暂不做 32 连接。支持显式代理、断点续传、限速、多选、编辑、重连、右键菜单、打开文件/文件夹。
- 删除使用页面内确认窗；删除文件默认不勾选，勾选时移入可恢复回收区。桌面文件打开能力在 Android 上应改为系统文件 Intent/FileProvider/存储访问框架，不自动运行下载文件。
- 已有四个平台完整包与轻量更新包；固定 Windows 在线安装器已生成，S3 上传仍待用户给出凭据文件路径。Android 开发任务不得自行发布 S3/GitHub、公开源码、生成或泄露正式签名密钥。
- Chrome 接管下载插件仅讨论过可行性，尚未实现，不能作为现成依赖。

## 值得参考的源码

`model.go`、`import.go`、`share.go`、`rename.go`、`node_order.go`：模型、协议导入、分享、改名与排序。
`core.go`、`core_assets.go`、`deploy.go`、`routing.go`、`geofiles.go`：核心配置与生命周期、路由资产。
`traffic.go`、`health.go`、`server_capability.go`：统计与检测。
`storage.go`、`storage_key_windows.go`、`storage_key_unix.go`：存储安全边界，不直接移植平台密钥实现。
`ai_api.go`、`api.go`、`remote.go`：接口与认证。
`downloads.go`、`downloads_open.go`、`downloads_trash.go`、`web/downloads.js`：下载行为与安全约束。
`web/changelog.json`：实际功能版本；README 中仍有“同时 3 个下载”的旧说明，应以 0.7.4 实现为准。

## Android 实现方向

1. 原生 Kotlin + Jetpack Compose，业务/存储/后台服务尽可能 Kotlin；协程与 Flow 管理状态，Gradle Kotlin DSL。沿用 Mirroxy 视觉风格，但适配手机，不复制桌面宽表格。
2. 先验证 Android 可用的官方代理核心库及许可证，再选 JNI/AAR/少量 Go 绑定；不要为追求纯 Kotlin 重写成熟的加密代理协议，也不要使用来源不明的二进制。
3. 首版先打通节点导入、编辑、启停、持久化和真实代理请求，再完成 VpnService 前台服务、全局/分应用路由、通知及断网/退出恢复。VPN 需用户授权；保护核心出站 socket，防止流量循环。一个用户/工作资料通常只有一个活动 VPN，不宣称可同时叠加其他 VPN 应用。
4. 默认不自动启动全设备 VPN；本地 HTTP/SOCKS 代理和 VPN 接管是不同模式。手机关闭界面不等于停止前台代理服务。
5. Room 等本地存储结合 Android Keystore 加密敏感字段；名称、统计、配置持续保留。勿把平台密钥或节点凭据写入日志。
6. 下载管理按后续阶段实现：先确认 aria2 Android 适配可行性，或采用 Kotlin 分段下载引擎。必须处理 Range、断点一致性、重连、校验、代理、后台限制与 Android 文件访问，不把 30×16 作为默认满负载。
7. APK 更新需遵循 Android 安装授权与签名机制，不能复用 Windows 在线安装器。首阶段产出可编译、可安装的 debug APK 和中文构建/测试说明；发布签名由用户掌控。

## 已发现的本机工具

Android SDK：`D:\DevTools\Android\Sdk`（已有 platforms/build-tools/platform-tools/cmdline-tools；具体版本及模拟器仍需核实）。
JDK：`D:\DevTools\jdk-21`。新任务应先核实 AGP/Gradle/Kotlin 的官方兼容组合，不凭记忆固定版本。
依赖工具下载到 `D:\DevTools`；若下载失败，使用用户提供的 HTTP/HTTPS 代理 `http://127.0.0.1:10808`，必要时 ALL_PROXY 为 `socks5://127.0.0.1:10808`；不修改系统代理，不输出代理或存储密钥。

## 验证与交付

必须持续实现，不只给计划。所有功能测试用隔离配置与模拟上游，不使用真实桌面节点作为测试数据。
区分“界面完成”“真实核心已联网”“VPN 真正路由成功”“已在设备/模拟器验证”，不要以假数据界面冒充代理可用。
做模拟器/adb 功能或性能验证时，读取并遵循相应 Android 测试 skill。没有设备则保留 APK 和可重复测试说明，明确实机验证缺口。
Android 新功能记录在 Android 自身变更记录，并同步根 `web/changelog.json` 的项目变更记录；不要为了 Android 开发随意升级正在运行的桌面程序。

## 2026-10-13 开发进展

已建立 `D:\Mirroxy\android` Kotlin/Compose 原生工程并产出可安装 `0.1.0-dev` debug APK，真实官方核心为 XTLS libXray 26.10.10（MIT 包装层 + MPL-2.0 Xray）。节点导入/编辑/改名/删除、Keystore 加密存储、固定 HTTP/SOCKS 端口与前台本地代理已实现。realme arm64 平板和 MuMu x86_64 模拟器各通过 4 项隔离测试；无线目标的 Redmi 型号标识实际属于 MuMu，不是 Redmi 真机。

VpnService/TUN/socket protect/分应用列表已接入；可选测试等待 120 秒未获得用户系统授权，未启动 VPN，路由结果仍待验证。订阅 URL 管理、流量统计、下载管理等后续阶段未完成。继续开发前读取 `ANDROID-BUILD-TEST.md` 与 `ANDROID-QA.md`；禁止把本地隔离测试当作真实订阅节点或完整 VPN 的通过证据。

原始官方参考：

- Compose：https://developer.android.com/develop/ui/compose/documentation
- VPN：https://developer.android.com/develop/connectivity/vpn
- Keystore：https://developer.android.com/privacy-and-security/keystore
