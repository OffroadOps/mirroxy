# 更新日志 / Changelog

本文件由 `packaging/render-changelog.py` 从 `web/changelog.json` 与
`web/changelog.en.json` 生成；面板内的「更新日志」页面读的是同样这两个文件。

## 0.9.0 (2026-10-30)

- 节点面板改为 v2rayN 式布局：顶部按订阅分组（全部 / 运行中 / 各订阅 / 手动添加），中间是紧凑可排序的节点表格（名称、协议、延迟、本地端口、连接、实时速度、累计流量），去掉占位的统计卡片和提示横条
- 新增底部状态栏：系统代理「接管 / 清除 / 不改变」与路由「绕过大陆 / 黑名单 / 全局」改为分段按钮，点一下即生效；右侧显示当前系统代理节点、运行数量、总速度与连接数。顶部原来的开关、节点下拉和清除按钮移除
- 节点支持单击选中、Ctrl / Shift 多选、Ctrl+A 全选，右键菜单覆盖全部操作，并可对多个节点批量启动、停止、测速、复制分享链接、修改出口策略与删除
- 新增快捷键：回车切换为系统代理、空格启动 / 停止、Ctrl+R 测速、F2 重命名、Ctrl+E 编辑、Ctrl+C 复制分享链接、Delete 删除、方向键移动、Ctrl+F 搜索
- 在节点页面直接按 Ctrl+V 即可导入剪贴板中的分享链接；粘贴单个订阅地址时自动打开添加订阅窗口并填好地址
- 新增批量测速：多选时测速所选节点，否则测速当前分组全部节点，显示进度并在完成后汇总可用数量；延迟按快慢着色，超时标红
- 每行的出口策略下拉框改为仅在与全局不同时显示的小标签，修改移到右键菜单；分组与排序方式会被记住
- 订阅并入节点面板：每个订阅在顶部有自己的分组标签，右键标签即可更新、编辑或删除；更新失败的订阅在标签上显示红点并在提示中说明原因；分组栏末尾的「＋」直接添加订阅
- 订阅管理页改为表格：名称、地址（只显示域名，完整订阅地址仍不在面板中出现）、节点数、最后更新与状态，支持更新、编辑、删除和全部更新；点击名称跳到节点面板对应分组
- 新增编辑订阅：可修改名称或替换订阅地址，节点、端口与自定义名称保持不变，新地址在下次更新时生效
- 新增删除订阅：可以连同节点一起删除，也可以保留节点为手动添加；不会重启其他节点的核心
- 全局设置改为即时保存：开关和下拉框改动后立即保存，数字项在离开输入框或按回车时保存，设置名称旁显示「保存中 / 已保存」；去掉「保存设置」按钮
- 设置值超出范围时在本地直接提示并恢复原值，不会发出请求；后台拒绝时同样恢复原值并显示原因。修改面板端口前会先确认，取消则保持原端口
- 远程访问时设置项改为只读并注明原因；远程管理的账号密码表单仍需手动保存，避免填到一半就开放远程访问
- 设置、Geo 文件、AI API 与更新日志页去掉英文标语，标题与卡片更紧凑
- 下载管理改为独立窗口：托盘菜单新增「下载管理」，用 Edge 应用窗口打开（无地址栏和标签页），面板侧栏不再显示下载页；没有托盘的平台保留侧栏入口，在新标签页打开同一页面
- 下载窗口改为 IDM 式布局：菜单栏、大图标工具栏、按文件类型分类的左侧目录（压缩文件、文档、音乐、程序、视频、其他）与未完成 / 已完成分组、可排序的任务列表和状态栏，支持深色模式
- 双击未完成任务打开实时刷新的属性窗口（地址、大小、已下载、速率、剩余时间、连接、代理），可在其中暂停、继续、断点重连或打开文件夹；双击已完成任务直接打开文件
- 下载窗口支持多选、右键菜单与快捷键：Ctrl+N 新建、Ctrl+V 粘贴链接新建、Del 删除、空格暂停 / 继续、回车打开；新增「删除已完成」，只删记录保留文件
- 移除不常用的任务编辑与限速入口；「浏览器接力」扩展下载与安装说明移到全局设置页，下载窗口的帮助菜单可直接跳转

<details><summary>English</summary>

- The node dashboard now uses a v2rayN-style layout: subscription tabs along the top (All / Running / each subscription / Manual) and a compact, sortable node table (name, protocol, latency, local port, connections, live speed, total traffic). The placeholder stat cards and hint banners are gone.
- New status bar at the bottom: the system proxy (Take over / Clear / Leave unchanged) and routing (Bypass mainland China / Blacklist / Global) are segmented buttons that apply on click, next to the current system-proxy node, running count, total speed and connections. The old top-bar switch, node picker and clear button are removed.
- Nodes can be selected with a click, Ctrl / Shift for multi-select and Ctrl+A for all. The right-click menu holds every action and works on several nodes at once: start, stop, test, copy share links, change the exit policy and delete.
- New shortcuts: Enter uses the node as system proxy, Space starts / stops, Ctrl+R tests, F2 renames, Ctrl+E edits, Ctrl+C copies share links, Delete removes, arrow keys move and Ctrl+F searches.
- Press Ctrl+V on the node page to import share links from the clipboard. Pasting a single subscription address opens the subscription form with the address filled in.
- Batch latency test: tests the selected nodes when several are selected, otherwise the whole group, with progress and a summary of available nodes. Latency is coloured by speed and timeouts are shown in red.
- The per-row exit-policy dropdown is replaced by a small tag shown only when it differs from the global mode; changing it moved to the right-click menu. The chosen tab and sort order are remembered.
- Subscriptions are part of the node dashboard: each one has its own tab, and right-clicking the tab updates, edits or deletes it. A subscription whose last update failed shows a red dot with the reason in its tooltip, and the "+" at the end of the tabs adds a subscription.
- The Subscriptions page is now a table with name, address (host only; the full subscription address still never appears in the panel), node count, last update and status, with update, edit, delete and update-all actions. Clicking a name opens its tab in the node dashboard.
- Subscriptions can be edited: change the name or replace the address. Nodes, ports and custom names stay as they are, and a new address takes effect on the next update.
- Subscriptions can be deleted together with their nodes, or with the nodes kept as manual nodes. Other nodes' cores are never restarted.
- Settings now save as you change them: switches and lists save immediately, numbers save when you leave the field or press Enter, and a Saving / Saved label appears next to the setting. The Save settings button is gone.
- Out-of-range values are rejected in the page and put back without a request; a value the backend refuses is also put back, with the reason shown. Changing the panel port asks for confirmation first, and cancelling keeps the current port.
- Over remote access the settings are read-only, with a note saying so. The remote-management account form still has its own Save button, so remote access is never opened with half-entered credentials.
- The Settings, Geo files, AI API and Changelog pages drop their slogan headings and use more compact titles and cards.
- The download manager is now its own window: the tray menu has a new Downloads item that opens it as an Edge app window (no address bar or tabs), and the dashboard sidebar no longer shows the download page. Platforms without a tray keep a sidebar link that opens the same page in a new tab.
- The download window uses an IDM-style layout: a menu bar, a large-icon toolbar, a category tree by file type (compressed, documents, music, programs, video, other) with unfinished and finished groups, a sortable task list and a status bar, with a dark theme.
- Double-clicking an unfinished task opens a live properties window (address, size, downloaded, rate, time left, connections, proxy) where it can be paused, resumed, reconnected or its folder opened; double-clicking a finished task opens the file.
- The download window supports multi-selection, a context menu and shortcuts: Ctrl+N new, Ctrl+V paste a link, Del delete, Space pause / resume, Enter open. A new Delete completed action removes the records and keeps the files.
- Task editing and the speed limit were dropped as rarely used. The browser relay extension download and setup steps moved to the Settings page, and the download window's Help menu links there.

</details>

## 0.8.5 (2026-10-28)

- 修复浏览器接力读不到外部网站 Cookie 的问题：扩展的站点权限只覆盖 localhost，Chrome 因此拒绝返回下载站点的 Cookie，「兼容登录态下载」实际不成立；现在扩展对全部站点申请权限，需要登录或防盗链的下载站可以正常带上 Cookie
- 扩展在读不到 Cookie 时会明确提示，不再静默降级，让「登录态失效」和「权限不足」可以区分
- 修复 HTTP / SOCKS 节点无法编辑密码的问题：这两类节点用用户名加密码认证，但编辑窗口把密码框藏了起来；现在用户名和密码都可编辑，并按协议显示对应标签
- SOCKS / HTTP 节点保存时若只填了密码而没有用户名，会明确报错，不再静默丢弃密码

<details><summary>English</summary>

- Fixed the browser relay not being able to read cookies from download sites: the extension only held host permissions for localhost, so Chrome refused to return the cookies of the download site and the promised signed-in downloads did not work. The extension now asks for access to all sites, so downloads that need a sign-in or an anti-hotlink header carry the browser cookies again.
- The extension now says so when it cannot read cookies instead of degrading silently, so a broken sign-in can be told apart from a missing permission.
- Fixed HTTP and SOCKS nodes not being editable: these authenticate with a username and a password, but the editor hid the password box. Both fields are editable now and are labelled for the protocol.
- Saving a SOCKS or HTTP node with a password but no username is refused with a clear message instead of dropping the password silently.

</details>

## 0.8.4 (2026-10-27)

- 浏览器接力改为零配置：扩展通过本机 15060 端口自动发现 Mirroxy 并完成配对，安装扩展后即可使用，不再需要复制或粘贴令牌
- 浏览器接力下载自动携带当前页面的 Referer、Cookie 和 User-Agent，兼容需要登录态或防盗链的下载站（如 123 盘等 CDN 直链）
- 修复手动导入扩展配置后仍沿用旧地址、测试连接提示 Failed to fetch 的体验问题；面板端口变化后扩展会自动重新配对
- 扩展增加自动重试与「重新自动连接」，自动配对失败时才回退到手动配置；浏览器 Cookie 仅发送给本机 Mirroxy，不写入面板显示或日志

<details><summary>English</summary>

- Browser relay is now zero-configuration: the extension discovers Mirroxy through the local 15060 port and pairs automatically, so no token has to be copied or pasted.
- Relayed downloads now carry the page Referer, cookies and User-Agent, so signed-in and anti-hotlink CDNs (including 123pan direct links) keep working.
- Fixed the confusing flow where an imported manual config kept the old address and the connection test showed "Failed to fetch"; the extension re-pairs automatically when the panel port changes.
- The extension now retries pairing automatically and exposes Reconnect; manual configuration is only a fallback. Browser cookies go only to the local Mirroxy and never appear in the panel or logs.

</details>

## 0.8.3 (2026-10-27)

- 新增浏览器下载接力：Chrome / Edge 扩展在浏览器发起下载时先把链接交给 Mirroxy，接单成功后才取消浏览器下载；Mirroxy 未运行、未配置或引擎异常时保留浏览器下载，不丢文件
- 下载管理页新增「浏览器接力」：一键下载扩展 ZIP、复制扩展配置、查看扩展连接状态与安装说明；扩展支持自动接管、右键「使用 Mirroxy 下载」、按网站排除
- 浏览器接力使用独立本机令牌，仅允许本机浏览器扩展来源调用；不读取或转发 Cookie、密码，面板状态不暴露该令牌
- 修复全新安装首次启动时浏览器接力令牌未生成的问题，令牌在后续启动与升级后保持不变

<details><summary>English</summary>

- Browser download relay: a Chrome / Edge extension offers new downloads to Mirroxy first and cancels the browser copy only after Mirroxy accepts. If Mirroxy is stopped, unconfigured or the engine fails, the browser download is kept.
- The Downloads page now includes a Browser relay card: download the extension ZIP, copy its configuration, check connection state and read setup steps. The extension supports automatic takeover, a "Download with Mirroxy" link menu and per-site exclusions.
- The relay uses a dedicated local token and accepts only local browser-extension origins. It never reads or forwards cookies or passwords, and the panel state never exposes the token.
- Fixed the connector token missing on the first start of a fresh installation; the token now persists across restarts and upgrades.

</details>

## 0.8.2 (2026-10-27)

- 默认面板端口改为 9950；未保存过面板端口的新安装若端口被占用，安装器自动顺延（9951、9952……）寻找可用端口，避免安装失败
- 安装时仍支持手动指定端口；已有安装的升级保留原端口，节点、凭据与流量数据不变

<details><summary>English</summary>

- The default local dashboard port is now 9950. For new installations without a saved port, an occupied port is advanced automatically (9951, 9952, ...) so setup does not fail.
- Setup still accepts a manually specified port; upgrades keep the existing port and preserve nodes, credentials and traffic data.

</details>

## 0.8.1 (2026-10-27)

- 新增节点编辑：右键或「⋯」菜单打开编辑窗口，可修改别名、地址、端口、认证信息、流控、传输层与 TLS / REALITY 参数，样式参考 v2rayN 的编辑表单
- 编辑只影响该节点：本地端口、启动状态、流量统计和其它节点的连接都保持不变；运行中的节点保存后由自己的核心短暂重启并应用新参数
- 订阅来源的节点编辑后，刷新订阅不再覆盖你的修改；供应商端改名或换密码也不会让编辑丢失或产生重复节点
- 修复雷神加速器检测失效：进程名匹配加入 leigod（原关键字 leiguan 与实际进程名不符），检测到后自动暂停系统代理、退出后自动恢复的联动恢复生效
- 分享链接导出重构为统一的参数提取逻辑（nodeSpec），编辑表单与分享链接共用同一份数据源，避免两边各自解析产生偏差

<details><summary>English</summary>

- Node editing: open the editor from the right-click or "⋯" menu to change the alias, address, port, credentials, flow, transport and TLS / REALITY parameters, styled after the v2rayN editor.
- Editing affects only that node: the local port, running state, traffic stats and other nodes' connections stay untouched; a running node applies new settings after a short restart of its own core.
- Subscription nodes keep your edits across refreshes; provider-side renames or rotated credentials no longer lose the edit or create duplicates.
- Fixed Leigod (LeiGod accelerator) detection: added the real process name leigod (the previous keyword leiguan never matched), so the auto pause/restore of the system proxy around the accelerator works again.
- Share-link export now shares one extraction path (nodeSpec) with the node editor, so the form and the link always agree on the node's settings.

</details>

## 0.8.0 (2026-10-20)

- 切换节点改为一步：双击节点行即按需启动并接管系统代理，节点菜单改为「启动并设为系统代理」，不再需要先启动再另外配置代理
- 顶部「代理方式 + 应用」表单改为「接管系统代理」开关，节点选择与路由模式即时生效，改完不再有需要点击的「应用」按钮
- 每个节点使用独立核心进程：起停一个节点不再重启其它节点的核心，其它节点正在进行的连接不会被连累
- 托盘「系统代理」子菜单列出全部节点（含未启动的），点选即启动并接管；已生效的节点再次点击即可交还系统代理
- 启动节点后明确提示尚未接管系统代理，并提供一键接管的入口
- 面板版本号改为运行时注入，静态资源缓存键始终与当前版本一致，不再出现前端版本落后于后台的情况
- 面板未信任本地根证书时在页首显示明确提示，不再只依赖浏览器的拦截页
- 修复可用性检测在绕过局域网时误判节点可用的问题，检测流量现在强制经过节点自身出口
- 安装脚本与安装说明改用 GitHub Releases 作为默认下载源（原 S3 页面已冻结在旧版本）；已在发布页校验附件与 SHA256SUMS。

<details><summary>English</summary>

- Switching to a node takes one step: double-clicking a node row starts it when needed and hands the system proxy over to it. The node menu now offers "Start and set as system proxy" instead of a separate start plus proxy configuration.
- The top bar proxy-mode form with its Apply button becomes a single "Take over system proxy" switch. The node picker and the routing mode apply immediately.
- Every node runs in its own core process. Starting or stopping one node no longer restarts the cores of the others, so their running connections survive.
- The tray "System proxy" submenu lists every node, including stopped ones. Picking one starts it and hands the proxy over; picking the active node hands the proxy back.
- Starting a node now says plainly that the system proxy is not taken over yet and offers a one-click way to do it.
- The panel version is injected at runtime, so the asset cache key always matches the running build.
- The panel shows a clear notice when the local root certificate is not trusted yet, instead of relying on the browser interstitial alone.
- Availability checks no longer report a node as usable while the LAN bypass lets the probe succeed without the node outbound; probe traffic is forced through the node itself.
- The install script and install instructions now default to GitHub Releases as the download source (the legacy S3 page stays frozen on old versions); attachment hashes were verified against SHA256SUMS on the release page.

</details>

## Android 0.1.0-dev (2026-10-13)

- 开始独立 Android 原生版开发：Kotlin + Jetpack Compose、官方 Android libXray 核心、节点导入/编辑/改名/删除与 Keystore 加密持久化，源码位于 android/。
- 真机隔离测试已验证 Shadowsocks 加密链路上的 HTTP/SOCKS 请求、固定端口和改名持久化，以及关闭界面后的前台本地代理服务。
- 实现显式授权的 VpnService、IPv4/IPv6 TUN 与分应用选择；VPN 路由实测仍待用户手动系统授权。Android 采用独立开发版本，不升级或替换桌面运行程序。

## 0.7.4 (2026-10-12)

- 下载页桌面布局适配窗口高度，不再整页纵向滚动；任务多时仅任务列表内部滚动，表头保持可见。
- 同时下载上限提高为 30 个任务；按官方 aria2 限制保留每任务最多 16 个连接。
- 右键菜单和详情新增打开完成文件、资源管理器定位文件；仅允许本机打开任务关联的普通文件，未完成文件不能运行。
- 新增断点重连操作，保留原文件与断点数据、链接及代理；下载连接设置 15 秒建连超时、30 秒无数据超时和有限重试，持续无进度时显示重连提示，不关闭 TLS 校验或自动切换代理。

<details><summary>English</summary>

- Desktop download management fits the viewport; large task lists scroll internally with sticky headers.
- Up to 30 concurrent downloads; the official aria2 per-server limit remains 16 connections.
- Open completed files and reveal tasks in the system file manager using validated local journal paths; incomplete files cannot be launched.
- Reconnect stuck downloads while keeping their partial files, resume data, URL and explicit proxy. Bounded connection/idle timeouts and retries report stalls without weakening TLS or falling back to direct access.

</details>

## 0.7.3 (2026-10-12)

- 下载页面新增任务行与空白区域右键菜单，支持多选、按任务状态启用操作、复制链接/路径、键盘菜单和屏幕边缘定位。
- 删除改用页面内悬浮确认窗，默认不勾选删除文件；勾选时只将已下载文件和断点数据移入保存目录的 .Mirroxy-trash 回收区，可恢复，不递归删除目录。
- 新增固定 Windows 在线引导安装器：每次从 HTTPS 发布清单拉取最新版完整程序 ZIP，校验大小、SHA256、文件清单与 AMD64 架构后安装；保留现有核心、用户数据与面板端口，失败不替换程序。以后默认只构建程序包，离线安装器按需生成。

<details><summary>English</summary>

- Download task rows and blank space now have context menus with state-aware actions, multiple selection, copy URL/path, keyboard access and viewport clamping.
- Removing downloads uses an in-page modal. File removal is unchecked by default; opting in moves known downloaded files and resume data to the destination's .Mirroxy-trash recovery area without recursively deleting directories.
- A fixed Windows online installer downloads the latest full program ZIP from the HTTPS catalog, validates size, SHA256, manifest and AMD64 executables before installing, and retains existing cores, data and dashboard port. Release builds no longer compile an installer unless the offline option is requested.

</details>

## 0.7.2 (2026-10-12)

- 修复下载进度条受安全策略拦截而显示满格的问题，使用原生 progress 控件按实际百分比绘制，不放宽 CSP 或使用内联样式。
- 下载失败现在显示 TLS 握手、证书校验、超时、网络、文件不存在等可读原因；不保存上游错误中的原始链接、路径或密码，不关闭证书校验，不自动切换直连。

<details><summary>English</summary>

- Download progress uses a native progress element with the real percentage; strict CSP remains unchanged and no inline styles are needed.
- Known aria2 failures now show readable TLS, certificate, timeout, network and file errors without storing raw URLs, paths or credentials; certificate validation and explicit proxy selection remain enabled.

</details>

## 0.7.1 (2026-10-12)

- 修复下载列表后台刷新时临时禁用工具栏的问题，已选任务的继续、暂停、编辑、重试和删除记录操作保持可用。

<details><summary>English</summary>

- Background refresh no longer disables the toolbar for selected downloads; resume, pause, edit, retry and remove-record controls remain available.

</details>

## 0.7.0 (2026-10-12)

- 集成 aria2 下载管理：IDM 式顶部工具栏与任务表格，无分类侧栏；支持新建、查询、暂停、继续、编辑、重试和批量删除记录，文件与断点数据保留。
- 每任务可设 1–16 个连接、限速、保存目录与文件名；支持直连、已启动 Mirroxy 节点、自定义 HTTP 代理，不改变系统代理，不在节点不可用时自动直连。
- 下载任务历史存入现有加密数据库，后台正常退出后保存与恢复；aria2 RPC 仅回环监听并使用独立随机凭据。Windows 包含官方 aria2 1.37.0；Linux/macOS 需安装 aria2c，下载功能当前仅本机开放。

<details><summary>English</summary>

- Added an aria2 download manager with an IDM-style toolbar and task table, without category sidebar. Add, inspect, pause, resume, edit, retry and remove records individually or in bulk; downloaded files are retained.
- Choose 1–16 connections, speed limit, destination, direct access, a running Mirroxy node or a custom HTTP proxy per task. System proxy settings are unchanged; unavailable nodes never silently fall back to direct access.
- Task history is encrypted and restored after normal backend restarts. Private aria2 RPC uses loopback-only listening and a random secret. Windows bundles official aria2 1.37.0; Linux/macOS require installed aria2c. Download management is local-only.

</details>

## 0.6.3 (2026-10-12)

- 未启动的节点行改为灰色背景和灰色信息文字，启动、复制、改名及更多操作按钮保持原有颜色与可用状态，亮色和暗色主题均支持。
- 已启动节点默认排在列表上方；更多菜单可手动置于列表末尾或恢复自动排序。置底设置持久保存，刷新、重启与订阅更新后保留，排序操作不重启代理核心。

<details><summary>English</summary>

- Stopped rows use gray backgrounds and muted information in both themes; start, copy, rename and other buttons retain their normal colors and availability.
- Started nodes appear first by default. Move to bottom overrides automatic sorting, and Restore automatic order clears that choice. Placement survives refreshes, restarts and subscription updates without restarting proxy cores.

</details>

## 0.6.2 (2026-10-12)

- Windows 安装向导新增面板端口设置（默认 9527）：安装前检测端口占用，提示关闭占用程序或更换端口；复制文件前再次检测，静默安装可使用 /PANELPORT=指定端口。
- 安装器将选择的面板端口保存到加密设置，后续启动继续使用；升级保留已有端口，正常退出旧版后复检，节点、名称、凭据与累计流量保留。

<details><summary>English</summary>

- Windows Setup checks dashboard port availability and lets users choose another port; silent installs support /PANELPORT=.
- Setup preserves the existing dashboard port on upgrades and saves an explicitly chosen port in encrypted settings while retaining nodes and traffic totals.

</details>

## 0.6.1 (2026-09-29)

- 新增「Geo 规则文件」管理页（设置 → Geo 文件）：查看 Xray 路由数据库与 sing-box 规则集共 14 个规则文件的状态、大小与版本，支持一键更新全部或单个文件；缺失的规则文件会标红并可通过「更新全部」补齐。
- Geo 规则文件更新会从规则源（Loyalsoldier v2ray-rules-dat 与 MetaCubeX meta-rules-dat）下载最新数据，下载自动优先经运行中的节点代理，替换前做校验，完成后对应内核短暂重启，节点与流量记录保留。
- 全局设置新增「每日自动更新 Geo 规则文件」开关（默认关闭）；规则源无新版本时自动跳过，不产生下载与重启。

<details><summary>English</summary>

- New Geo files management page (Settings → Geo files): view status, size, and version of all 14 rule assets — the Xray routing databases and sing-box rule sets — with one-click update for all or individual files; missing files are highlighted and can be restored via Update all.
- Geo rule file updates download the latest data from the rule sources (Loyalsoldier v2ray-rules-dat and MetaCubeX meta-rules-dat), prefer a running node as proxy, verify before replacing, and briefly restart the relevant core afterwards. Nodes and traffic totals are preserved.
- Settings gains an Update Geo rule files daily toggle (off by default); when the rule source has no new version, the automatic update skips without downloading or restarting.

</details>

## 0.6.0 (2026-09-07)

- 全局设置新增「内核升级」区块：显示两个内核的当前版本与官方最新版本，进入设置页自动查询官方版本，「升级内核」一键更新（升级会短暂重启对应内核，节点与流量记录保留）。
- 安装包升级后现在默认在桌面和开始菜单创建 Mirroxy 快捷方式（此前桌面图标需要手动勾选）。
- 托盘菜单进一步收窄：流量行改为紧凑格式（如 ↑337.5K/s · 60.4G），版本行仅显示 v0.6.0，「退出 Mirroxy」精简为「退出」。
- 托盘「系统代理」「路由模式」的当前状态改为标题下方的独立灰色信息行显示，菜单标题只保留四个字，整体宽度进一步收窄。
- 托盘菜单按使用频率重排并瘦身：顶部为「系统代理 / 路由模式」两个子菜单，操作项精简为「打开面板 / 重启面板 / UAC运行 / 停止代理」；系统代理子菜单移除「不改变/清除」，点击当前生效的节点即可撤销系统代理（再点其他节点即切换）。
- 彻底移除 PAC 模式：代理方式仅保留 不改变 / 清除 / 自动配置；旧配置中的 PAC 选择会自动迁移，启动时清理注册表中的 PAC 残留，界面、托盘与 AI API 同步精简。
- 应用代理页改版：默认应用节点移至概览中间、能力检测并入概览行，应用列表改为可展开详情的列表（程序可逐条增删），移除冗余说明文字。
- 修复面板刷新后总是回到节点面板的问题：刷新或重开页面后停留在离开前的页面。
- 节点分享弹窗新增「显示二维码」：默认只显示链接，点击后由本机离线生成节点二维码，方便手机扫码导入，链接不会经过任何第三方服务。
- 节点二维码适配主题：暗色模式生成反色二维码与深色底板，不再显示白色卡片；亮色模式保持黑字白底。
- 亮色模式下侧栏改为与内容区一致的亮色配色，各文字对比度同步调整；暗色模式侧栏保持原样。
- Windows 托盘菜单新增「重新加载」：优雅保存状态后自动重启后台，节点、端口与流量记录保留。
- 新增加速器冲突检测：开启应用代理前自动识别网易 UU、雷神、迅游等加速器进程，提示冲突风险并可选择继续或先退出加速器。
- 新增「加速器兼容模式」开关：关闭 TUN 严格路由以降低与 TUN 类加速器的互相拦截；防泄漏保护相应减弱。
- 新增加速器守护（默认开启）：检测到网易 UU、雷神等加速器进程时自动暂停 Mirroxy 设置的系统代理，加速器退出后自动恢复。手动指向节点端口的自定义代理与其他软件的系统代理不受影响，可在全局设置关闭。
- 新增「绕过局域网和本机地址」全局设置（默认开启）：局域网与 127.0.0.1 等本机地址始终直连，全局模式下同样生效；此前仅绕过大陆和黑名单模式内置该行为。
- 托盘菜单大幅扩展：新增「管理员运行」（主动通过 UAC，后续 TUN 授权免提示）、「系统代理」子菜单（不改变/清除/按节点设为系统代理或 PAC，当前项勾选）和「路由模式」子菜单，不开面板即可完成日常切换。
- 托盘菜单重构瘦身：「重新加载」更名「重启」；新增「停止全部代理 / 运行全部代理」全局开关；系统代理子菜单每个节点只出现一次（点选即设为系统代理）；PAC 选项移入「路由模式」子菜单（PAC·节点名）；流量与版本信息行紧凑排版，整体宽度显著收窄。
- 面板升级为 HTTPS：内置本地 CA（mkcert 同类方案）为 127.0.0.1 签发证书，设置页一键安装到系统信任后浏览器显示安全锁，并提供 CA 下载用于 Firefox 和手机。
- 修复 Windows 一键信任根证书可能误报成功及旧标签仍显示不安全的问题：改用当前用户证书库可靠导入，校验证书指纹与实际 HTTPS 握手，并在成功后自动重新建立页面安全连接。
- 面板端口可在全局设置中修改（1024–65000），保存后自动切换并跳转新地址；默认端口变更为 9527，旧地址 15060 仍会自动跳转。PAC 地址随端口自动更新。
- 概览行进一步精简为「默认应用节点 + 节点游戏网络能力」两列；应用列表、目录扫描和指定应用组件完整适配暗色主题。
- 侧栏「游戏 / 应用代理」显示已配置应用数量角标，「订阅管理」显示订阅数量角标。
- 节点「⋯ 更多」菜单新增「指定应用」：可以直接为该节点绑定任意应用，绑定应用按此节点的出口策略分流。
- 应用配置新增目录扫描：填写或浏览一个目录后一键扫描其中的 .exe（最多 3 层子目录），主程序自动识别置顶，勾选即可加入。
- 应用代理的节点专用入口改为跟随该节点的出口策略分流（例如绕过大陆：大陆与局域网直连，其余经节点）；「全部走此节点」模式行为保持不变，本机与局域网地址始终直连。
- 新增 Windows 游戏 / 应用代理：网页选择运行进程或程序路径，选中应用的 TCP/UDP 走绑定节点，未选应用保持原网络方式。
- 应用代理使用独立 TUN 和节点专用认证回环入口；默认关闭，关闭时不修改系统路由和系统代理。
- 新增节点游戏网络能力检测，从客户端真实穿过节点检查 TCP、UDP、IPv6 和 UDP 映射稳定性。
- 新增 -probe-server 只读服务器探针，以 JSON 输出主机 TCP、UDP、IPv6、STUN 映射及两个网络核心版本。

<details><summary>English</summary>

- Settings gained a “Core upgrade” section showing the current and official latest version of both cores. The official versions are checked automatically when the page opens, and “Upgrade core” updates in one click (the relevant core restarts briefly; nodes and traffic totals are preserved).
- Installer-based upgrades now create Mirroxy shortcuts on the desktop and in the Start menu by default (the desktop icon previously required an opt-in checkbox).
- The tray menu narrowed further: traffic rows use a compact format (e.g. ↑337.5K/s · 60.4G), the version row shows just v0.6.0, and the exit item became simply “退出”.
- The tray “System proxy” and “Routing” titles are now four plain words with the current state shown as a grayed info line underneath, narrowing the menu further.
- The tray menu was reordered and slimmed: the “System proxy / Routing” submenus sit on top, actions are condensed to “Open dashboard / Restart dashboard / Run elevated / Stop proxies”, and the system-proxy submenu dropped the unchanged/clear entries — clicking the active node releases the system proxy, clicking any other node switches to it.
- PAC mode was removed entirely: proxy modes are now unchanged / clear / use selected node only. Stored PAC choices migrate automatically, leftover registry PAC settings are cleaned at startup, and the UI, tray, and AI API were simplified accordingly.
- App proxy page redesign: the default node moved to the center of the overview with the capability test beside it, the application list became an expandable list with per-line executable management, and redundant copy was removed.
- Fixed the dashboard always returning to the node page on refresh: the dashboard now reopens on the page you left.
- The node share dialog gained “Show QR code”: the link is shown by default, and clicking generates the node QR code offline on this machine for phone import — the link never touches any third-party service.
- The QR code now follows the theme: dark mode renders an inverted code on the dark card colour instead of a glaring white plate; light mode keeps black on white.
- The sidebar now uses a light palette in light mode with adjusted text contrast; the dark sidebar remains in dark mode.
- The Windows tray menu gained “Reload”: the backend saves its state and restarts automatically, keeping nodes, ports, and traffic totals.
- Added accelerator conflict detection: before starting the app proxy, Mirroxy identifies running processes from NetEase UU, Leiguan, Xunyou and similar accelerators, warns about the conflict, and lets you continue or quit the accelerator first.
- Added an “Accelerator compatibility mode” switch: it disables TUN strict routing to reduce mutual blocking with TUN-based accelerators; leak protection becomes weaker as a trade-off.
- Added the accelerator guard (on by default): while NetEase UU, Leiguan, or similar accelerator processes run, the system proxy set by Mirroxy is paused automatically and restored when they exit. Custom per-app proxy targets and other software’s system proxies stay untouched; the guard can be disabled in Settings.
- Added a “Bypass LAN and local addresses” global setting (on by default): LAN and loopback addresses stay direct even in Global mode; previously only Bypass-Mainland and Blacklist modes behaved this way.
- Expanded the tray menu: “Run as administrator” (approves UAC up front so later TUN prompts are skipped), a “System proxy” submenu (unchanged / clear / set proxy or PAC per running node, current choice checked), and a “Routing” submenu — everyday switching no longer requires the dashboard.
- Tray menu refactor: “Reload” became “Restart”; a global “Stop all proxies / Run all proxies” switch was added; the system-proxy submenu lists each node once (click to set as system proxy); the PAC options moved into the “Routing” submenu (PAC·node); traffic and version rows are compact, so the menu is much narrower.
- The dashboard now serves HTTPS: a built-in local CA (the mkcert approach) issues a certificate for 127.0.0.1, a one-click install adds it to the system trust store for the padlock, and the CA can be downloaded for Firefox and phones.
- Fixed Windows root-CA installation falsely reporting success and an already-open tab remaining marked unsafe: Mirroxy now verifies the exact certificate and live HTTPS handshake, then automatically establishes a fresh secure page connection.
- The panel port is now editable in Settings (1024–65000); saving switches over and redirects to the new address. The default port changed to 9527, and the old 15060 address keeps redirecting. PAC URLs follow the port automatically.
- The overview was simplified to two columns (default node + gaming capability test); the application list, folder scan, and assign-apps components now fully support the dark theme.
- The sidebar “Games / App proxy” item now shows a profile count badge, and “Subscriptions” shows a subscription count badge.
- Added “Assign apps” to each node's more menu: bind arbitrary applications directly to a node, and bound apps follow that node's routing policy.
- App profiles gained a folder scan: enter or browse a folder, scan its .exe files (up to three levels deep), with main programs detected and pre-selected on top.
- The authenticated per-node app ingress now follows the node's routing policy (for example bypass Mainland China: mainland and LAN traffic connects directly while the rest uses the node). The route-all mode behaves as before, and loopback and LAN addresses always stay direct.
- Added Windows game and app proxy controls in the web dashboard. Select running processes or executable paths; selected apps route TCP and UDP through their bound node while other apps remain unchanged.
- App routing uses a separate TUN and authenticated per-node loopback ingress. It is off by default and leaves system routes and the system proxy untouched while off.
- Added an end-to-end node gaming capability test for TCP, UDP, IPv6, and UDP mapping stability.
- Added a read-only -probe-server command that reports host TCP, UDP, IPv6, STUN mapping, and both network-core versions as JSON.

</details>

## 0.5.2 (2026-09-06)

- 精简本地桌面侧栏：移除工作空间身份、免登录说明和退出块；Linux 服务端继续保留登录与后台管理入口。
- 语言与主题控件改为同排紧凑布局，语言默认显示 Auto，主题使用月亮与太阳矢量图标。
- AI API / 远程访问和更新日志归入全局设置，并将“我的节点”调整为“节点列表”、清理节点页冗余说明。

<details><summary>English</summary>

- Simplified the local desktop sidebar by removing workspace identity, no-sign-in details, and quit controls; Linux server deployments retain authentication and backend management controls.
- Placed language and theme controls on one compact row, labeled the automatic language choice Auto, and replaced theme text with moon and sun vector icons.
- Moved AI API / Remote Access and Changelog under Settings, renamed My nodes to Node list, and removed redundant notes from the node page.

</details>

## 0.5.1 (2026-09-06)

- Windows 托盘菜单新增当前版本号；检测到更高版本时才显示可选更新入口，点击后仍需确认，不强制安装。

<details><summary>English</summary>

- Added the current version to the Windows tray menu. An optional update action appears only when a newer version is available and still asks for confirmation before installing.

</details>

## 0.5.0 (2026-09-05)

- 新增简体中文与 English 双语界面；首次访问默认跟随浏览器语言，手动选择后由浏览器记住偏好。
- Windows 完整包改为 Inno Setup 7 x64 安装程序，自动识别中英文；同版本重复运行不改动，高版本直接升级并保留 data 用户数据。
- 公开发布页升级为简洁的 Mirroxy 产品官网，加入隐私安全的虚构节点演示、核心功能介绍、分平台下载与完整版本历史。
- 产品更名为 Mirroxy，启用全新的网页、浏览器、Windows 程序和托盘图标。
- Windows 在线更新改用安装目录中独立的 upgrade.exe：校验候选主程序与更新器后等待旧进程退出，替换失败或新版本未就绪时自动回滚。
- 安装目录中的核心、路由资源、运行配置和日志统一改用 Mirroxy 通用文件名；今后从官方来源更新核心时也会自动写入固定的通用名称。
- 节点、订阅、远程凭据、累计流量和系统代理恢复信息迁移到 AES-GCM 加密的 SQLite 数据库；Windows 密钥由当前用户 DPAPI 保护，迁移成功后删除旧版明文 JSON 与运行配置。
- 0.4.x 到 0.5.0 涉及主程序、目录及更新协议改名，需要使用一次完整安装包；从 0.5.0 起继续在面板内在线更新。

<details><summary>English</summary>

- Added Simplified Chinese and English UI. First visit follows the browser language; a manual choice is remembered locally.
- Replaced the Windows ZIP with an Inno Setup 7 x64 installer that detects Chinese or English, skips same-version reinstalls, upgrades newer versions in place, and preserves user data.
- Redesigned the public release page as a concise Mirroxy product site with privacy-safe demo data, product highlights, platform downloads, and complete release history.
- Renamed the product to Mirroxy with new web, browser, Windows executable, and tray icons.
- Windows updates now use a separate upgrade.exe that verifies the candidate, waits for the old process, and rolls back when replacement or startup validation fails.
- Core binaries, routing assets, runtime configuration, and logs now use generic Mirroxy filenames.
- Nodes, subscriptions, remote credentials, traffic totals, and proxy recovery state are stored in an AES-GCM encrypted SQLite database; Windows protects the key with DPAPI.
- Moving from 0.4.x to 0.5.0 requires one full installer because the executable, directory, and update protocol changed. In-app updates resume from 0.5.0 onward.

</details>

## 0.4.4 (2026-09-04)

- 新增蓝绿 S 品牌图标，统一网页标志、浏览器图标与 Windows 程序及托盘图标。
- 新增亮色 / 暗色模式切换，默认亮色；浏览器记住选择，页面、表单和弹窗同步适配。
- 实际生效的系统代理节点整行高亮，并显示“系统代理”或“系统 PAC”标记；切换节点后自动更新，未应用的选择不提前高亮。

<details><summary>English</summary>

- Added the blue-green S brand icon across the web UI, browser, Windows program, and tray.
- Added light and dark themes, defaulting to light and remembering the browser preference.
- Highlighted the active system-proxy node and kept the indicator synchronized after changes.

</details>

## 0.4.3 (2026-09-04)

- 顶部新增全局路由模式：绕过大陆、黑名单、全局，默认绕过大陆；节点可跟随全局或单独指定策略。
- 补齐两种核心的黑名单规则，改进大陆 IP 分流，联网监控强制通过所选节点检测。
- 新增程序在线更新：下载并校验轻量更新包，保留节点、端口、自定义名称及累计流量，自动重启恢复。

<details><summary>English</summary>

- Added global routing modes: Bypass Mainland China, Blacklist, and Global, with per-node overrides.
- Completed blacklist and Mainland China split-routing rules for both network cores.
- Added verified lightweight in-app updates that preserve nodes, ports, custom names, and traffic totals.

</details>

## 0.4.2 (2026-09-04)

- 左侧新增“更新日志”，可离线查看各版本新增功能与修复。
- 每个节点名称旁新增 🖊 编辑按钮，支持自定义名称，订阅更新后继续保留。
- 改名立即保存，无需重启代理；分享链接与 AI API 同步使用新名称。
- 新增公开静态发布页，汇总所有发布版本与各平台下载，后续发布自动更新并保留历史版本。

<details><summary>English</summary>

- Added an offline changelog view.
- Added persistent custom node names that survive subscription refreshes.
- Name changes take effect without restarting cores and are used by share links and the AI API.
- Added the public static release page with retained version history.

</details>

## 0.4.1 (2026-09-04)

- 节点右键菜单与更多菜单新增“分享链接”，可复制到兼容客户端。
- 提供 Windows x86_64、Linux ARM64 / AMD64、macOS ARM64 安装包，预置两个网络核心。
- 上线公开下载安装地址及 Linux 一键安装脚本；macOS 安装包要求 macOS 26 或更新版本。

<details><summary>English</summary>

- Added share-link export to node menus.
- Added Windows x86_64, Linux ARM64 / AMD64, and macOS ARM64 packages with two bundled network cores.
- Published public downloads and a Linux one-line installer; macOS requires version 26 or newer.

</details>

## 0.4.0 (2026-09-04)

- 支持 Armbian / Debian / Ubuntu 服务器部署，通过 systemd 常驻运行与开机启动。
- 安装时生成 HTTP 管理地址和账号密码；本机免登录，远程访问需要认证。
- 支持导入节点后自动选择核心、下载缺失核心、分配端口并启动代理。

<details><summary>English</summary>

- Added deployment for Armbian, Debian, and Ubuntu with systemd startup.
- Installation generates an HTTP management address and credentials; local access remains passwordless.
- Added automatic core selection, missing-core downloads, port allocation, and node startup.

</details>

## 0.3.1

- 移除节点面板大幅介绍区，缩短汇总区域，将节点列表整体上移。
- 系统代理选择放入顶部工具栏，导入按钮移至节点列表标题旁。

<details><summary>English</summary>

- Compacted the node dashboard summary and moved the table upward.
- Moved system-proxy selection to the top toolbar and import beside the node heading.

</details>

## 0.3.0

- 增加全部节点可用性监控，名称默认绿色，检测失败变红，恢复后变回绿色。
- 顶部显示全节点实时上传、下载速度及累计上传、下载流量。

<details><summary>English</summary>

- Added availability monitoring for every node with automatic status-color recovery.
- Added aggregate live upload, download, and cumulative traffic totals.

</details>

## 0.2.0

- 节点改为列表展示，增加每节点连接数、实时速度与累计流量。
- 增加 Windows 托盘、全局设置、AI API、远程账号密码认证与 PAC 功能。
- 支持四种系统代理模式，默认不改变系统代理，便于与 v2rayN 同时使用。

<details><summary>English</summary>

- Changed nodes to a table with connections, live rates, and cumulative traffic.
- Added the Windows tray, settings, AI API, remote authentication, and PAC.
- Added four system-proxy modes while defaulting to no change for v2rayN coexistence.

</details>

## 0.1.0

- 首个 Web 管理面板，后台二进制常驻运行。
- 导入分享链接、订阅或 v2rayN 节点，从 5060 起为不同节点分配独立的本地代理端口。

<details><summary>English</summary>

- Introduced the web dashboard backed by a persistent binary process.
- Added share-link, subscription, and v2rayN imports with stable per-node local ports starting at 5060.

</details>
