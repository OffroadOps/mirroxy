# Mirroxy 浏览器下载接力 / Browser Download Relay

让 Chrome / Edge 的下载自动交给本机 Mirroxy 的下载管理器（aria2），并在 Mirroxy 成功接单后才取消浏览器下载。接单失败（Mirroxy 未运行、未配置、引擎异常）时保留浏览器下载，不会丢文件。

## 安装

1. 在 Mirroxy 面板打开「全局设置」，在「浏览器接力」卡片点击「下载扩展 ZIP」。
2. 解压 ZIP 到固定目录（例如 `D:\Mirroxy-extension`）。
3. Chrome / Edge 打开 `chrome://extensions`（Edge 为 `edge://extensions`），开启「开发者模式」。
4. 点击「加载已解压的扩展程序」，选择解压后的目录。

**不需要复制令牌。** 扩展会通过本机 15060 端口自动发现正在运行的 Mirroxy 并完成配对；面板端口变化后会自动重新配对。若提示未连接，先启动 Mirroxy 后台，再点浏览器图标 →「重新连接」。

只有自动连接失败（例如 15060 被其他程序占用）时，才需要在 Mirroxy 面板点「复制扩展配置（备用）」，在扩展设置的「手动配置」中导入。

## 使用

- 自动接管：浏览器发起新的 HTTP/HTTPS/FTP 下载时，扩展先把 URL、Referer、Cookie 和 User-Agent 交给 Mirroxy；接单成功后取消浏览器下载，任务出现在 Mirroxy「下载管理」窗口（托盘菜单打开）。
- 手动接管：在链接上右键选择「使用 Mirroxy 下载此链接」；在页面空白处右键选择「使用 Mirroxy 下载当前页面」。
- 排除网站：扩展设置里填写不接管的域名（每行一个）。
- 总开关 / 自动接管开关在工具栏图标弹窗中随时切换。

## 限制

- 仅支持 Chrome / Edge 等 Chromium 浏览器（MV3）。
- 扩展需要访问你下载的站点，才能读取该站点的 Cookie 用于登录态下载；浏览器首次加载扩展时会询问权限，请允许。
- 登录态由浏览器 Cookie 自动转发；极少数需要额外鉴权头或 DRM 的站点仍可能失败，可加入「不接管的网站」。
- 浏览器自身会在事件触发后短暂开始下载再被取消，通常不会留下完整文件；极小的文件可能已经完成，此时不会重复转交。
- 会为当前下载域转发浏览器 Cookie、Referer 与 User-Agent 给本机 Mirroxy，用于登录/防盗链下载；不会发送到任何外部服务器。
- 扩展通过本机回环地址与 Mirroxy 通信；自动配对使用本机 15060 端口，无需信任本地证书。
- 如果浏览器提示没有权限，扩展会通知「未读取到浏览器 Cookie」，此时需要登录的下载会失败。

## English

Install the unpacked extension; it finds Mirroxy automatically through the local 15060 port, with no token copying and no certificate setup. New downloads are relayed to Mirroxy and the browser copy is cancelled only after Mirroxy accepts the task. Failures leave the browser download untouched. Referer, cookies and User-Agent are forwarded to the local Mirroxy for signed-in downloads.

The extension asks for access to all sites so it can read the cookies of the page you download from; Chrome refuses to return cookies for sites the extension has no permission for. Allow the permission when the browser asks, otherwise signed-in downloads fall back to an anonymous request and the extension reports that it could not read the cookies.

