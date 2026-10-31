package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"mirroxy/web"
)

type desktopAssets struct{ fs.FS }

func (a desktopAssets) Open(name string) (fs.File, error) {
	if name == "index.html" {
		name = "downloads.html"
	}
	return a.FS.Open(name)
}

type Desktop struct {
	ctx     context.Context
	backend *backend
}

func (d *Desktop) Request(path, body string) (string, error) { return d.backend.request(path, body) }
func (d *Desktop) PickDirectory(current string) (string, error) {
	if info, err := os.Stat(current); err != nil || !info.IsDir() {
		current = ""
	}
	return runtime.OpenDirectoryDialog(d.ctx, runtime.OpenDialogOptions{Title: "选择下载保存目录", DefaultDirectory: current})
}
func (d *Desktop) OpenPanel(relay bool) {
	suffix := "/"
	if relay {
		suffix += "#browser-relay"
	}
	runtime.BrowserOpenURL(d.ctx, d.backend.base+suffix)
}
func (d *Desktop) Close() { runtime.Quit(d.ctx) }
func main() {
	port := flag.Int("panel", 15060, "Mirroxy 本机面板端口")
	flag.Parse()
	if *port < 1 || *port > 65535 {
		fmt.Fprintln(os.Stderr, "无效面板端口")
		return
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	d := &Desktop{backend: newBackend(*port)}
	err = wails.Run(&options.App{
		Title: "Mirroxy 下载管理", Width: 1280, Height: 800, MinWidth: 900, MinHeight: 580,
		AssetServer:        &assetserver.Options{Assets: desktopAssets{web.Files}},
		OnStartup:          func(ctx context.Context) { d.ctx = ctx },
		Bind:               []interface{}{d},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: fmt.Sprintf("Mirroxy-downloads-%d", *port), OnSecondInstanceLaunch: func(_ options.SecondInstanceData) { runtime.WindowUnminimise(d.ctx); runtime.WindowShow(d.ctx) }},
		Windows:            &windows.Options{WebviewUserDataPath: filepath.Join(base, "Mirroxy", fmt.Sprintf("downloads-%d", *port))},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
