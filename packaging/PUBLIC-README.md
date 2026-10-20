# Mirroxy

多节点同时运行，每个节点一个固定的本地 HTTP / SOCKS5 端口，通过 Web 面板管理。

**[公开发布页：四平台下载、历史版本与更新日志](https://ny-1s.enzonix.com/bucket-1776-2431/scrcpyproxy/index.html)**

## Linux 一键安装

适用于 Armbian / Debian / Ubuntu 的 systemd 环境，自动识别 ARM64 或 AMD64：

```bash
curl -fsSL --retry 3 https://ny-1s.enzonix.com/bucket-1776-2431/scrcpyproxy/install.sh | sudo bash
```

安装包预置两个网络核心、路由资源和 CA 证书。安装完成后显示远程 HTTP 面板地址、admin
账号与随机密码，并启用开机自启。也可从发布页下载 .run 文件进行完全离线安装。

## Windows 与 macOS

- Windows x86_64：运行 `Mirroxy-windows-amd64.exe` 安装程序；安装器按系统语言显示中文或 English。同版本重复运行不修改现有安装，更高版本原位升级并保留 `data`。
- macOS ARM64：解压后以普通用户运行 `bash ./install-macos.sh`。当前完整包要求 macOS 26+，未公证且尚未实机验证。

本机面板为 `http://127.0.0.1:15060/`，无需登录。远程管理必须使用账号密码。
Windows 默认不改变系统代理，可继续让 v2rayN 使用 `127.0.0.1:10808`。

Windows 0.6.0 新增网页控制的游戏 / 应用代理。选中的 `.exe` 全部 TCP/UDP 走绑定节点，
未选应用保持原网络方式；功能默认关闭，仅在开启时请求管理员权限创建 TUN。页面可穿过节点
检测 TCP、UDP、IPv6 和 UDP 映射特征。

进入面板后粘贴分享链接或订阅，程序会选择适合的核心并从 5060 起分配端口。节点可分别启停、
测试、改名、分享、统计连接与流量，并选择绕过大陆、黑名单或全局路由。

## 0.5.0 升级说明

产品已从旧名称迁移为 Mirroxy。核心、路由文件、临时配置和日志改用 Mirroxy 通用名称，
今后的在线核心更新也保持这些名称。真实上游来源与许可证仍保存在发布包的 NOTICE 中。

节点、订阅、远程凭据和累计流量写入 AES-GCM 加密的 SQLite 数据库。Windows 密钥由当前用户
DPAPI 保护；Linux/macOS 密钥依靠本机文件权限保护。

0.4.x 到 0.5.0 需要执行一次完整包安装。0.5.0 之后可在“更新日志”页面在线更新；Windows
由独立 `upgrade.exe` 替换并验证程序，失败时自动回滚。更新切换会短暂重启代理。

本仓库保持私有，仅发布二进制、安装脚本和说明。发布物不包含应用源码、用户节点、数据库、
密钥、日志或存储凭据。下载后可使用 `SHA256SUMS` 校验。
