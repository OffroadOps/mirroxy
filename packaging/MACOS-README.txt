Mirroxy 0.6.0 — macOS ARM64

预置网络核心 B 的当前官方 ARM64 构建要求 macOS 26 或更高版本，安装器会提前检查。
本包未经过 Apple Developer ID 公证，也尚未在真实 Mac 上验证。

解压后以当前登录用户运行：
  bash ./install-macos.sh
也可双击 install.command。不要使用 sudo。

安装目录：~/Library/Application Support/Mirroxy
LaunchAgent：com.offroadops.mirroxy
本机面板：http://127.0.0.1:15060/，无需登录。
代理端口从 5060 起分配，HTTP / SOCKS5 共用。macOS 版不修改系统代理。

安装器会从旧 ~/Library/Application Support/scrcpyproxy 复制旧版 JSON 数据并迁移。
节点、订阅、远程凭据和累计流量保存到 AES-GCM 加密的 data/Mirroxy.db；
data/Mirroxy.key 仅供当前用户读取。拥有该用户文件权限的人仍可同时读取密钥与数据库。

从 0.5.0 起可在面板在线更新。macOS 使用原 PID exec 保持 LaunchAgent 监管，切换时会短暂重启。
发布包包含 ARM64 主程序、预置核心、路由数据、证书和许可证，不包含用户数据或源代码。
