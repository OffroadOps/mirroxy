# Mirroxy Android 构建与验证

2026-10-13 · Android 0.1.0-dev · 桌面仍为 0.7.4。

## 工程与工具链

源码、Gradle Kotlin DSL、Android 资产和测试全部位于 `D:\Mirroxy\android`。没有读取桌面 data、真实节点、订阅或平台存储密钥，也没有停止或替换桌面运行程序。

- SDK：`D:\DevTools\Android\Sdk`，compileSdk/targetSdk 36、minSdk 26。
- JDK：`D:\DevTools\jdk-21`，Temurin 21.0.12.1；应用 JVM 字节码目标 17。
- Gradle 8.12.1、AGP 8.10.1、Kotlin 与 Compose compiler 2.2.20。
- Compose BOM 2025.08.00、Activity Compose 1.10.1。
- 核心：XTLS 官方 libXray 26.10.10 Android AAR，未修改；MIT 包装层与 MPL-2.0 Xray。固定下载地址、许可证和摘要见 `android/THIRD-PARTY.md`。

官方兼容资料：

- [AGP 8.10 支持 API 36，最低 Gradle 8.11.1、JDK 17](https://developer.android.com/build/releases/agp-8-10-0-release-notes)
- [Kotlin 2.2.20 支持 Gradle 至 8.14、AGP 至 8.11.1](https://kotlinlang.org/docs/gradle-configure-project.html)
- [Gradle 8.12.1 的 Java 运行兼容范围](https://docs.gradle.org/8.12.1/userguide/compatibility.html)
- [Compose compiler 与 Kotlin 使用相同版本插件](https://kotlinlang.org/docs/compose-compiler-migration-guide.html)

## 构建

PowerShell：

```powershell
Set-Location D:\Mirroxy\android
.\scripts\build.ps1
```

构建脚本使用本机 SDK/JDK，Gradle 用户缓存固定在 `D:\DevTools\gradle-home`，必要时通过 `scripts/fetch-core.ps1` 获取固定官方核心，并校验 SHA-256。已将现有机器缓存复制用于首次初始化，原缓存保留。新机器需先在 `local.properties` 设置 sdk.dir。Gradle wrapper 固定版本并包含官方分发 ZIP 摘要。

依赖下载失败时，在本次终端设置用户提供的 HTTP_PROXY / HTTPS_PROXY / ALL_PROXY；Gradle 的 JVM 下载若不读取环境变量，可显式添加 `-Dhttps.proxyHost=127.0.0.1 -Dhttps.proxyPort=10808 -Dhttp.proxyHost=127.0.0.1 -Dhttp.proxyPort=10808`。不修改系统代理。

产物：

- `android/app/build/outputs/apk/debug/app-debug.apk`
- `android/app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk`
- `android/app/build/reports/lint-results-debug.html`

当前是包含多个 ABI 的通用 debug APK，使用 Android 工具默认 debug 签名。没有生成正式发布签名或上传源码/包。已检查 APK v2 签名和 16 KiB ZIP 对齐。

## 已实现

原生 Compose 节点界面，分享链接/Base64 内容/文件/单服务器 Xray outbounds JSON 导入，旧 servers/vnext 格式转换，去重、改名、配置编辑、确认删除。名称和固定端口写入 Android Keystore AES-GCM 加密文件，AtomicFile 原子落盘；禁用云备份和设备迁移备份。

每次运行一个节点，HTTP 在节点固定回环端口、SOCKS 在相邻端口，默认从 15060/15061 分配；两种本地协议均由真实原生核心处理。启动本地模式不启动 VPN；关闭界面不停止前台代理；通知提供停止操作。不静默换节点、不回退直连。进程被系统终止后显示停止，用户需手动重新启动。

VpnService 使用系统 prepare 授权、原生 TUN fd、IPv4/IPv6 地址及路由、核心出站 socket protect 和独立 DNS socket 保护。应用路由为允许应用白名单；空白名单为全设备。修改列表后下次启动生效。现阶段无自动启动、always-on/锁定模式保证或完整网络切换恢复验证。

## 测试证据和边界

在 realme RMP2501 平板（arm64，API 36）和 MuMu 模拟器（x86_64，API 35）各执行四项测试，均 `OK (4 tests)`。MuMu 报告的 Redmi 型号属于模拟设备标识，不能据此称为 Redmi 真机。平板上任务安装的应用和测试包已卸载，MuMu 保留开发 APK。

1. 原生 Shadowsocks AES-128-GCM 客户端与隔离服务端，HTTP 和 SOCKS 请求均返回独立测试 HTTP origin 的精确响应。不是 UI mock；此证据验证隔离加密链路，不代表真实用户订阅已联网。
2. 加密持久化、读取、改名、配置替换、固定端口保留、去重与删除；磁盘文件不包含测试名称明文。
3. VLESS、Trojan、SS、SOCKS 官方分享解析以及 Kotlin HTTP 分享解析，Base64 批量内容。
4. 前台 ProxyService 真实 HTTP 上游请求，关闭 Activity 后服务继续代理，停止后状态清理。测试节点按 ID 清理，其他节点不动。

`assembleDebug`、`assembleDebugAndroidTest`、`lintDebug` 成功，lint 无 error。warning 包含固定依赖可升级提示、KTX 建议、平台备份弃用提示和 Intent 被错误识别为 SAM 的提示，不作为功能通过证据。

可重复执行（必须指定 adb serial，停止当前 Mirroxy 服务）：

```powershell
$adb = 'D:\DevTools\Android\Sdk\platform-tools\adb.exe'
& $adb devices
& $adb -s <serial> install -r .\app\build\outputs\apk\debug\app-debug.apk
& $adb -s <serial> install -r .\app\build\outputs\apk\androidTest\debug\app-debug-androidTest.apk
& $adb -s <serial> shell am instrument -w -e class com.mirroxy.android.CoreIntegrationTest com.mirroxy.android.test/androidx.test.runner.AndroidJUnitRunner
```

VPN 可选测试须人工确认系统对话框，仅接管 Mirroxy 的 UID，结束后停止服务、恢复应用列表并删除测试节点：

```powershell
& $adb -s <serial> shell am instrument -w -e class com.mirroxy.android.VpnIntegrationTest -e vpn true com.mirroxy.android.test/androidx.test.runner.AndroidJUnitRunner
```

VPN 最终验证结果单独记录在 `ANDROID-QA.md`。没有授权则不会启动 VPN。默认常规测试跳过此项。

## 后续范围

订阅 URL 加密管理和刷新（当前只导入订阅内容）、累计流量、持续测速、复杂规则路由、多节点并行、断网/切网恢复、下载管理尚未完成。下载模块后续维持最多 30 任务、每任务最多 16 连接，使用系统文件访问/打开与安装授权，下载后不得自动运行。协议支持按实际 Xray 核心及 Android 集成验证范围显示，不宣称本开发版支持 sing-box 专属协议或所有协议组合。
