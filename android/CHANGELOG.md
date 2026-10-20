# Android 变更记录

## 0.1.0-dev · 2026-10-13

- Kotlin + Compose 原生节点界面和应用路由选择器。
- 集成官方 Android libXray 26.10.10，HTTP/SOCKS 回环代理和真实请求检测。
- 分享链接、Base64 内容和 outbounds JSON 导入，去重、改名、配置编辑、确认删除、固定端口；兼容旧 servers/vnext 单节点格式。
- Android Keystore AES-GCM 加密节点记录，AtomicFile 原子写入，关闭备份。
- 独立本地代理前台服务、显式 VPN 授权、IPv4/IPv6 TUN、socket protect、DNS 保护、分应用白名单。
- 开发阶段每次运行一个节点；订阅定时刷新、累计流量、下载管理后续实现。
