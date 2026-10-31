# 下载桌面客户端（Windows 首版）

Windows 托盘的「下载管理」启动同目录的 `Mirroxy-downloads.exe -panel <本机端口>`。
使用 Wails v2.17.0 和系统 WebView2；界面嵌入程序，不启动 Edge、不从 HTTP 面板加载页面。
下载任务仍归 Mirroxy 后台所有。关闭客户端不会停止下载或节点；后台未运行时窗口会提示连接失败。
默认目录、代理、连接数与限速由后台加密保存。原有任务与浏览器接力继续复用。

## 构建

在 Windows 执行 `./build.ps1`，输出到 `D:\Mirroxy\work\build`。
或在 `desktop` 中执行：

```powershell
go build -tags production -trimpath -ldflags '-s -w -H=windowsgui' -o ../work/Mirroxy-downloads.exe .
go test ./...
```

`desktop` 是独立模块；主后台不依赖 Wails，Linux 四平台后台构建维持原方式。
桌面窗口目前只接入 Windows 发行包；Linux/macOS 桌面客户端尚未实机验证，不宣称支持。
Windows 需要 WebView2 Runtime。没有运行库时应安装微软官方运行库。
客户端缓存位于系统用户缓存目录的 `Mirroxy/downloads-<port>`，不放入发行包。

更新或手动替换桌面程序前先关闭下载窗口，避免 Windows 文件占用。不会因此停止后台任务。
前端只通过受限 Go 方法访问下载 API；桥接固定连接 127.0.0.1、禁用代理和重定向，写请求取得本机 token。
主面板和浏览器接力设置仍通过系统浏览器打开。

为兼容旧版在线更新器的文件白名单，当前桌面客户端随 Windows 完整包分发，在线更新包不新增此文件。首次安装或升级桌面客户端使用完整包。
