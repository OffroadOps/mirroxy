# Mirroxy

Mirroxy 是一个多节点本地代理管理器。一个 Go 后台进程提供 Web 面板，每个已启动节点绑定一个
固定回环端口，同时支持 HTTP 与 SOCKS5。Windows 不依赖 .NET；Linux 主要面向 Armbian、
Debian、Ubuntu 的 systemd 环境。

源码工作区为 `D:\Mirroxy`。公开发布仍使用兼容入口：
<https://ny-1s.enzonix.com/bucket-1776-2431/scrcpyproxy/index.html>。

## 使用方式

Windows 运行 `Mirroxy-windows-amd64.exe` 安装程序后启动 Mirroxy，打开 <https://127.0.0.1:9527/>（或安装时选择的端口）。导入分享链接、
订阅或 v2rayN SQLite 节点库，再启动所需节点。首次从 5060 起分配空闲 TCP/UDP 端口，随后
保持绑定。下载器填写 `127.0.0.1` 和节点端口即可。

切换到某个节点只需一步：**双击节点行**，程序会按需启动该节点并把系统代理接管过去；顶部的
「接管系统代理」开关可以随时交还或重新接管，节点下拉框和路由模式都即时生效。

Windows 安装向导会检测面板端口占用。端口被占用时，请关闭占用程序后重试，或在向导中选择其他端口；升级默认保留已有面板端口。静默安装可以通过 `/PANELPORT=9528` 指定端口。

默认代理方式是“不改变系统代理”，不会改写 v2rayN 当前使用的 `127.0.0.1:10808`。需要时
可以明确选择清除系统代理、自动配置系统代理或 PAC，并在退出时恢复接管前的设置。

托盘双击打开面板，右键退出后台。关闭网页不会停止代理。节点列表显示连接数、实时上传下载、
累计流量和可用性；可改名、分享、测试、设为系统代理或单独指定路由。

支持导入 VMess、VLESS/REALITY、Shadowsocks、Trojan、SOCKS、HTTP/HTTPS、Hysteria2、
TUIC 与 AnyTLS。不同协议由网络核心 A/B 承载。缺失核心可从官方发布源下载并校验摘要，
最终始终写入通用的 Mirroxy 文件名。

## 下载管理（aria2）

0.7.0 起本机面板新增下载页：采用 IDM 风格顶部工具栏与任务表格，不提供分类侧栏。
支持新建、搜索/筛选、暂停、继续、编辑链接/代理/连接数/限速、重试、单选/多选操作。
默认删除只移除任务记录，保留文件及 `.aria2` 断点数据。0.7.3 起提供任务行/空白处右键菜单，删除使用页面内确认窗。
“同时删除下载文件和断点数据”默认不勾选；勾选时移入保存目录的 `.Mirroxy-trash` 回收区，可根据 `restore.json` 手动恢复。
回收区不会立即释放磁盘空间，不包含任务 URL/代理凭据。只处理关联的普通文件，拒绝外部路径、符号链接、目录和其他任务共用的文件。
编辑先暂停，保留文件名与目录；更换链接须指向同一文件。
每任务最多 16 个连接，同时最多 3 个下载；服务器不支持 Range 或文件较小时实际连接数会减少。

每个任务明确选择直连、已启动的 Mirroxy 节点或自定义 HTTP 代理（可带认证）。
直连不继承环境或 Windows 系统代理，节点模式遵循该节点的出口策略；节点未启动时报错，不自动回退直连。
aria2 不直接支持 SOCKS5，Mirroxy 的同一端口可通过 HTTP 代理使用。
默认保存到当前系统用户的 Downloads，允许自行填写绝对目录，不允许写入程序代码/data/bin 目录。

Windows 完整包预置官方 aria2 1.37.0 独立进程及许可证；更新包保留既有核心，并兼容旧版更新白名单。
在线更新后缺少引擎时，可在下载页点击「安装 aria2 引擎」，从固定官方地址下载并验证 SHA256 后安装，不重启节点。
Linux/macOS 使用系统安装的 `aria2c`（例如 Debian/Ubuntu `sudo apt install aria2`、macOS `brew install aria2`）；未宣称其他平台实机验证通过。
引擎使用随机回环端口与随机 RPC 凭据，不读取其他 aria2/Motrix 的配置或任务。
历史/任务链接与代理信息通过现有加密数据库保存，不使用明文 aria2 session 文件。
正常关闭后台会保存状态并停止下载，下次启动恢复之前正在下载的任务；关闭网页不会停止下载。
本版本下载接口仅本机访问，远程面板暂不开放下载管理。HTTP API 为 `GET /api/downloads`、
`POST /api/downloads/add`、`POST /api/downloads/action`，写入沿用面板 `X-App-Token` 和同源保护。

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
AES-GCM 加密存入 SQLite `data/Mirroxy.db`。Windows 的 256 位主密钥再用当前用户 DPAPI
保护后保存为 `data/Mirroxy.key`。Linux/macOS 密钥文件权限为 0600，因此无法抵抗拥有同一
用户权限或 root 权限的读取者，但普通文件扫描不会直接看到节点明文。备份时数据库和密钥必须
一起保存。

核心启动配置写入 `data/runtime/Mirroxy_session_*.cfg.pending`，校验后启动核心，端口就绪
即删除；失败路径也清理。运行日志使用不可识别核心种类的通用名称。许可证 NOTICE 仍列出真实
上游项目，这是分发依赖必须保留的法律信息。

首次启动 0.5.0 会读取旧版 `settings.json` 和 `traffic.json`，成功写入加密数据库后删除
旧明文文件及旧版生成配置。迁移失败时保留原文件并停止启动，避免静默丢失数据。

## 系统代理与路由

顶部的「接管系统代理」开关是日常入口：打开后系统代理指向所选节点的本地端口，关闭后把
接管前的设置原样交还。节点下拉框列出全部节点，未启动的节点标为「未启动」，选中时会先启动
再接管。另有「清除」用于强制关闭系统代理。默认不接管，因此不会改写 v2rayN 等其它软件
当前使用的代理设置。

| 操作 | 行为 |
|---|---|
| 不接管 | 默认；不写 Windows 代理设置。 |
| 接管系统代理 | 使用所选节点的 HTTP 端口；节点未运行时先启动。 |
| 清除 | 关闭当前静态代理和 PAC。 |

全局路由默认为绕过大陆，也可选择黑名单或全局。每个节点可跟随全局或独立覆盖。节点故障不会
切换到另一节点；固定端口保持失败，以避免下载或应用在不知情时换线路。

每个节点运行在独立的核心进程中。启动或停止一个节点只重启它自己的核心，其它节点正在进行的
连接不会中断；只有切换全局路由模式会重建全部节点的核心并短暂重连。

## Windows 游戏 / 应用代理

0.6.0 起 Windows 本机面板提供“游戏 / 应用代理”。可以从当前运行进程选择完整程序路径，
也可以手动加入多个 `.exe`；每个配置可跟随默认应用节点或单独覆盖节点。选中程序的 TCP/UDP
全部走绑定节点，未选程序保持当前网络方式，本机与局域网地址直连。

应用代理使用单独的 sing-box TUN。节点核心提供只监听 `127.0.0.1` 的随机认证内部入口，
强制进入绑定节点，不继承普通 506x 端口的绕过大陆或黑名单规则。功能默认关闭；开启时请求
一次 Windows 管理员授权，停止或退出 Mirroxy 时撤销 TUN。当前没有宣称 WFP 持久化断网保护：
强杀整个管理员路由助手后 Windows 可能恢复普通直连。

页面还可真实穿过默认应用节点检测 TCP、UDP、IPv6 和 UDP 映射特征。服务器主机可运行
`Mirroxy -probe-server` 输出只读 JSON 报告；两者区别与结果解释见
`docs/server-capability-probe.md`。

## API 与远程访问

本机 API 位于 `http://127.0.0.1:15060/api/v1/`，回环访问免认证。远程监听默认关闭；开启
后网页与 API 都要求账号密码，包括从本机反向代理到远程端口的请求。

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

## 在线更新

0.4.x 到 0.5.0 涉及品牌、主程序名和更新协议变化，需要使用一次 0.5.0 完整包迁移。之后在面板
“更新日志”中在线更新。

Windows 使用安装目录中的独立 `upgrade.exe`。应用校验下载包、逐文件摘要、候选主程序与
候选更新器后，启动候选更新器；它等待旧进程退出，备份程序和数据，替换主程序及自身，再检查
版本和全部启用节点。候选未就绪会恢复旧程序与数据。

Linux/macOS 使用原 PID `exec` 保持 systemd 或 LaunchAgent 监管。所有平台切换时都会短暂
重启连接，不宣称零中断。轻量更新保留已安装核心、数据库与密钥，只补齐缺少的路由集合。

## Linux 与 macOS

Linux ARM64 一键安装：

```bash
curl -fsSL --retry 3 https://ny-1s.enzonix.com/bucket-1776-2431/scrcpyproxy/install.sh | sudo bash
```

离线安装使用 `sudo bash ./Mirroxy-linux-arm64.run` 或 AMD64 对应文件。目标目录
`/opt/mirroxy`，服务为 `mirroxy`，控制工具为 `mirroxyctl`。安装时生成 HTTP 远程面板
地址、admin 和随机密码。旧 `/opt/scrcpyproxy` 数据会复制迁移，旧服务在新服务就绪后停用。

macOS ARM64 解压后执行 `bash ./install-macos.sh`。当前预置核心组合要求 macOS 26+；
安装包未公证且尚未在真实 Mac 上验证。后台由 `com.offroadops.mirroxy` LaunchAgent 管理。

## 构建与验证

需要 Go 1.26+；首次构建固定在线安装器或按需构建离线安装器需要 Inno Setup 7 x64。安装器使用固定在
`packaging/inno/ChineseSimplified.isl` 的官方简体中文语言文件，并同时提供 English：

```powershell
go mod download
go test ./...
go build -trimpath -ldflags '-s -w -H=windowsgui' -o Mirroxy.exe .
go build -trimpath -ldflags '-s -w -H=windowsgui' -o upgrade.exe ./cmd/upgrade
python packaging/build-release.py --go <go>
```

`build-release.py` 会自动在 `D:\Program Files`、`C:\Program Files`、`C:\Program Files (x86)` 和
`%LOCALAPPDATA%\Programs` 下探测 Inno Setup 7 的 `ISCC.exe`；装在非标准位置时用 `--iscc <路径>` 指定。
默认不编译 Windows 安装器，只生成四平台程序包。首次单独运行
`python packaging/build-online-installer.py --go <go> --iscc <路径>`，生成固定
`dist/bootstrap/Mirroxy-online-setup.exe`；后续应用版本重复使用该文件，不重新编译。
需要离线向导时给 `build-release.py` 加 `--offline-windows --iscc <路径>`。

真实核心隔离测试使用：

```powershell
$env:MIRROXY_TEST_XRAY = 'D:\Mirroxy\bin\runtime\Mirroxy_core_A.exe'
$env:MIRROXY_TEST_SINGBOX = 'D:\Mirroxy\bin\runtime\Mirroxy_core_B.exe'
$env:MIRROXY_TEST_RULES = 'D:\Mirroxy\bin\resources'
go test -v .
```

`MIRROXY_TEST_RULES` 提供分流规则文件，真实核心测试会把它复制到临时运行目录；不设置时
这些测试会跳过。测试使用临时目录和本机模拟上游，不修改真实系统代理、节点或端口。Windows
代理测试使用独立注册表路径。macOS 仅交叉编译和包结构校验，没有实机运行证明。

## 发布

`packaging/build-release.py` 生成 Windows x86_64、Linux ARM64、Linux AMD64、macOS ARM64
四个平台完整包与四个平台 `Mirroxy-update-*.zip`。Windows 完整载荷是
`Mirroxy-windows-amd64.zip`。固定 `Mirroxy-online-setup.exe` 每次从 HTTPS
`releases.json` 读取最新版并下载完整 ZIP，验证来源、大小、SHA256、内部文件清单和 AMD64
架构后安装；不会关闭 TLS 校验或打包源码。可填写下载代理，留空直连，不修改系统代理。
`/DOWNLOADPROXY=http://127.0.0.1:10808` 可用于静默安装。
同版本不修改文件，拒绝降级；更高版本使用同一 AppId
原位升级；安装、升级和卸载均保留安装目录内的 `data`。发布白名单不包含源码、本机 data、节点、
密钥、日志或 S3 凭据。在线升级保留已有核心与端口，缺失核心在首次安装时补齐；正常退出旧
后台后才替换，连接会短暂重连。下载或验证失败不替换已安装程序。
历史离线 EXE 继续保留，之后只在需要时编译，不必跟随每次应用更新重打安装器。

```text
python packaging/build-release.py --go <go>
python packaging/publish-s3.py --version 0.7.4 --credentials <临时凭据.json> --verification-proxy http://127.0.0.1:10808
```

发布脚本上传版本文件，匿名重新下载并验证 SHA-256，然后更新稳定安装脚本、`releases.json`
和静态页面。历史版本保留；同一版本不得替换为不同字节。S3 固定前缀和私有 GitHub 仓库名仍
沿用旧标识以保持已有链接兼容。
发布脚本首次上传固定 Windows 在线安装器到 `scrcpyproxy/Mirroxy-online-setup.exe`，以后只核对
相同字节并复用；不允许换成不同内容。每个新版本仍需构建、上传程序 ZIP、四平台在线更新包、
更新日志、稳定 Linux 安装脚本及静态发布页。固定 Windows 安装器不会减少这些程序发布步骤。

真实上游核心名称、版本、源码链接和许可证只保留在构建元数据与包内 NOTICE 中。项目实现参考
v2rayN 的协议配置行为；没有复制完整 v2rayN 客户端。
