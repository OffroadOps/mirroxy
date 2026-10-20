Mirroxy 0.8.0

下载管理：Windows 完整包预置 aria2 1.37.0；从旧版在线更新后，下载页可点击「安装 aria2 引擎」从官方源下载并校验。Linux/macOS 需先从可信系统包管理器安装 aria2（aria2c）。下载管理目前仅本机面板可用，远程监听不开放任务和文件路径接口。其他平台未宣称 aria2 实机验证通过。

Windows 托盘菜单会显示当前版本；仅在检测到更高版本时提供“更新到 v…（可选）”，
用户点击并确认后才开始安装，不会强制更新。

本版完成产品更名并启用全新图标。安装目录中的主程序、核心、路由资源、临时配置与日志
统一使用 Mirroxy 名称；面板在线更新核心后仍写入固定的 Mirroxy_core_A/B 文件。

节点、订阅、远程凭据、累计流量和系统代理恢复信息迁移到 AES-GCM 加密 SQLite 数据库。
Windows 密钥由当前用户 DPAPI 保护；Linux/macOS 使用权限为 0600 的本机密钥文件。
迁移成功后旧版 settings.json、traffic.json 和旧运行配置会被删除。

Windows 使用固定 Mirroxy-online-setup.exe（仅编译一次）拉取服务端最新完整 ZIP，校验后安装；
可填写 HTTP/HTTPS 下载代理，不修改系统代理。同版本不替换、拒绝降级；更高版本原位升级，
保留 data、面板端口与已有核心。下载失败不替换现有程序。历史离线 EXE 仍可使用。
包内仍含独立 upgrade.exe。下载页新增右键菜单、页面内删除浮窗；默认不删文件，勾选时移入
保存目录的 .Mirroxy-trash 回收区，可按 restore.json 恢复，不立即释放磁盘空间。
0.4.x 到 0.5.0 因主程序及更新协议改名，需要使用一次完整包；
从 0.5.0 起可在面板内更新，升级失败或候选程序未就绪时恢复旧版。更新会短暂重启连接。

公开发布页（保留全部历史版本）：
https://ny-1s.enzonix.com/bucket-1776-2431/scrcpyproxy/index.html

完整包：
  Mirroxy-windows-amd64.zip（完整程序载荷）
  Mirroxy-online-setup.exe（固定在线安装器）
  Mirroxy-windows-amd64.exe（按需构建的离线安装器）
  Mirroxy-linux-arm64.run / Mirroxy-linux-arm64.tar.gz
  Mirroxy-linux-amd64.run / Mirroxy-linux-amd64.tar.gz
  Mirroxy-macos-arm64.zip

程序在线更新包：
  Mirroxy-update-windows-amd64.zip
  Mirroxy-update-linux-arm64.zip
  Mirroxy-update-linux-amd64.zip
  Mirroxy-update-macos-arm64.zip

所有完整包均预置对应平台的两个网络核心、路由数据和 CA 证书。真实上游项目名称、版本、
下载地址和许可证保留在包内 licenses/NOTICE.txt 与 manifest.json，以满足来源和许可证要求。
macOS ARM64 包要求 macOS 26+，未公证且尚未实机验证。Linux 面向 systemd 环境。

默认不改变 Windows 系统代理，可继续与 v2rayN 同时使用。发布物不包含应用源码、用户节点、
数据库、密钥、日志或发布存储凭据。使用 SHA256SUMS 校验下载文件。
