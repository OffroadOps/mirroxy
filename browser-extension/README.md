# Mirroxy 浏览器下载接力 / Browser Download Relay

让 Chrome / Edge 的下载自动交给本机 Mirroxy 的下载管理器（aria2），并在 Mirroxy 成功接单后才取消浏览器下载。接单失败（Mirroxy 未运行、未配置、引擎异常）时保留浏览器下载，不会丢文件。

## 安装

1. 在 Mirroxy 面板打开「下载管理」，在「浏览器接力」卡片点击「下载扩展 ZIP」。
2. 解压 ZIP 到固定目录（例如 `D:\Mirroxy-extension`）。
3. Chrome / Edge 打开 `chrome://extensions`（Edge 为 `edge://extensions`），开启「开发者模式」。
4. 点击「加载已解压的扩展程序」，选择解压后的目录。
5. 回到 Mirroxy「下载管理」，点击「复制扩展配置」。
6. 点击浏览器工具栏的 Mirroxy 图标 →「扩展设置」，粘贴配置 JSON →「导入配置」→「保存」→「测试连接」。

> 面板使用本地 HTTPS 自签证书时，请先在面板页首点击「信任根证书」，否则扩展会提示证书错误。

## 使用

- 自动接管：浏览器发起新的 HTTP/HTTPS/FTP 下载时，扩展先把 URL 交给 Mirroxy；接单成功后取消浏览器下载，任务出现在 Mirroxy「下载管理」。
- 手动接管：在链接上右键选择「使用 Mirroxy 下载此链接」；在页面空白处右键选择「使用 Mirroxy 下载当前页面」。
- 排除网站：扩展设置里填写不接管的域名（每行一个）。
- 总开关 / 自动接管开关在 ⓘ 图标弹窗中随时切换。

## 限制

- 仅支持 Chrome / Edge 等 Chromium 浏览器（MV3）。
- 带登录态、依赖浏览器 Cookie 的下载可能无法被 Mirroxy 继续；请把该站点加入「不接管的网站」，或使用浏览器原下载。
- 浏览器自身会在事件触发后短暂开始下载再被取消，通常不会留下完整文件；极小的文件可能已经完成，此时不会重复转交。
- 不读取、不转发浏览器 Cookie 或密码；只发送下载 URL 与文件名。
- 扩展通过本机回环地址与 Mirroxy 通信，令牌仅保存在浏览器扩展本地存储。

## English

Install the unpacked extension, copy the connector config from Mirroxy → Downloads, paste it in the extension options, then test the connection. New downloads are relayed to Mirroxy and the browser copy is cancelled only after Mirroxy accepts the task. Failures leave the browser download untouched. Cookie-authenticated downloads may need the site added to the skip list.
