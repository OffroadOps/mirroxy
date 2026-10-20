Mirroxy 0.6.0 — Armbian / Debian / Ubuntu

支持 Linux ARM64（aarch64）和 AMD64（x86_64），要求 systemd。
离线包已经预置网络核心 A/B、路由数据和 CA 证书，安装时无需联网。

安装 ARM64：
  sudo bash ./Mirroxy-linux-arm64.run
安装 AMD64：
  sudo bash ./Mirroxy-linux-amd64.run
可指定显示地址和远程端口：
  sudo bash ./Mirroxy-linux-arm64.run --host 你的IP或域名 --port 15061

安装目录：/opt/mirroxy
服务名称：mirroxy
控制命令：
  sudo mirroxyctl info
  sudo mirroxyctl status
  sudo mirroxyctl logs
  sudo mirroxyctl restart
  sudo mirroxyctl password
  sudo mirroxyctl enable
  sudo mirroxyctl disable

远程 HTTP 面板默认为 http://机器IP:15061/，使用安装时生成的 admin 账号和随机密码。
本机 API 为 http://127.0.0.1:15060/api/v1/，本机无需认证。
代理端口只监听 127.0.0.1，从 5060 起分配，HTTP / SOCKS5 共用。例如：
  curl -x http://127.0.0.1:5060 https://example.com

进入面板粘贴分享链接或订阅后，会自动选择核心、分配端口并启动；可在全局设置关闭。
Linux 不修改系统代理。重新安装新完整包会保留节点、端口、账号和累计流量。

服务器出口能力只读检测：
  sudo -u mirroxy /opt/mirroxy/Mirroxy -dir /opt/mirroxy -probe-server
输出 JSON，包括 TCP、UDP、IPv6、STUN 映射特征和两个网络核心版本；不会修改配置或防火墙。
旧 /opt/scrcpyproxy 安装会被检测：数据复制到新目录，旧服务在新服务就绪后停用。

数据保存在 data/Mirroxy.db 并用 AES-GCM 加密；data/Mirroxy.key 是仅限 mirroxy 用户读取的
本机密钥。Linux 管理员仍可读取数据库和密钥，因此它用于避免普通文件扫描直接看到节点明文，
不等同于抵抗 root。请把数据库和密钥一起备份。

从 0.5.0 起可在面板在线更新。Linux 通过原 PID exec 切换程序，以保持 systemd 监管；
更新会短暂重启代理并恢复启用节点。完整包和在线更新包均不包含用户数据或应用源码。
