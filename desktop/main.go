package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"mirroxy/web"
)

type desktopAssets struct {
	fs.FS
	view string
}

func (a desktopAssets) Open(name string) (fs.File, error) {
	if name == "index.html" && a.view != "panel" {
		name = "downloads.html"
	}
	return a.FS.Open(name)
}

type Desktop struct {
	ctx     context.Context
	backend *backend
	view    string
}

func (d *Desktop) Request(path, body string) (string, error) { return d.backend.request(path, body) }
func (d *Desktop) SetPanelPort(port int)                     { d.backend.setPort(port) }
func (d *Desktop) PickDirectory(current string) (string, error) {
	if info, err := os.Stat(current); err != nil || !info.IsDir() {
		current = ""
	}
	return runtime.OpenDirectoryDialog(d.ctx, runtime.OpenDialogOptions{Title: "选择下载保存目录", DefaultDirectory: current})
}
func (d *Desktop) launchSelf(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}
func (d *Desktop) OpenDownloads() {
	if err := d.launchSelf("-view", "downloads", "-panel", fmt.Sprint(d.backend.currentPort())); err != nil {
		runtime.BrowserOpenURL(d.ctx, d.backend.baseURL()+"/downloads")
	}
}
func (d *Desktop) OpenPanel(relay bool) {
	args := []string{"-view", "panel", "-panel", fmt.Sprint(d.backend.currentPort())}
	suffix := "/"
	if relay {
		args = append(args, "-relay")
		suffix += "#browser-relay"
	}
	if err := d.launchSelf(args...); err != nil {
		runtime.BrowserOpenURL(d.ctx, d.backend.baseURL()+suffix)
	}
}
func (d *Desktop) OpenInBrowser() {
	runtime.BrowserOpenURL(d.ctx, d.backend.baseURL()+"/")
}
func (d *Desktop) Close() { runtime.Quit(d.ctx) }

func main() {
	port := flag.Int("panel", 15060, "Mirroxy 本机面板端口")
	view := flag.String("view", "downloads", "桌面窗口视图：panel 或 downloads")
	relay := flag.Bool("relay", false, "在主面板打开浏览器接力设置")
	flag.Parse()
	if *port < 1 || *port > 65535 {
		fmt.Fprintln(os.Stderr, "无效面板端口")
		return
	}
	if *view != "panel" && *view != "downloads" {
		fmt.Fprintln(os.Stderr, "无效视图模式")
		return
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	title, width, height, minWidth, minHeight := "Mirroxy 下载管理", 1280, 800, 900, 580
	if *view == "panel" {
		title, width, height, minWidth, minHeight = "Mirroxy", 1180, 740, 900, 560
	}
	d := &Desktop{backend: newBackend(*port), view: *view}
	err = wails.Run(&options.App{
		Title: title, Width: width, Height: height, MinWidth: minWidth, MinHeight: minHeight,
		AssetServer: &assetserver.Options{
			Assets:  desktopAssets{FS: web.Files, view: *view},
			Handler: d.backend.apiHandler(),
		},
		OnStartup: func(ctx context.Context) { d.ctx = ctx },
		OnDomReady: func(ctx context.Context) {
			if *view == "panel" && *relay {
				runtime.WindowExecJS(ctx, "location.hash = '#browser-relay'")
			}
		},
		Bind: []interface{}{d},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: fmt.Sprintf("Mirroxy-%s-%d", *view, *port),
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				runtime.WindowUnminimise(d.ctx)
				runtime.WindowShow(d.ctx)
				runtime.WindowSetAlwaysOnTop(d.ctx, true)
				runtime.WindowSetAlwaysOnTop(d.ctx, false)
				if d.view == "panel" && slices.Contains(data.Args, "-relay") {
					runtime.WindowExecJS(d.ctx, "location.hash = '#browser-relay'")
				}
			},
		},
		Windows: &windows.Options{
			WebviewUserDataPath: filepath.Join(base, "Mirroxy", fmt.Sprintf("%s-%d", *view, *port)),
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
