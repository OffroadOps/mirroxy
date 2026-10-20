# Android QA · 2026-10-13

## 已通过

- Kotlin + Compose debug APK 和 Android instrumentation 测试 APK 编译成功。
- lintDebug 成功，无错误（8 warnings）。
- 官方 libXray 发布 ZIP SHA-256 与 GitHub asset digest 相符；APK v2 签名及 16 KiB ZIP 对齐检查通过。
- realme RMP2501 真机平板，arm64：`OK (4 tests)`。
- MuMu x86_64 模拟器，Android 15 / API 35：`OK (4 tests)`。
- 四项内容为 Shadowsocks 加密链路上 HTTP/SOCKS 真实请求、加密持久化及编辑/改名/固定端口、分享导入、关闭界面后的前台服务与真实 HTTP 上游请求。

测试完全使用隔离生成的密码、节点、回环端口和 origin；不是桌面真实节点。单次链路测试仅证明本机隔离核心通信，未验证真实远端订阅/节点。

## VPN 待确认

可选 `VpnIntegrationTest` 已运行到 Android 系统授权步骤。120 秒内未获得授权，返回：`Human VPN consent was not granted within 120 seconds`。未启动 VPN、没有进行 IPv4/IPv6 TUN 请求断言，不能报告 VPN 路由通过。

测试 finally 已恢复应用白名单、删除生成的测试节点并关闭测试上游。需要用户在系统 VPN 对话框手动授权后重跑；代码准备接管测试应用自己的 UID，不接管其他应用。DNS、UDP、断网恢复、网络切换、always-on 与不同厂商系统仍需后续专项测试。

## 设备识别纠正

USB serial `LRCYTSAIEQNRMROJ` 实际为 realme RMP2501 平板；本次创建的 app/test 包已卸载。无线 `127.0.0.1:16384` 是本机 MuMu 模拟器，虽然报告 Redmi/23113RKC6C 型号，但 ABI 为 x86_64，不能当作 Redmi 真机。开发 APK 目前安装在 MuMu。

## 界面和使用

已检查 Compose 节点首页、空列表、通知授权、原生节点按钮及应用启动。未自动启动设备 VPN。所有服务在测试结束时停止。没有打开或运行下载 APK，未更改桌面系统代理或读取私有配置。

构建和重测命令见 `ANDROID-BUILD-TEST.md`；Android 后续功能范围见 `android/CHANGELOG.md`。
