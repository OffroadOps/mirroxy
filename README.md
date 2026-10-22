<div align="center">

# Mirroxy

**多节点本地代理管理器 — 源码开放**

[English](README.en.md) · [简体中文](README.md)

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26%2B-blue.svg)](go.mod)
[![Platforms](https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey.svg)](#下载)

</div>

Mirroxy 是一个用 Go 编写的多节点本地代理管理器。单个后台进程提供本机 Web 面板、Windows
托盘、AI 可调用的 HTTP API、节点生命周期、流量统计、分流路由和在线更新能力。

每个运行中的节点拥有**固定的本地混合端口**，同一端口同时接受 HTTP 与 SOCKS5；节点可以
**同时运行多个**，互不抢占端口与线路。所有节点跑在**各自独立的核心进程**里，起停一个节点
不会打断其它节点正在进行的连接。

> 切换节点只需一步：**双击节点行**，程序会按需启动该节点并把系统代理接管过去。

## 主要特性

- **一步切换节点** — 双击节点行即启动并接管系统代理；顶部「接管系统代理」开关、节点下拉框
  与路由模式全部即时生效，没有需要额外点击的「应用」。
- **节点独立核心进程** — 启动/停止一个节点只重启它自己的核心，其它节点的连接不受影响。
- **多节点并行** — 节点从 5060 起分配固定端口，可同时运行、分别统计连接数与流量。
- **多协议** — VMess、VLESS/REALITY、Shadowsocks、Trojan、SOCKS、HTTP/HTTPS、Hysteria2、
  TUIC、AnyTLS；VMess/VLESS/Trojan 等由网络核心 A 承载，Hysteria2/TUIC/AnyTLS 由网络核心 B 承载。
- **分流路由** — 全局与节点级路由：绕过大陆、黑名单、全局；全局路由模式对局域网与本机地址
  保持直连（可关闭）。
- **Windows 游戏 / 应用代理** — 按 `.exe` 或完整路径选择程序，选中程序的 TCP/UDP 走绑定节点，
  未选程序保持原网络方式；只 TUN 不改系统路由。
- **集成下载管理** — 面板内 aria2 下载页：新建、暂停/继续、限速、批量操作、断点重连，
  每个任务可指定直连、已启动的 Mirroxy 节点或自定义 HTTP 代理。
- **隐私优先** — 节点、订阅、远程凭据与流量统计写入 AES-GCM 加密的 SQLite；Windows 主密钥由
  DPAPI 保护。普通状态接口不返回节点密码、出站配置、订阅 URL 或 UI token。
- **远程管理** — 可选的远程面板与 API，必须账号密码；敏感设置仅允许本机修改。
- **中英双语 + 明暗主题** — 界面语言默认跟随浏览器。

## 下载

二进制发布在 [GitHub Releases](https://github.com/OffroadOps/mirroxy/releases)（四个平台：
Windows x86_64、Linux ARM64、Linux AMD64、macOS ARM64），每次发布附带 `SHA256SUMS`。

| 平台 | 文件 | 说明 |
|---|---|---|
| Windows x86_64 | `Mirroxy-windows-amd64.zip` | 完整程序包；也提供 `.exe` 安装程序 |
| Linux ARM64 | `Mirroxy-linux-arm64.run` | 离线安装包（`sudo bash ./Mirroxy-linux-arm64.run`） |
| Linux AMD64 | `Mirroxy-linux-amd64.run` | 离线安装包 |
| macOS ARM64 | `Mirroxy-macos-arm64.zip` | 解压后 `bash ./install-macos.sh` |

Linux 也可以用一键脚本（systemd 环境，自动识别 ARM64 / AMD64）：

```bash
curl -fsSL --retry 3 https://raw.githubusercontent.com/OffroadOps/mirroxy/master/install.sh | sudo bash
```

> 历史版本与旧入口仍保留在 [S3 发布页](https://ny-1s.enzonix.com/bucket-1776-2431/scrcpyproxy/index.html)，
> 新版本以 GitHub Releases 为准。

## 使用方式

Windows 解压或安装后启动 Mirroxy，打开 <https://127.0.0.1:9527/>（安装时可选端口）。导入分享
链接、订阅或 v2rayN SQLite 节点库，再启动所需节点。首次从 5060 起分配空闲 TCP/UDP 端口，
随后保持绑定。下载器填写 `127.0.0.1` 和节点端口即可。

切换到某个节点只需一步：**双击节点行**，程序会按需启动该节点并把系统代理接管过去；顶部的
「接管系统代理」开关可以随时交还或重新接管，节点下拉框和路由模式都即时生效。

Windows 安装向导会检测面板端口占用。端口被占用时，请关闭占用程序后重试，或在向导中选择其他
端口；升级默认保留已有面板端口。静默安装可以通过 `/PANELPORT=9528` 指定端口。

默认代理方式是「不改变系统代理」，不会改写 v2rayN 当前使用的 `127.0.0.1:10808`。需要时可以在
顶部切换为接管系统代理或清除系统代理，并在退出时恢复接管前的设置。

托盘双击打开面板，右键退出后台。关闭网页不会停止代理。节点列表显示连接数、实时上传下载、
累计流量和可用性；可改名、分享、测试、设为系统代理或单独指定路由。

### 系统代理与路由

顶部的「接管系统代理」开关是日常入口：打开后系统代理指向所选节点的本地端口，关闭后把接管前
的设置原样交还。节点下拉框列出全部节点，未启动的节点标为「未启动」，选中时会先启动再接管。
另有「清除」用于强制关闭系统代理。默认不接管，因此不会改写其它软件当前使用的代理设置。

| 操作 | 行为 |
|---|---|
| 不接管 | 默认；不写 Windows 代理设置。 |
| 接管系统代理 | 使用所选节点的 HTTP 端口；节点未运行时先启动。 |
| 清除 | 关闭当前静态代理和 PAC。 |

全局路由默认为绕过大陆，也可选择黑名单或全局。每个节点可跟随全局或独立覆盖。节点故障不会
切换到另一节点；固定端口保持失败，以避免下载或应用在不知情时换线路。

每个节点运行在独立的核心进程中。启动或停止一个节点只重启它自己的核心，其它节点正在进行的
连接不会中断；只有切换全局路由模式会重建全部节点的核心并短暂重连。

### 下载管理（aria2）

面板内置 IDM 风格下载页：顶部工具栏与任务表格，不设分类侧栏。支持新建、搜索/筛选、暂停、
继续、编辑链接/代理/连接数/限速、重试、单选/多选操作。默认删除只移除任务记录，保留文件及
`.aria2` 断点数据；勾选「同时删除下载文件和断点数据」时移入保存目录的 `.Mirroxy-trash` 回收区。

每个任务明确选择直连、已启动的 Mirroxy 节点或自定义 HTTP 代理（可带认证）。直连不继承环境或
Windows 系统代理；节点模式遵循该节点的出口策略；节点未启动时报错，不自动回退直连。aria2
不直接支持 SOCKS5，Mirroxy 的同一端口可通过 HTTP 代理使用。

Windows 完整包预置官方 aria2 1.37.0 独立进程及许可证；更新包保留既有核心。Linux/macOS 使用
系统安装的 `aria2c`（例如 Debian/Ubuntu `sudo apt install aria2`、macOS `brew install aria2`）；
未宣称其他平台实机验证通过。

### Windows 游戏 / 应用代理

可以从当前运行进程选择完整程序路径，也可以手动加入多个 `.exe`；每个配置可跟随默认应用节点
或单独覆盖节点。选中程序的 TCP/UDP 全部走绑定节点，未选程序保持当前网络方式，本机与局域网
地址直连。

应用代理使用单独的 sing-box TUN。节点核心提供只监听 `127.0.0.1` 的随机认证内部入口，强制进入
绑定节点，不继承普通 506x 端口的绕过大陆或黑名单规则。功能默认关闭；开启时请求一次 Windows
管理员授权，停止或退出 Mirroxy 时撤销 TUN。当前没有宣称 WFP 持久断网保护：强杀整个管理员
路由助手后 Windows 可能恢复普通直连。

页面还可真实穿过默认应用节点检测 TCP、UDP、IPv6 和 UDP 映射特征。服务器主机可运行
`Mirroxy -probe-server` 输出只读 JSON 报告。

## 目录与本地数据

```text
Mirroxy.exe
upgrade.exe
bin/
  runtime/
    Mirroxy_core_A.exe
    Mirroxy_core_B.exe
  resources/
    Mirroxy_route_ip.bin
    Mirroxy_route_domain.bin
    Mirroxy_set_01.bin ... Mirroxy_set_12.bin
    Mirroxy_trust.pem
data/
  Mirroxy.db
  Mirroxy.key
  Mirroxy.log
  Mirroxy_upgrade.log
  runtime/
    Mirroxy_worker_*.log
```

设置、节点出站、订阅、远程凭据、流量统计、更新状态和系统代理恢复信息分别序列化后，以
AES-GCM 加密存入 SQLite `data/Mirroxy.db`。Windows 的 256 位主密钥再用当前用户 DPAPI 保护后
保存为 `data/Mirroxy.key`。Linux/macOS 密钥文件权限为 0600。备份时数据库和密钥必须一起保存。

核心启动配置写入 `data/runtime/Mirroxy_session_*.cfg.pending`，校验后启动核心，端口就绪即
删除；失败路径也清理。运行日志使用不可识别核心种类的通用名称。

首次启动 0.5.0 会读取旧版 `settings.json` 和 `traffic.json`，成功写入加密数据库后删除旧版
明文文件及旧版生成配置。迁移失败时保留原文件并停止启动，避免静默丢失数据。

## API 与远程访问

本机 API 位于 `http://127.0.0.1:15060/api/v1/`，回环访问免认证。远程监听默认关闭；开启后网页
与 API 都要求账号密码，包括从本机反向代理到远程端口的请求。

- `GET /api/v1/guide`
- `GET /api/v1/openapi.json`
- `GET /api/v1/nodes`
- `GET /api/v1/traffic`
- `GET /api/v1/status`
- `POST /api/v1/nodes/{id}/start|stop|test|share|rename`
- `POST /api/v1/nodes/import`
- `POST /api/v1/routing`
- `POST /api/v1/updates/check`
- `POST /api/v1/updates/apply`

普通状态接口不返回节点密码、完整出站配置、订阅 URL 或 UI token。分享接口会返回完整分享链接，
远程调用仍受认证保护。

## 从源码构建

需要 Go 1.26+。

```bash
go mod download
go test ./...
go build -trimpath -ldflags '-s -w -H=windowsgui' -o Mirroxy.exe .
go build -trimpath -ldflags '-s -w' -o upgrade.exe ./cmd/upgrade
```

Windows 图形界面需要 `-H=windowsgui`；`cmd/upgrade` 是独立的更新助手。

### 跨平台构建与打包

```bash
python packaging/build-release.py --go <go>
```

生成 Windows x86_64、Linux ARM64、Linux AMD64、macOS ARM64 四个平台完整包与四个平台
`Mirroxy-update-*.zip`。Windows 完整载荷是 `Mirroxy-windows-amd64.zip`。默认不编译 Windows
安装程序，只生成四平台程序包；需要离线向导时加 `--offline-windows --iscc <路径>`。

固定 `Mirroxy-online-setup.exe` 每次从 HTTPS `releases.json` 读取最新版并下载完整 ZIP，验证
来源、大小、SHA256、内部文件清单和 AMD64 架构后安装；不会关闭 TLS 校验或打包源码。可填写
下载代理，留空直连，不修改系统代理。`/DOWNLOADPROXY=http://127.0.0.1:10808` 可用于静默安装。

同版本不修改文件，拒绝降级；更高版本使用同一 AppId 原位升级；安装、升级和卸载均保留安装
目录内的 `data`。发布白名单不包含本机 data、节点、密钥、日志或存储凭据。在线升级保留已有
核心与端口，缺失核心在首次安装时补齐；正常退出旧后台后才替换，连接会短暂重连。下载或验证
失败不替换已安装程序。

## 测试

```bash
go test ./...
go vet ./...
```

真实核心隔离测试使用环境变量指定真实核心与规则文件：

```powershell
$env:MIRROXY_TEST_XRAY = 'D:\Mirroxy\bin\runtime\Mirroxy_core_A.exe'
$env:MIRROXY_TEST_SINGBOX = 'D:\Mirroxy\bin\runtime\Mirroxy_core_B.exe'
$env:MIRROXY_TEST_RULES = 'D:\Mirroxy\bin\resources'
go test -v .
```

`MIRROXY_TEST_RULES` 提供分流规则文件，真实核心测试会把它复制到临时运行目录；不设置时这些
测试会跳过。测试使用临时目录和本机模拟上游，不修改真实系统代理、节点或端口。Windows 代理
测试使用独立注册表路径。macOS 仅交叉编译和包结构校验，没有实机运行证明。

前端 JavaScript 可用 `node --check web/app.js` 做语法检查；`packaging/` 下有若干界面回归脚本。

## 发布流程

```bash
python packaging/build-release.py --go <go>
python packaging/publish-release.py --version 0.8.0
```

`publish-release.py` 校验 `dist/<tag>` 内的 SHA-256，创建草稿 Release、上传四个平台程序包与
在线更新包，再发布正式 Release，并把 `README.md` 与 `install.sh` 同步到仓库默认分支。历史
版本保留；同一版本不得替换为不同字节。

`packaging/publish-s3.py` 与 `packaging/release_site.py` 保留用于历史 S3 发布页归档，新版本
不再上传到 S3。

真实上游核心名称、版本、源码链接和许可证只保留在构建元数据与包内 NOTICE 中。项目实现参考
v2rayN 的协议配置行为；没有复制完整 v2rayN 客户端。

## 项目结构

| 目录 | 说明 |
|---|---|
| 根目录 `*.go` | Go 后端：面板、API、节点、核心、路由、托盘、更新 |
| `web/` | 面板前端（原生 HTML/CSS/JS，含中英 i18n 与主题） |
| `cmd/upgrade/` | Windows 独立更新助手 |
| `cmd/bootstrap/` | 引导安装器后端 |
| `packaging/` | 打包、发布与页面生成脚本 |
| `android/` | 独立开发的 Android 原生版（Kotlin + Jetpack Compose） |
| `licenses/` | 第三方组件许可证与 NOTICE |
| `docs/` | 设计与验证文档 |

## 贡献

欢迎提交 Issue 与 Pull Request。请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md)；安全问题请按
[SECURITY.md](SECURITY.md) 私下报告，不要开公开 Issue。

## 许可证

Mirroxy 本体采用 [MIT 许可证](LICENSE)。项目不修改其分发的第三方组件，它们各自保留原许可
（Xray 为 MPL-2.0，sing-box 为 MIT，aria2 为 GPL-2.0-or-later），完整说明见
[licenses/NOTICE.txt](licenses/NOTICE.txt)。

## 致谢

- [Xray-core](https://github.com/XTLS/Xray-core) — 网络核心 A
- [sing-box](https://github.com/SagerNet/sing-box) — 网络核心 B
- [aria2](https://github.com/aria2/aria2) — 下载引擎
- [v2ray-rules-dat](https://github.com/Loyalsoldier/v2ray-rules-dat)、
  [meta-rules-dat](https://github.com/MetaCubeX/meta-rules-dat) — 分流规则数据
- [v2rayN](https://github.com/2dust/v2rayN) — 协议配置行为的参考实现
