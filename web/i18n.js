'use strict';
(() => {
  const storageKey = 'Mirroxy.language';
  const zh = {
    processing: '处理中…', backendOnline: '后台运行中', backendOffline: '后台已断开',
    pageNodes: '节点面板', pageSubscriptions: '订阅管理', pageSettings: '全局设置', pageAPI: 'AI API / 远程访问', pageChangelog: '更新日志', pageGeoFiles: 'Geo 文件',
    available: '可用', unavailable: '不可用', pendingCheck: '待检测', checking: '检测中',
    running: '运行中', startError: '启动异常', stopped: '未启动', coreA: '网络核心 A', coreB: '网络核心 B',
    start: '启动', stop: '停止', currentVersion: '当前版本', noRecords: '暂无记录',
  };
  const en = {
    '置于列表末尾': 'Move to bottom', '恢复自动排序': 'Restore automatic order', '已置底': 'Moved to bottom',
    '已置于列表末尾': 'Moved to the bottom of the list', '已恢复自动排序': 'Automatic order restored',
    '手动置底，启动后仍保持在列表末尾': 'Manually moved to the bottom; stays there when started',
    processing: 'Working…', backendOnline: 'Backend running', backendOffline: 'Backend disconnected',
    pageNodes: 'Node Dashboard', pageSubscriptions: 'Subscriptions', pageSettings: 'Settings', pageAPI: 'AI API / Remote Access', pageChangelog: 'Changelog', pageGeoFiles: 'Geo files',
    available: 'Available', unavailable: 'Unavailable', pendingCheck: 'Not checked', checking: 'Checking',
    running: 'Running', startError: 'Failed to start', stopped: 'Stopped', coreA: 'Network Core A', coreB: 'Network Core B',
    start: 'Start', stop: 'Stop', currentVersion: 'Current version', noRecords: 'No records yet',
    'Mirroxy · 本地代理面板': 'Mirroxy · Local Proxy Dashboard',
    '我的本地代理': 'My Local Proxy', 'Windows 工作空间': 'Windows workspace', '工作空间': 'Workspace',
    '节点面板': 'Node Dashboard', '订阅管理': 'Subscriptions', '全局设置': 'Settings', 'AI API / 远程': 'AI API / Remote', 'AI API / 远程访问': 'AI API / Remote Access', '更新日志': 'Changelog',
    '语言': 'Language', '暗色模式': 'Dark mode', '亮色模式': 'Light mode', '切换到暗色模式': 'Switch to dark mode', '切换到亮色模式': 'Switch to light mode',
    '本机免登录': 'Local access · No sign-in', '关闭网页，代理依然运行': 'Proxy stays running after the page is closed', '退出后台': 'Quit backend',
    '代理方式': 'Proxy mode', '系统代理模式': 'System proxy mode', '系统代理使用的节点': 'Node used by system proxy',
    '清除系统代理': 'Clear system proxy', '自动配置系统代理': 'Use selected node', '不改变系统代理': 'Leave system proxy unchanged', 'PAC 模式': 'PAC mode',
    '路由模式': 'Routing mode', '全局路由模式': 'Global routing mode', '绕过大陆 (Whitelist)': 'Bypass Mainland China (Whitelist)', '黑名单 (Blacklist)': 'Blacklist', '全局 (Global)': 'Global', '应用': 'Apply',
    '全部节点': 'All nodes', '等待首次可用性检测': 'Waiting for the first availability check', '运行中的代理': 'Running proxies',
    '↑ 全节点实时上传': '↑ Live upload', '累计上传': 'Uploaded', '↓ 全节点实时下载': '↓ Live download', '累计下载': 'Downloaded',
    '节点列表': 'Node list', '导入节点': 'Import nodes', '搜索节点或协议': 'Search nodes or protocols', '搜索节点': 'Search nodes', '全部停止': 'Stop all',
    '每秒刷新': 'Refreshes every second', '连接': 'Connections', '可用性监控准备中': 'Availability monitor is getting ready',
    '节点 / 状态': 'Node / Status', '本地 HTTP / SOCKS5': 'Local HTTP / SOCKS5', '连接数': 'Connections',
    '当前已建立的入站 TCP 连接；不含 UDP 流数量': 'Current inbound TCP connections; excludes UDP flows', '↑ 上传速度': '↑ Upload', '↓ 下载速度': '↓ Download', '出口策略': 'Routing', '操作': 'Actions',
    '速度单位 B/s；累计流量从本版本启用统计后计算，关闭网页或正常重启不会清零。包括此端口按规则直连的流量。': 'Rates use B/s. Totals persist when the page is closed or the app restarts, and include traffic routed directly by the selected rules.',
    '从你的第一个节点开始': 'Start with your first node',
    '下载器中填写 127.0.0.1 和节点显示的端口即可。要让浏览器使用它，请选择该节点的「设为系统代理」。': 'Enter 127.0.0.1 and the displayed node port in your app. To use it in a browser, choose “Set as system proxy” for that node.',
    '更新节点信息，已匹配节点的本地端口保持不变。': 'Refresh node details while keeping matched local ports unchanged.', '＋ 添加订阅': '＋ Add subscription',
    '支持常见分享链接组成的明文或 Base64 订阅。订阅中找不到原节点时会停用该节点，不会自动改走其他线路。': 'Supports plain-text or Base64 subscriptions with common share links. Missing nodes are disabled instead of silently switching routes.',
    '全局设置导航': 'Settings navigation', '常规': 'General', '常用运行选项，修改后点击保存。': 'Common runtime options. Save after making changes.', '保存设置': 'Save settings', '启动与运行': 'Startup & runtime',
    '登录 Windows 后自动启动': 'Start after Windows sign-in', '随当前用户登录启动后台，不需要管理员权限': 'Starts for the current user without administrator privileges',
    '手动启动时打开网页面板': 'Open dashboard on manual launch', '开机自启始终静默；托盘双击随时打开面板': 'Automatic startup stays silent; double-click the tray icon to open the dashboard',
    '启动时恢复已启用节点': 'Restore enabled nodes on startup', '关闭后，下次启动将所有节点置为停止；系统代理默认不改变': 'When disabled, all nodes start stopped; the system proxy remains unchanged by default',
    '起始代理端口': 'Starting proxy port', '仅影响新分配的端口，已有端口保持不变': 'Only affects newly assigned ports; existing ports remain unchanged',
    '新导入节点的默认出口策略': 'Default routing for imported nodes', '已有节点可在列表中分别修改': 'Existing nodes can be changed individually',
    '跟随全局路由（推荐）': 'Follow global routing (recommended)', '黑名单': 'Blacklist', '全部流量走此节点': 'Route all traffic through this node', '绕过局域网和大陆': 'Bypass LAN and Mainland China',
    '核心日志等级': 'Core log level', '排查连接问题时可临时使用详细日志；改变后重启相应核心': 'Use detailed logs temporarily for troubleshooting; changing this restarts the relevant core',
    '警告（推荐）': 'Warnings (recommended)', '仅错误': 'Errors only', '一般信息': 'Information', '调试': 'Debug',
    '节点可用性监控': 'Node availability monitoring', '自动检测所有节点': 'Automatically check all nodes',
    '默认绿色；联网检测失败后变红，恢复后变绿。关闭网页后仍会检测。': 'Nodes are green by default, turn red after a failed check, and recover to green. Checks continue after the page is closed.',
    '检测间隔（秒）': 'Check interval (seconds)', '默认 60 秒，可设为 30–3600 秒；主站失败后尝试备用站点。': 'Defaults to 60 seconds; accepts 30–3600 seconds and tries a fallback endpoint when needed.',
    '停止的节点也会检测，检测完成后保持停止。检测会产生少量流量；运行中节点经本地端口产生的检测流量也计入累计值。名称旁显示最近的检测结果，悬停可查看时间和延迟。': 'Stopped nodes are also checked and remain stopped. Checks use a small amount of traffic. Hover the latest result beside a node name to see its time and latency.',
    '导入与核心': 'Import & cores', '导入后自动部署节点': 'Deploy nodes after import', '识别协议，选择对应核心并自动分配独立端口。Linux 默认开启。': 'Detects the protocol, chooses a core, and assigns a dedicated port. Enabled by default on Linux.',
    '缺少核心时自动下载': 'Download missing cores automatically', '优先使用预置核心；缺失时下载匹配系统和 CPU 架构的官方核心并校验。': 'Uses bundled cores first; missing cores are downloaded from official releases and verified.',
    'Linux 后台由 systemd 管理。开机自启和服务重启可使用控制台命令 Mirroxyctl。': 'The Linux backend is managed by systemd. Use Mirroxyctl for startup and service restarts.',
    '更新': 'Updates', '每日自动更新核心': 'Update cores daily', '校验官方包并检查配置兼容性；更新会短暂重启相应核心': 'Verifies official packages and configuration compatibility; the relevant core restarts briefly',
    '自动更新订阅': 'Update subscriptions automatically', '保留匹配节点的端口，配置变更时重启相应核心': 'Keeps ports for matched nodes and restarts the relevant core after configuration changes',
    '订阅更新间隔（小时）': 'Subscription interval (hours)', '开启自动更新订阅时生效': 'Applies when automatic subscription updates are enabled',
    'PAC 自动代理脚本': 'PAC automatic proxy script', '额外直连域名（每行一个，支持通配符）': 'Additional direct domains (one per line; wildcards supported)',
    '不填写时只使用内置局域网规则。脚本不会在代理故障时自动改走其他节点。': 'When empty, only built-in LAN rules are used. The script never switches nodes after a proxy failure.',
    '代理核心': 'Proxy cores', '运行记录': 'Activity log', '暂无记录': 'No records yet', '未检测到': 'Not detected',
    '本机直接调用，远程通过账号密码管理，支持 HTTP / HTTPS。': 'Use directly on this device or manage remotely with credentials over HTTP / HTTPS.',
    '给 AI 使用的 API': 'API for AI tools', '复制 AI 接入说明': 'Copy AI instructions', '接口说明': 'API guide', '远程管理': 'Remote management',
    '默认关闭；开启前请设置账号密码。': 'Disabled by default. Set a username and password before enabling it.', '允许远程管理访问': 'Allow remote management',
    '开放独立管理端口；包括局域网在内的非本机访问均需账号密码': 'Opens a dedicated management port; all non-local access requires credentials',
    '连接协议': 'Protocol', '远程管理端口': 'Remote management port', '账号': 'Username', '密码': 'Password', '自行设置账号': 'Choose a username', '至少 10 个字符；已有密码时留空保留': 'At least 10 characters; leave blank to keep the current password',
    'HTTPS 证书（可选）': 'HTTPS certificate (optional)', '证书文件（PEM）': 'Certificate file (PEM)', '私钥文件（PEM）': 'Private key file (PEM)', '保存远程设置': 'Save remote settings',
    '各版本新增功能与修复，离线也可查看。': 'Features and fixes for every version, available offline.', '当前版本': 'Current version',
    '检查程序更新，保留节点和流量记录。': 'Check for app updates while preserving nodes and traffic totals.', '检查更新': 'Check for updates', '下载并更新': 'Download & update',
    '本机运行 · 独立端口 · 固定出口': 'Runs locally · Dedicated ports · Fixed routes',
    '添加你的节点': 'Add your nodes', '选择一种方式，导入后即可开启代理。': 'Choose an import method, then start using the proxy.', '关闭': 'Close',
    '从 v2rayN 导入': 'Import from v2rayN', '分享链接': 'Share links', '订阅地址': 'Subscription URL', 'v2rayN 所在目录': 'v2rayN folder', '例如 D:\\v2rayN': 'Example: D:\\v2rayN',
    '读取现有节点，不修改 v2rayN 配置。节点密码保存在本机。': 'Reads existing nodes without changing v2rayN. Node credentials stay on this device.',
    '节点分享链接': 'Node share links', '订阅名称': 'Subscription name', '例如：我的下载线路': 'Example: Download route',
    '导入后自行选择要启动的节点': 'Choose which nodes to start after import', '节点链接': 'Node link', '复制链接': 'Copy link',
    '修改节点名称': 'Rename node', '节点名称': 'Node name', '例如：日常上网 / 下载专用': 'Example: Daily use / Downloads', '保存名称': 'Save name', '核心日志': 'Core logs',
  };
  Object.assign(en, {
    '浏览器语言 / Browser': 'Browser language / Auto', '简体中文': '简体中文',
    '下载器中填写': 'Enter',
    '和节点显示的端口即可。要让浏览器使用它，请选择该节点的「设为系统代理」。': 'and the displayed node port in your app. To use it in a browser, choose “Set as system proxy” for that node.',
    'Mirroxy 已启动': 'Mirroxy started',
    '请手动复制：': 'Copy manually: ', '后台响应异常，请刷新页面': 'Unexpected backend response. Refresh the page.', '操作未完成': 'The operation did not complete.',
    '在本机应用中设置 HTTP / SOCKS5 代理，或在控制台使用：curl -x http://127.0.0.1:节点端口 https://example.com': 'Configure an HTTP / SOCKS5 proxy in a local app, or run: curl -x http://127.0.0.1:node-port https://example.com',
    ' · 服务端': ' · Server', '导入后自动选择核心、分配端口并启动': 'Automatically select a core, assign a port, and start after import',
    '最后更新：': 'Last updated: ', '尚未更新': 'Never updated', '更新订阅': 'Refresh', '还没有添加订阅': 'No subscriptions yet', '添加订阅后，可以在这里一键更新节点。': 'Add a subscription to refresh its nodes here.',
    '正在提供代理': 'Serving proxy traffic', '待命': 'Standby', '未检测到': 'Not detected', ' · 最新 ': ' · Latest ', '更新核心': 'Update core', '日志': 'Logs',
    '远程 {scheme} 已开启，端口 {port}；需要账号密码。': 'Remote {scheme} is running on port {port}; credentials are required.',
    '远程访问已关闭。本机仍可免登录使用面板和 API。': 'Remote access is off. The local dashboard and API remain available without sign-in.',
    '本机免登录 · 远程需认证': 'Local access · Remote authentication required', '运行目录：': 'Runtime folder: ',
    '每 {seconds} 秒检测所有节点': 'Check all nodes every {seconds} seconds', '自动检测已关闭': 'Automatic checks are off', '最近检测：': 'Last checked: ', '尚未完成首次联网检测': 'The first network check has not finished',
    '个节点等待统计': 'nodes are waiting for statistics', '统计连接中断': 'Statistics connection interrupted',
    '修改 {name} 的名称': 'Rename {name}', '修改节点名称': 'Rename node', 'Windows 当前正在使用此节点的 PAC 脚本': 'Windows is using this node’s PAC script', 'Windows 当前正在使用此节点作为系统代理': 'Windows is using this node as the system proxy',
    '系统 PAC': 'System PAC', '系统代理': 'System proxy', '网络': 'Network ', '点击复制地址': 'Click to copy address', '启动后分配': 'Assigned after start', '{name}出口策略': '{name} routing', '{name}更多操作': 'More actions for {name}',
    '跟随全局': 'Follow global', '绕过大陆': 'Bypass Mainland China', '全部走此节点': 'Route all through this node', '立即检测可用性': 'Check availability now', '设为系统代理': 'Set as system proxy', '设为系统 PAC': 'Set as system PAC', '复制 PAC 地址': 'Copy PAC URL', '重新分配端口': 'Reassign port', '查看核心日志': 'View core logs', '删除节点': 'Delete node', '没有匹配的节点': 'No matching nodes',
    '局域网和大陆直连，其他流量走节点': 'Connect to LAN and Mainland China directly; route other traffic through the node', '命中代理名单走节点，其余直连': 'Route matched destinations through the node and connect to others directly', '全部流量走对应节点': 'Route all traffic through the selected node', '。适用于跟随全局的节点，点击应用生效。': '. Applies to nodes following global routing; click Apply to save.',
    '不写入系统设置。当前 v2rayN 等软件的系统代理保持原样；各节点端口可独立使用。': 'Do not change system settings. Existing proxy software remains in control; each node port can still be used directly.',
    '应用后将关闭当前 Windows 系统代理和 PAC，包括其他代理软件配置的代理。': 'Turn off the current Windows system proxy and PAC, including settings made by other proxy apps.',
    '应用后将所选节点设为 Windows 系统代理。请先启动该节点。': 'Use the selected node as the Windows system proxy. Start the node first.', '应用后使用所选节点的 PAC 自动代理脚本。请先启动该节点。': 'Use the selected node’s PAC script. Start the node first.',
    '当前 PAC：': 'Current PAC: ', '当前系统代理：': 'Current system proxy: ', '当前未启用系统代理': 'No system proxy is currently enabled', '应用路由及代理方式': 'Apply routing and proxy mode',
    '添加并更新': 'Add & refresh', '导入并部署': 'Import & deploy', '当前 v': 'Current v', ' · 可更新至 v': ' · Update available: v',
    '更新已开始，下载期间代理继续运行；后台重启后页面自动恢复。': 'Update started. Proxies keep running during download, and the page reconnects after the backend restarts.',
    '请填写节点名称': 'Enter a node name', '节点名称已保存': 'Node name saved', '正在读取更新日志…': 'Loading changelog…', '更新日志读取失败，请在全局设置中重试。': 'Could not load the changelog. Open Changelog in Settings to retry.',
    '路由及代理方式已应用': 'Routing and proxy mode applied', '请填写订阅地址': 'Enter a subscription URL', '导入 {count} 个节点。': 'Imported {count} nodes.',
    '已导入 {count} 个节点，自动启动 {deployed} 个代理': 'Imported {count} nodes and automatically started {deployed} proxies', '已导入 {count} 个节点，点击「启动」即可使用': 'Imported {count} nodes. Click Start to use them.',
    '分享链接已复制': 'Share link copied', '请复制已选中的链接': 'Copy the selected link', '已复制 ': 'Copied ', '删除这个节点？它的本地代理端口也会停止。': 'Delete this node? Its local proxy port will stop.',
    '代理已启动，端口固定绑定此节点': 'Proxy started; its port remains bound to this node', '操作已完成': 'Operation completed', '节点可用：': 'Node available: ', '系统 PAC 已启用': 'System PAC enabled', '系统代理已启用，支持系统代理的浏览器和应用现在使用此节点': 'System proxy enabled. Compatible browsers and apps now use this node.',
    '订阅已更新，共 {count} 个节点': 'Subscription refreshed: {count} nodes', '官方最新版本：': 'Latest official version: ', '正在下载并检查核心更新，请稍候…': 'Downloading and verifying the core update…', '核心已就绪': 'Core ready',
    '全局设置已保存': 'Settings saved', '远程设置已保存': 'Remote settings saved', '所有本地代理已停止': 'All local proxies stopped',
    '已是最新版本': 'You are up to date', '发现新版本，可以在线更新': 'A new version is available for an optional update',
    '正在检查新版本': 'Checking for a new version', '更新正在进行，请稍后': 'An update is already in progress',
    '正在下载并校验更新包，当前代理继续运行': 'Downloading and verifying the update; proxies remain available',
    '正在检查新版本兼容性': 'Checking compatibility with the new version',
    '下载校验完成，后台正在自动重启并恢复节点': 'Update verified; restarting the backend and restoring nodes',
    '在线更新完成，节点已恢复': 'Update complete; nodes have been restored',
    '上次更新已中断，可以重新检查并更新': 'The previous update was interrupted; check again to retry',
    '请使用 Mirroxy API 管理本机代理。基础地址：': 'Use the Mirroxy API to manage local proxies. Base URL: ', '。本机无需认证。': '. Local access needs no authentication. ', '。使用面板账号密码（HTTP Basic）认证。': '. Authenticate with the dashboard username and password (HTTP Basic). ', '先读取 ': 'Read ', '和 ': 'and ', '，再 GET nodes 获取节点 id。默认不要修改系统代理。': ', then GET nodes for node IDs. Do not change the system proxy by default.', '已复制，可直接发给 AI': 'Copied. You can send it directly to an AI tool.',
    '退出 Mirroxy 后台？所有由它提供的代理都会停止。': 'Quit the Mirroxy backend? All proxies it provides will stop.', '后台已退出，使用 launchctl kickstart 可再次启动': 'Backend exited. Use launchctl kickstart to start it again.', '后台已退出。控制台运行 sudo Mirroxyctl start 可再次启动': 'Backend exited. Run sudo Mirroxyctl start to start it again.', '后台已退出。双击 Mirroxy.exe 可再次启动': 'Backend exited. Double-click Mirroxy.exe to start it again.', '后台已退出': 'Backend exited',
    '粘贴节点分享链接或添加订阅，自动识别代理协议。': 'Paste node share links or add a subscription to detect proxy protocols automatically.', '开启自动部署后，导入即可获得独立代理端口。': 'Enable automatic deployment to assign a dedicated proxy port during import.',
    '在节点的「更多」中复制专属 PAC 地址，或在面板上方选择「PAC 模式」并指定节点。默认直连局域网，其余请求通过该节点；大陆地址是否直连由节点出口策略决定。': 'Copy a node-specific PAC URL from More, or select PAC mode and a node above. LAN traffic connects directly; node routing decides whether Mainland China addresses do the same.',
    '本机原生 HTTP 客户端无需登录、无需 token。支持读取节点与流量、导入、启停、联网检测、订阅与系统代理操作。': 'Native local HTTP clients need no sign-in or token. Read nodes and traffic, import, start or stop, run checks, refresh subscriptions, and control the system proxy.',
    '不填写时自动生成自签证书，首次远程打开会提示证书未受信任。公网域名建议填写对应的正式证书。更换协议或证书时请同时选用新管理端口，保存后访问新地址。': 'When empty, a self-signed certificate is generated. Use a trusted certificate for public domains. When changing protocol or certificate, also choose a new management port.',
    '开启后通过所选协议和此设备的 IP:端口访问。此处不会自动修改防火墙或路由器；代理端口仍仅监听本机。Windows 的远程设置只能在本机修改；Linux 管理员可在面板修改。': 'Connect using the selected protocol and this device’s IP and port. Mirroxy does not change firewall or router settings; proxy ports remain local-only. Windows remote settings can only be changed locally.',
    '支持 VMess、VLESS / REALITY、SS、Trojan、SOCKS、HTTP、Hysteria2、TUIC、AnyTLS。': 'Supports VMess, VLESS / REALITY, SS, Trojan, SOCKS, HTTP, Hysteria2, TUIC, and AnyTLS.',
    '复制后可导入 v2rayN 等兼容客户端。链接包含节点认证信息，请只发给需要使用的人。': 'Import the copied link into v2rayN or another compatible client. It contains node credentials; share it only with intended users.', '自定义名称会保留到后续订阅更新，改名立即生效。': 'Custom names persist across subscription refreshes and take effect immediately.',
  });

  Object.assign(en, {
    '游戏 / 应用代理': 'Games / App proxy',
    '选中的程序全部流量走代理；其他程序保持原来的网络方式。': 'Route all traffic from selected apps through a proxy while leaving other apps unchanged.',
    '保存配置': 'Save configuration', '开启应用代理': 'Start app proxy', '停止应用代理': 'Stop app proxy',
    '未开启': 'Off', '应用代理运行中': 'App proxy is running', '正在请求管理员授权': 'Waiting for administrator approval', '正在停止': 'Stopping', '应用代理异常': 'App proxy error',
    '按程序精确选择出口': 'Choose routes per application', '已按应用接管网络': 'Application traffic is routed',
    'TCP 和 UDP 默认全部经过绑定节点，本机与局域网地址保持直连。': 'Traffic not matching direct rules goes through the bound node; splitting follows the node routing policy (such as bypassing Mainland China). Loopback and LAN addresses stay direct.',
    '默认应用节点': 'Default app node', '应用配置选择“跟随默认”时使用这里的节点。': 'Used when an app profile follows the default node.',
    '首次开启会请求 Windows 管理员授权并创建 Mirroxy Apps 虚拟网卡。应用代理默认关闭；不开启时不会创建 TUN、修改路由或影响现有系统代理。': 'Starting for the first time requests Windows administrator approval and creates the Mirroxy Apps virtual adapter. The feature is off by default and does not touch TUN, routes, or the system proxy while off.',
    '应用列表': 'Applications', '一个游戏可以同时加入启动器、游戏本体和反作弊进程。': 'A game profile may include its launcher, game executable, and anti-cheat process.',
    '添加应用': 'Add application', '编辑应用': 'Edit application', '还没有应用配置': 'No application profiles yet', '添加游戏或程序后，再开启应用代理。': 'Add a game or app before starting the app proxy.',
    '请选择节点': 'Select a node', '已启用': 'Enabled', '已停用': 'Disabled', '跟随默认': 'Follow default', '节点已失效': 'Node unavailable', '编辑': 'Edit',
    '跟随默认应用节点': 'Follow default app node', '显示名称': 'Display name', '例如：Valorant': 'Example: Valorant', '绑定节点': 'Bound node',
    '程序路径或名称': 'Executable path or name', '按完整路径匹配最准确，也可以只填写 exe 文件名。': 'A full path is most precise; an exe file name is also supported.',
    '选择正在运行的程序…': 'Choose a running process…', '刷新进程': 'Refresh processes', '加入所选': 'Add selected',
    '启用此配置': 'Enable this profile', '关闭后保留配置但不接管此应用。': 'Keep the profile without routing this app.', '保存应用': 'Save application',
    '请先选择默认应用节点': 'Select a default app node first', '请至少填写一个程序路径或 exe 名称': 'Enter at least one executable path or exe name',
    '应用代理配置已保存': 'App proxy configuration saved', '应用代理已停止': 'App proxy stopped', '应用代理已启动': 'App proxy started', '处理中…': 'Working…',
    '节点游戏网络能力': 'Node gaming capabilities', '真实穿过默认应用节点检测 TCP、UDP、IPv6 和 UDP 映射特征；首次检测可能短暂重启此节点一次。': 'Test TCP, UDP, IPv6, and UDP mapping through the default app node. The first test may briefly restart this node once.',
    '检测默认节点': 'Test default node', '尚未检测': 'Not tested', '支持': 'Supported', '不支持': 'Unsupported', '映射稳定': 'Stable mapping', '对称型倾向': 'Symmetric tendency', '未知': 'Unknown',
    '观察到的 UDP 出口：': 'Observed UDP egress: ', '检测时间：': 'Tested: ', '请先启动默认应用节点': 'Start the default app node first', '能力检测完成：': 'Capability test complete: ',
    '请先启动节点再检测游戏网络能力': 'Start the node before testing gaming capabilities', '节点能力只能从本机面板检测': 'Node capabilities can only be tested from the local dashboard',
    '删除应用配置 {name}？': 'Delete application profile {name}?',
    '应用配置最多 128 个': 'Up to 128 application profiles are supported', '默认应用节点不存在': 'The default app node no longer exists',
    '请先停止应用代理再修改配置': 'Stop the app proxy before editing its configuration', '请至少启用一个应用配置': 'Enable at least one application profile',
    '应用代理目前仅支持 Windows': 'App proxy is currently supported on Windows only', '应用代理只能从本机面板修改': 'App proxy can only be changed from the local dashboard',
    '此节点正被应用代理使用，请先停止应用代理': 'This node is used by the app proxy. Stop app proxy first.', '应用代理正在运行，请在本机面板停止后再全部停止节点': 'App proxy is running. Stop it from the local dashboard before stopping all nodes.',
    '等待管理员授权或虚拟网卡启动超时': 'Timed out waiting for administrator approval or the virtual adapter',
    '管理员授权被取消或启动失败': 'Administrator approval was cancelled or startup failed', '应用路由核心已退出，请重新开启': 'The app-routing core exited. Start it again.'
  });

  Object.assign(en, {
    '指定应用': 'Assign apps', '绑定到 {name} 的应用按它的出口策略分流。': 'Apps bound to {name} follow its routing policy.',
    '还没有应用绑定到此节点，点击「添加应用」并配合目录扫描选择程序。': 'No apps bound to this node yet. Click “Add application” and use the folder scan to pick executables.',
    '个程序': ' programs', '出口策略': 'Routing',
    '例如：C:\\Users\\zzh\\AppData\\Local\\Programs': 'e.g. C:\\Users\\zzh\\AppData\\Local\\Programs', '扫描目录': 'Scan folder', '浏览目录': 'Browse', '扫描目录 exe': 'Scan for exes',
    '没有子目录': 'No subfolders', '主程序': 'Main program', '已选 {count} 个程序': '{count} programs selected',
    '请先勾选要加入的程序': 'Tick at least one program first', '已加入 {count} 个程序，记得保存应用': 'Added {count} programs. Remember to save the app.',
    '只能在本机浏览目录': 'Folders can only be browsed from the local dashboard', '只能在本机扫描程序目录': 'Folders can only be scanned from the local dashboard',
    '请先填写要扫描的目录': 'Enter a folder to scan first', '请填写完整的绝对路径目录': 'Enter an absolute folder path',
    '目录不存在或无法读取': 'The folder does not exist or cannot be read', '请填写目录而不是文件路径': 'Enter a folder, not a file',
    '路径无效': 'Invalid path', '此目录（含最多 3 层子目录）没有找到 .exe 程序': 'No .exe files found in this folder (up to 3 levels deep)',
    '选中的程序按绑定节点的出口策略走代理；其他程序保持原来的网络方式。': 'Selected apps follow the bound node’s routing policy; other apps keep their own networking.',
    '未命中直连规则的流量经过绑定节点；分流跟随绑定节点的出口策略（如绕过大陆），本机与局域网地址始终直连。': 'Traffic not matching direct rules goes through the bound node; splitting follows the node routing policy (such as bypassing Mainland China). Loopback and LAN addresses stay direct.',
    '详情': 'Details', '收起': 'Collapse', '移除': 'Remove', '添加': 'Add', '检测': 'Test',
    '新增程序路径或 exe 名称': 'New program path or exe name', '该程序已在列表中': 'This program is already in the list',
    '已添加，记得保存配置': 'Added. Remember to save the configuration.', '请填写程序路径或 exe 名称': 'Enter a program path or exe name',
    '显示二维码': 'Show QR code', '只看链接': 'Link only', '节点二维码': 'Node QR code',
    '加速器兼容模式': 'Accelerator compatibility mode',
    '关闭 TUN 严格路由，降低与网易 UU、雷神等 TUN 类加速器的路由与防火墙冲突；防泄漏保护会相应减弱。保存配置后，停止并重新开启应用代理生效。': 'Relax TUN strict routing to reduce route and firewall conflicts with TUN-based game accelerators such as NetEase UU and Leiguan; leak protection becomes weaker. Save, then stop and restart the app proxy to apply.',
    '检测到正在运行的疑似加速器：{names}。\n\n两者都会接管游戏流量，可能互相冲突、加速失效或掉线。建议先退出加速器再开启。仍要继续吗？': 'Suspected running game accelerator: {names}.\n\nBoth products hijack game traffic and may conflict, break acceleration, or drop connections. Quit the accelerator first. Start the app proxy anyway?',
    '已取消开启，请先退出加速器': 'Cancelled. Quit the accelerator first.', '只能在本机面板检测加速器冲突': 'Accelerator conflicts can only be checked from the local dashboard',
    '加速器联动': 'Accelerator integration',
    '检测到加速器时自动暂停系统代理': 'Pause the system proxy while an accelerator runs',
    '运行网易 UU、雷神等加速器时自动撤下 Mirroxy 设置的系统代理，退出加速器后自动恢复。手动指向节点端口的自定义代理与其他软件的系统代理不受影响。默认开启。': 'While NetEase UU, Leiguan, or similar accelerators run, the system proxy set by Mirroxy is withdrawn automatically and restored when the accelerator exits. Custom per-app proxy targets and other software’s system proxies stay untouched. Enabled by default.',
    ' · 加速器运行中，已自动暂停': ' · Paused while a game accelerator is running',
    '内核升级': 'Core upgrade', '升级内核': 'Upgrade core', '官方最新版本': 'Official latest:', '尚未检查官方版本': 'Official version not checked yet', ' · 官方最新 ': ' · Official latest ', '进入本页自动查询官方最新版本；升级会短暂重启对应内核，节点与流量记录保留。': 'The official latest version is checked automatically when you open this page. Upgrading briefly restarts the relevant core; nodes and traffic totals are preserved.',
    '绕过局域网和本机地址': 'Bypass LAN and local addresses',
    '局域网与 127.0.0.1 等本机地址始终直连、不经过节点，全局模式下同样生效；关闭后局域网地址也会通过节点代理。默认开启。': 'LAN and loopback addresses such as 127.0.0.1 stay direct — even in Global mode. When off, LAN traffic also goes through the node. Enabled by default.',
    '面板端口': 'Panel port', 'HTTPS 证书': 'HTTPS certificate', '信任根证书': 'Trust root CA', '下载 CA': 'Download CA',
    '本机面板监听端口，默认 9527。保存后自动切换，页面会跳转到新地址。': 'Local dashboard port, 9527 by default. Saving switches over automatically; the page then redirects to the new address.',
    '面板默认使用内置本地 CA 签发的证书（等同于 mkcert 方案）。点击「信任根证书」后，请在 Windows 根证书安全警告中核对名称为 Mirroxy Local CA，再选择「是」；程序会在导入后立即校验。Firefox 使用独立证书库时，请用「下载 CA」手动导入。': 'The panel uses a certificate issued by its built-in local CA (the mkcert approach). After choosing “Trust root CA”, confirm that the Windows trusted-root warning names “Mirroxy Local CA”, then choose Yes; Mirroxy verifies the live HTTPS endpoint after import. Firefox may use a separate store, so import the downloaded CA there if needed.',
    '证书已受信任，浏览器显示安全锁': 'Certificate trusted; browsers show the padlock', '证书已生成，点击「信任根证书」完成安装': 'Certificate issued. Click “Trust root CA” to finish installing it',
    '面板当前为 HTTP：证书未就绪': 'Panel is on HTTP: certificate not ready', '有效期至': 'Valid until',
    '根证书已安装并通过 HTTPS 校验，正在重新建立安全连接…': 'Root CA installed and verified over HTTPS. Re-establishing a secure connection…',
    '面板端口已变更，正在跳转到新地址…': 'Panel port changed; redirecting to the new address…',
    '只能在本机下载根证书': 'The root CA can only be downloaded from the local dashboard', '只能在本机查看证书状态': 'Certificate status is only available locally', '只能在本机安装根证书': 'The root CA can only be installed from the local dashboard',
    '根证书尚未生成，请先在设置中生成或开启 HTTPS 面板': 'The root CA has not been generated yet', '面板服务尚未就绪': 'The panel server is not ready yet', '面板端口正在切换，请稍候': 'A panel port switch is already in progress',
    '根证书已导入，但 HTTPS 校验仍未通过；请完全退出浏览器后重试': 'The root CA was imported, but live HTTPS verification still failed. Fully close the browser and try again',
    '二维码只能在本机面板生成': 'The QR code can only be generated from the local dashboard', '节点不存在': 'Node not found', '二维码生成失败：': 'QR code generation failed: '
  });

  Object.assign(en, {
    'Geo 文件': 'Geo files', 'Geo 规则文件': 'Geo rule files',
    '分流规则使用的 IP 归属与域名分类数据库，在这里查看状态并一键更新。': 'IP geolocation and domain category databases used by routing rules. Check their status and update them here.',
    '检查远程版本': 'Check remote version', '更新全部': 'Update all',
    '更新时从规则源下载最新文件（自动优先经运行中的节点代理），替换前校验版本与内容，完成后对应内核会短暂重启，节点与流量记录保留。': 'Updates download the latest files from the rule sources (preferring a running node as proxy), verify version and content before replacing, and briefly restart the relevant core afterwards. Nodes and traffic totals are preserved.',
    'Xray 路由数据库（网络核心 A）': 'Xray routing databases (Network Core A)',
    '进入本页自动查询规则源最新版本。': 'The latest source version is checked automatically when you open this page.',
    '规则源最新版本：': 'Latest source version: ',
    'sing-box 规则集（网络核心 B）': 'sing-box rule sets (Network Core B)',
    '网络核心 B 的分流规则使用这些本地规则集；缺失或过期会直接影响绕过大陆与黑名单模式的准确性。': 'Core B routing uses these local rule sets; missing or outdated files directly affect the accuracy of Bypass Mainland China and Blacklist modes.',
    '每日自动更新 Geo 规则文件': 'Update Geo rule files daily',
    '规则源有新版本时自动下载并替换；更新会短暂重启对应内核。默认关闭，也可在「Geo 文件」页手动更新': 'Downloads and replaces files automatically when the rule source publishes a new version; the relevant core restarts briefly. Off by default; manual updates live on the Geo files page.',
    '已就绪': 'Ready', '缺失': 'Missing', '文件': 'File', '大小': 'Size', '更新时间': 'Updated',
    '文件尚未就绪': 'File not in place yet', '规则版本': 'Rule version', '内置快照': 'Bundled snapshot', '更新': 'Update',
    '正在下载并更新 Geo 规则文件，完成后对应内核会短暂重启…': 'Downloading and updating Geo rule files; the relevant core restarts briefly afterwards…',
    'Geo 规则文件已更新': 'Geo rule files updated',
    '正在更新全部 Geo 规则文件，请稍候…': 'Updating all Geo rule files, please wait…',
    'Geo 规则文件已全部更新': 'All Geo rule files updated',
    '有 {count} 个规则文件缺失，点击「更新全部」可自动补齐。': '{count} rule files are missing. Click “Update all” to restore them.',
    'IP 归属数据库：局域网与大陆直连、黑名单 IP 组判断的底层数据': 'IP geolocation database: underlying data for LAN/Mainland direct rules and blacklist IP groups',
    '域名分类数据库：局域网与大陆直连、黑名单域名组判断的底层数据': 'Domain category database: underlying data for LAN/Mainland direct rules and blacklist domain groups',
    '大陆 IP 段：绕过大陆模式下直连': 'Mainland China IP ranges: direct in Bypass Mainland China mode',
    '大陆域名：绕过大陆模式下直连': 'Mainland China domains: direct in Bypass Mainland China mode',
    '局域网与内网域名：各模式下直连': 'LAN and private domains: direct in every mode',
    'GFW 列表：黑名单模式下走代理': 'GFW list: proxied in Blacklist mode',
    'GreatFire 列表：黑名单模式下走代理': 'GreatFire list: proxied in Blacklist mode',
    'Google 域名：黑名单模式下走代理': 'Google domains: proxied in Blacklist mode',
    'Facebook IP 段：黑名单模式下走代理': 'Facebook IP ranges: proxied in Blacklist mode',
    'Fastly IP 段：黑名单模式下走代理': 'Fastly IP ranges: proxied in Blacklist mode',
    'Google IP 段：黑名单模式下走代理': 'Google IP ranges: proxied in Blacklist mode',
    'Netflix IP 段：黑名单模式下走代理': 'Netflix IP ranges: proxied in Blacklist mode',
    'Telegram IP 段：黑名单模式下走代理': 'Telegram IP ranges: proxied in Blacklist mode',
    'Twitter IP 段：黑名单模式下走代理': 'Twitter IP ranges: proxied in Blacklist mode'
  });

  Object.assign(en, {
    '下载任务右键菜单':'Download context menu','复制下载链接':'Copy download URL','复制保存路径':'Copy file path','保留已下载文件和断点数据':'Keeps downloaded files and resume data','全选':'Select all',
    '删除下载任务':'Remove downloads','关闭删除弹窗':'Close removal dialog','同时删除下载文件和断点数据':'Also remove downloaded files and resume data','即将删除':'Removing','个下载任务':'downloads','删除记录及文件':'Remove records and files','取消':'Cancel',
    '移入保存目录的 .Mirroxy-trash 回收区，可手动恢复。':'Moves files into .Mirroxy-trash in the destination folder; they can be restored manually.',
    '默认只删除任务记录，保留所有已下载文件。勾选时只处理该任务关联的文件，不删除目录。':'By default only records are removed. Opting in handles only the task’s associated files, never directories.',
    '文件将从原保存位置移入回收区。':'Files will move from their original location into the recovery area.',
    '下载管理':'Downloads','新建下载':'New download','继续':'Resume','暂停':'Pause','编辑':'Edit','重试':'Retry','删除记录':'Remove records','全部暂停':'Pause all','刷新':'Refresh',
    '文件名':'File name','状态 / 进度':'Status / progress','速度':'Speed','剩余时间':'Time left','连接':'Connections','代理':'Proxy','操作':'Actions',
    '下载中':'Downloading','等待中':'Queued','已暂停':'Paused','已完成':'Completed','失败':'Failed','已移除':'Removed','直连':'Direct','自定义代理':'Custom proxy','节点已移除':'Node removed',
    '全部任务':'All tasks','下载中 / 等待中':'Downloading / queued','未选择任务':'No selection','已选':'Selected','项':'items','个任务':'tasks',
    '下载地址':'Download URL','文件名（留空自动识别）':'File name (optional)','保存目录':'Destination folder','代理方式':'Proxy mode','代理地址':'Proxy URL','代理节点':'Proxy node',
    '直连（不使用系统代理）':'Direct (ignore system proxy)','Mirroxy 节点':'Mirroxy node','自定义 HTTP 代理':'Custom HTTP proxy','每任务最大连接数':'Connections per task','限速（KiB/s；0 不限速）':'Limit (KiB/s; 0 = unlimited)',
    '仅添加，稍后下载':'Add paused','开始下载':'Start download','编辑下载任务':'Edit download','保存修改':'Save changes','关闭下载弹窗':'Close download dialog',
    'aria2 运行中':'aria2 running','aria2 待命':'aria2 idle','aria2 未安装':'aria2 not installed','引擎待命':'Engine idle','安装 aria2 引擎':'Install aria2 engine','正在下载并校验引擎…':'Downloading and verifying engine…',
    '搜索下载任务':'Search downloads','搜索文件名或链接':'Search file name or URL','任务状态':'Task status','选择当前列表全部任务':'Select visible tasks','下载工具栏':'Download toolbar',
    '保存位置':'Saved to','已下载':'Downloaded','错误':'Error','添加你的第一个下载':'Add your first download','没有匹配的任务':'No matching tasks','试试其他状态或搜索词。':'Try another status or search.',
    '熟悉的任务列表，独立的下载代理。关闭网页，下载继续。':'A familiar task list with independent download proxies. Downloads continue when the page closes.',
    '下载到运行 Mirroxy 的这台电脑。':'Downloads are saved on the computer running Mirroxy.',
    '粘贴链接，选择代理和连接数，即可开始下载。':'Paste a URL and choose a proxy and connection count.',
    '支持 HTTP / HTTPS / FTP':'Supports HTTP / HTTPS / FTP',
    '默认只删除任务记录；勾选删除文件时移入可恢复的回收区。多连接需要服务器支持 Range；节点代理遵循该节点的出口策略。':'By default only task records are removed; optionally move files to a recoverable trash area. Multiple connections require Range support; node proxies follow the node exit policy.',
    '最多同时下载 30 个任务 · 每任务最多 16 个连接':'Up to 30 concurrent tasks · up to 16 connections each',
    '打开文件':'Open file','打开所在文件夹':'Show in folder','断点重连':'Reconnect from saved progress',
    '暂无速度，可断点重连':'No data; reconnect available',
    '已连续 45 秒没有下载进度，可保留断点重连。':'No progress for 45 seconds. Reconnect while keeping downloaded pieces.',
    '只删除所选下载记录，保留已下载文件和断点数据。继续吗？':'Remove selected records but keep downloaded files and resume data?',
    '编辑任务前请先暂停。文件名和目录保持不变；更换链接时必须指向相同文件，避免损坏断点数据。保存后继续保持暂停，点击「继续」开始。':'Pause before editing. File name and destination are retained. Replacement URLs must refer to the same file to preserve partial data. Resume after saving.',
    '只显示已启动节点。不会自动启动节点，也不会在代理故障时切换为直连。自定义代理使用 HTTP，可填 http://用户名:密码@主机:端口。':'Only running nodes are offered. Nodes are not started automatically, and proxy failures never fall back to direct access. Custom proxies use HTTP and may include username/password.',
    '选择已启动的节点':'Choose a running node'
  });
  function browserLocale() {
    const values = navigator.languages?.length ? navigator.languages : [navigator.language || 'en'];
    return values.some(value => String(value).toLowerCase().startsWith('zh')) ? 'zh-CN' : 'en';
  }
  let preference = 'auto';
  try { preference = localStorage.getItem(storageKey) || 'auto'; } catch {}
  const locale = preference === 'zh-CN' || preference === 'en' ? preference : browserLocale();
  function t(key, params) {
    const table = locale === 'en' ? en : zh;
    return String(table[key] ?? key).replace(/\{(\w+)\}/g, (match, name) => Object.prototype.hasOwnProperty.call(params || {}, name) ? params[name] : match);
  }
  function translateTree(root) {
    if (locale !== 'en') return;
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    const nodes = [];
    while (walker.nextNode()) nodes.push(walker.currentNode);
    nodes.forEach(node => {
      const source = node.nodeValue;
      const trimmed = source.trim();
      const translated = t(trimmed);
      if (trimmed && translated !== trimmed) node.nodeValue = source.replace(trimmed, translated);
    });
    root.querySelectorAll?.('[placeholder],[aria-label],[title]').forEach(element => {
      for (const name of ['placeholder', 'aria-label', 'title']) if (element.hasAttribute(name)) element.setAttribute(name, t(element.getAttribute(name)));
    });
  }
  function initializeSelector() {
    const select = document.getElementById('languageSelect');
    if (!select) return;
    select.value = preference;
    select.addEventListener('change', () => {
      try { localStorage.setItem(storageKey, select.value); } catch {}
      location.reload();
    });
  }
  document.documentElement.lang = locale;
  document.title = t('Mirroxy · 本地代理面板');
  window.MirroxyI18n = { locale, preference, t, translateTree };
  document.addEventListener('DOMContentLoaded', () => { translateTree(document.body); initializeSelector(); });
})();
