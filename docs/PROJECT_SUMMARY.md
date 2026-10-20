# Mirroxy 项目总览

更新时间：2026-10-20  
唯一源码目录：`D:\Mirroxy`  
旧目录状态：`E:\scrcpyproxy` 已退役，默认跳过，仅保留旧程序、核心与历史数据。

## 1. 产品定位

Mirroxy 是一个 Go 编写的多节点本地代理管理器。单个后台进程提供本机 Web 面板、托盘、API、节点生命周期、流量统计、路由和在线更新能力。每个运行节点拥有固定的本地混合端口，同一端口同时接受 HTTP 与 SOCKS5；Windows 默认不修改现有系统代理，因此可与 v2rayN 并存。

当前源码版本为 **0.8.0**；最后确认的正式发布版本为 **0.7.4**。0.8.0 尚未制作正式安装包或上传 S3。

## 2. 技术结构

| 层 | 主要文件 | 职责 |
|---|---|---|
| 启动与生命周期 | `main.go`, `bootstrap.go`, `shutdown_existing.go` | 启动、本机单实例、服务端初始化、优雅退出 |
| 配置与数据 | `model.go`, `storage.go`, `storage_key_*.go`, `settings.go` | AES-GCM SQLite 状态、Windows DPAPI 密钥、设置迁移 |
| 节点与核心 | `import.go`, `core.go`, `core_assets.go`, `deploy.go`, `routing.go` | 节点导入、核心 A/B 配置、固定端口、路由与部署 |
| 网络状态 | `health.go`, `traffic.go`, `tcp_*.go`, `pac.go` | 可用性、实时/累计流量、连接数、PAC |
| Web/API | `api.go`, `ai_api.go`, `remote.go`, `web/` | 本机面板、AI API、远程认证、i18n、主题 |
| Windows 桌面 | `windows.go`, `tray_windows.go`, `app_router_windows.go` | 系统代理、托盘、UAC 应用路由助手 |
| 游戏/应用代理 | `app_router.go`, `capability.go`, `server_capability.go` | TUN、按进程路由、节点/服务器 UDP 与 IPv6 探测 |
| 更新与发布 | `self_update*.go`, `cmd/upgrade/`, `packaging/` | 可选更新、回滚、Inno Setup、四平台发布与 S3 页面 |

## 3. 已有功能

- 支持 VMess、VLESS/REALITY、Shadowsocks、Trojan、SOCKS、HTTP/HTTPS、Hysteria2、TUIC、AnyTLS。
- 节点从 5060 起获得固定本地端口；支持启动、停止、改名、分享、测试和订阅刷新。
- 切换节点一步完成：双击节点行即按需启动并接管系统代理；顶部「接管系统代理」开关可交还或重新接管。
- 全局与节点级路由：绕过大陆、黑名单、全局代理；路由模式即时生效。
- 系统代理：不接管（默认）、接管、清除三种状态，退出时恢复接管前的设置。
- 每个节点运行在独立核心进程中，起停一个节点不会重启其它节点的核心。
- 节点连接数、实时速度、累计流量和周期可用性监控。
- 简体中文/English i18n，默认跟随浏览器；亮色/暗色主题。
- 本机免登录；服务器远程面板需要账号密码；敏感设置仅允许本机修改。
- Windows Inno Setup 7 安装器支持同版本无变化、高版本原位升级和用户数据保留。
- 四平台发布目标：Windows AMD64、Linux AMD64、Linux ARM64、macOS ARM64。
- 可选在线更新，Windows 使用独立更新助手并在失败时回滚程序和数据。

## 4. 0.8.0 日常路径单步化

- 节点行双击 = 启动并接管系统代理；节点菜单提供「启动并设为系统代理」。
- 顶部由「代理方式 + 应用」表单改为「接管系统代理」开关，节点与路由模式即时生效。
- 核心进程从“按引擎共享”改为“每节点一个”，起停一个节点不再连累其它节点的连接。
- 托盘系统代理子菜单列出全部节点，点选即启动并接管；托盘路径补齐了缺失的 `App.mu` 保护。
- 面板版本号运行时注入，静态资源缓存键与运行版本一致。
- 未信任本地根证书时在页首给出明确提示。
- 可用性检测强制经过节点自身出口，修复绕过局域网时的误判。

## 4.1 游戏/应用代理

- Windows 本机网页按 `.exe` 名称或完整路径选择应用，可设置默认节点和单应用覆盖节点。
- sing-box TUN 同时声明 IPv4/IPv6，按 `process_name` / `process_path` 匹配 TCP 与 UDP。
- 每个节点使用独立、随机认证、仅监听回环的应用入口，强制走绑定节点，不继承普通端口的绕过规则。
- 主后台保持普通权限；仅 TUN 助手请求 UAC。功能默认关闭，不在重启后静默恢复。
- 远程面板不能读取本机进程、修改应用路由或借“全部停止”间接控制 TUN。
- 提供客户端完整链路检测和 `Mirroxy -probe-server` 服务器只读探针，报告 TCP、UDP、IPv6、STUN 映射及核心版本。

Windows 实机已经确认 UAC 助手与 TUN 能进入 `running` 状态，退出后虚拟网卡消失、系统代理仍为 `127.0.0.1:10808`。由于测试回合中断，尚未完成真实应用穿过节点后的出口对比、强杀核心、切网卡和断线恢复矩阵，因此当前不能宣称生产级断网保护。

## 5. 数据与安全边界

- `data/Mirroxy.db` 保存加密状态；`data/Mirroxy.key` 是配套密钥，备份和恢复必须成对处理。
- 节点密码、订阅 URL、内部应用端口与认证密钥不返回普通状态接口。
- 本地接口校验 Host、Origin、Fetch Metadata 与 UI token；远程监听始终要求认证。
- 发布白名单不包含源码、本机数据、节点、密钥、日志或发布凭据。
- 当前应用代理不是 WFP 持久 kill-switch。若管理员 TUN 助手被强杀，Windows 可能恢复普通直连；这项限制必须在 UI 和发布说明中保留。

## 6. 构建、测试与发布状态

已完成的开发验证：

- Go 全量单元测试与 `-race` 竞态检测。
- `go vet`、前端 JavaScript 语法检查。
- 使用 Xray 26.7.28 与 sing-box 1.14.0 做真实配置校验。
- Windows AMD64、Linux AMD64、Linux ARM64、macOS ARM64 交叉编译。
- 隔离 Web UI、进程枚举、应用配置隐私和远程权限边界测试。

仍需完成：

1. Windows TUN 的真实 TCP/UDP 出口对比与 DNS/IPv6 行为验证。
2. 节点退出、核心强杀、主程序强杀、网卡切换、睡眠恢复和在线更新期间的故障测试。
3. 通过后生成 0.8.0 四平台完整包和四个平台在线更新包。
4. 匿名下载复核 SHA-256 后再更新 S3 稳定入口与历史版本页面。

## 7. 目录规则

- 所有源码、文档和开发脚本只进入 `D:\Mirroxy`。
- 本地核心及路由资源位于 `D:\Mirroxy\bin`；构建缓存和隔离测试位于 `D:\Mirroxy\work`；发布产物位于 `D:\Mirroxy\dist`。这三个目录均不进入源码版本控制。
- `E:\scrcpyproxy` 不再作为任何构建或测试依赖。旧目录的 `data` 不自动删除，留作历史恢复来源。
- S3 前缀、公开 URL、GitHub 兼容名称以及跨版本迁移逻辑中的 `scrcpyproxy` 必须保留，除非另行安排破坏兼容性的迁移。
