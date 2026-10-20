# Mirroxy Android · 0.1.0-dev

独立 Android 代理管理器，Kotlin + Jetpack Compose + XTLS 官方 Android libXray。最低 Android 8.0（API 26）。

在本目录运行 `scripts/build.ps1`，产物为 `app/build/outputs/apk/debug/app-debug.apk`。完整中文说明和验证边界位于 `D:\Mirroxy\docs\ANDROID-BUILD-TEST.md` 与 `ANDROID-QA.md`。

当前支持单运行节点、真实 HTTP/SOCKS 回环代理、节点导入/配置编辑/改名/确认删除、Keystore 加密保存、前台服务。VPN 接入和应用白名单已实现，系统授权后的路由测试尚待完成。订阅 URL 管理、流量统计、多节点并行、下载管理为后续工作。

本目录不引用桌面 data、正式签名或真实订阅；没有自动启动 VPN、开机启动或下载后自动运行。第三方许可见 `THIRD-PARTY.md`。
