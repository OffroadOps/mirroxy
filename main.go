package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

//go:embed web/*
var frontend embed.FS

var version = "0.7.4"

func main() {
	exe, _ := os.Executable()
	root := flag.String("dir", filepath.Dir(exe), "运行目录，包含 bin 与 data")
	port := flag.Int("panel", 15060, "本机 Web 面板端口")
	noOpen := flag.Bool("no-open", false, "启动时不打开浏览器")
	initServer := flag.Bool("init", false, "初始化服务器：生成 HTTP 管理地址和账号密码")
	resetPassword := flag.Bool("reset-password", false, "停止服务后重置管理密码")
	accessInfo := flag.Bool("access-info", false, "显示安装时生成的管理地址和账号密码")
	publicHost := flag.String("public-host", "", "安装时展示的 IP 或域名；默认自动检测网卡 IP")
	remotePort := flag.Int("remote-port", 15061, "初始化时的 HTTP 管理端口")
	check := flag.Bool("check", false, "检查本平台预置核心及配置")
	showVersion := flag.Bool("version", false, "显示版本")
	setupPanelPort := flag.String("setup-panel-port", "", "内部安装器面板端口操作")
	shutdownExistingFlag := flag.Bool("shutdown-existing", false, "保存状态并退出已运行的本机后台")
	ready := flag.Bool("ready", false, "检查本机后台是否就绪")
	afterUpdate := flag.String("after-update", "", "内部更新恢复计划")
	resumeUpdate := flag.Bool("resume-update", false, "在线更新后恢复节点")
	appRouterHelper := flag.Bool("app-router-helper", false, "内部应用路由助手")
	appRouterConfig := flag.String("app-router-config", "", "内部应用路由配置")
	appRouterControl := flag.Int("app-router-control", 0, "内部应用路由控制端口")
	appRouterSecret := flag.String("app-router-secret", "", "内部应用路由控制凭据")
	appRouterParent := flag.Int("app-router-parent", 0, "内部应用路由父进程")
	relaunchWait := flag.Int("relaunch-wait", 0, "内部重新加载：等待该 PID 退出后继续启动")
	probeServer := flag.Bool("probe-server", false, "只读检测服务器 TCP/UDP/IPv6/NAT 出口能力并输出 JSON")
	flag.Parse()
	if *relaunchWait > 0 {
		if err := waitForProcessExit(*relaunchWait, 30*time.Second); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *shutdownExistingFlag {
		if err := shutdownExisting(*port); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *ready {
		if err := checkReady(*port); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	var err error
	*root, err = filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *setupPanelPort != "" {
		if err = installerPanelPort(*root, *setupPanelPort, *port); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *probeServer {
		report := probeServerCapabilities(*root)
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err = encoder.Encode(report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := os.MkdirAll(filepath.Join(*root, "data"), 0700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *appRouterHelper {
		if err = runApplicationRouterHelper(*root, *appRouterConfig, *appRouterControl, *appRouterSecret, *appRouterParent); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *initServer || *resetPassword || *accessInfo || *check {
		a := &App{root: *root, panelPort: *port}
		switch {
		case *accessInfo:
			err = a.printAccessInfo()
		case *check:
			err = a.checkInstallation()
		default:
			err = a.initializeServer(*publicHost, *remotePort, *resetPassword)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	logfile, err := os.OpenFile(filepath.Join(*root, "data", "Mirroxy.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer logfile.Close()
	log.SetOutput(logfile)
	if serverPlatform() {
		log.SetOutput(io.MultiWriter(logfile, os.Stderr))
	}
	a := &App{root: *root, panelPort: *port, token: newID() + newID(), cores: map[string]*Core{}, shutdown: make(chan struct{}), trayDone: make(chan struct{}), appRouterPhase: "stopped"}
	if err = a.load(); err != nil {
		log.Print(err)
		if *afterUpdate != "" {
			_ = rollbackAndExec(*afterUpdate)
		}
		os.Exit(1)
	}
	// The legacy compiled default means "no explicit choice": the configured
	// panel port wins so the port is editable from the settings page.
	if *port == legacyPanelPort {
		a.panelPort = a.configuredPanelPort()
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", a.panelPort))
	if err != nil {
		if !*noOpen {
			openBrowser(fmt.Sprintf("http://127.0.0.1:%d", a.panelPort))
		}
		if *afterUpdate != "" {
			_ = rollbackAndExec(*afterUpdate)
			os.Exit(1)
		}
		log.Printf("panel already occupied: %v", err)
		if serverPlatform() {
			os.Exit(1)
		}
		return
	}
	if err = a.loadTraffic(); err != nil {
		log.Print(err)
		listener.Close()
		if *afterUpdate != "" {
			_ = rollbackAndExec(*afterUpdate)
		}
		os.Exit(1)
	}
	listener = a.wrapPanelTLS(listener)
	if a.panelPort != legacyPanelPort {
		go a.serveLegacyRedirect(legacyPanelPort)
	}
	removeLegacyPlaintext(a.dataPath(""))
	a.recoverAutostart()
	if !a.cfg.ResumeNodes && !*resumeUpdate {
		for i := range a.cfg.Nodes {
			a.cfg.Nodes[i].Enabled = false
		}
		if err = a.save(); err != nil {
			log.Print(err)
			listener.Close()
			os.Exit(1)
		}
	}
	for _, engine := range []string{"xray", "sing-box"} {
		a.cfg.Updated[engine] = coreVersion(a.corePath(engine), engine)
	}
	a.job = createJob()
	defer closeJob(a.job)
	if err = a.reconcile(); err != nil {
		if *afterUpdate != "" {
			listener.Close()
			for _, c := range a.cores {
				c.stop()
			}
			_ = rollbackAndExec(*afterUpdate)
			os.Exit(1)
		}
		a.event("启动代理失败：" + err.Error())
	}
	a.loadAppUpdate()
	a.event("Mirroxy 已启动")
	a.recoverProxy()
	a.retirePAC()
	server := &http.Server{Handler: a.handler(false), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	a.panelServer = server
	remote, rl, remoteErr := a.prepareRemote(a.cfg.Remote)
	if remoteErr != nil {
		if *afterUpdate != "" {
			listener.Close()
			for _, c := range a.cores {
				c.stop()
			}
			_ = rollbackAndExec(*afterUpdate)
			os.Exit(1)
		}
		a.remoteError = remoteErr.Error()
		a.event("远程访问未启动：" + remoteErr.Error())
	}
	if a.cfg.SystemProxyMode == "auto" {
		if a.cfg.PausedSystemProxy != nil || (a.acceleratorGuardEnabled() && a.acceleratorRunning() != "") {
			if a.cfg.PausedSystemProxy == nil {
				a.cfg.PausedSystemProxy = &PausedSystemProxy{Mode: a.cfg.SystemProxyMode, Node: a.cfg.SystemProxyNode}
				_ = a.save()
			}
			a.event("检测到加速器运行，启动时暂缓接管系统代理；加速器退出后自动恢复")
		} else if e := a.applySystemProxy(a.cfg.SystemProxyMode, a.cfg.SystemProxyNode); e != nil {
			a.event("系统代理未接管：" + e.Error())
		}
	}
	openAtStart := a.cfg.OpenBrowser
	if remoteErr == nil {
		a.startRemote(remote, rl)
	}
	go func() {
		if e := server.Serve(listener); e != nil && e != http.ErrServerClosed {
			log.Print(e)
		}
	}()
	go a.maintenance()
	go a.recoverMissingCores()
	go a.trafficLoop()
	a.healthDone = make(chan struct{})
	go a.healthLoop()
	go a.acceleratorGuardLoop()
	go a.restoreDownloads()
	go a.runTray()
	if !*noOpen && openAtStart {
		openBrowser(a.panelURL())
	}
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	select {
	case <-a.shutdown:
	case <-interrupts:
	}
	a.requestQuit()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	_ = a.stopApplicationRouting()
	a.stopDownloads()
	a.mu.Lock()
	if a.remoteServer != nil {
		a.remoteServer.Close()
	}
	a.sampleTraffic()
	_ = a.writeState("counters", a.traffic)
	a.closeTray()
	a.restoreProxy()
	for _, c := range a.cores {
		c.stop()
	}
	removeLegacyPlaintext(a.dataPath(""))
	a.event("后台已退出")
	a.mu.Unlock()
	<-a.healthDone
	select {
	case <-a.trayDone:
	case <-time.After(time.Second):
	}
	if a.pendingAppUpdate != "" {
		if err := finishAppUpdate(a.pendingAppUpdate); err != nil {
			log.Print(err)
			os.Exit(1)
		}
	}
	a.closeStore()
}

func (a *App) handler(remote bool) http.Handler {
	assets, _ := fs.Sub(frontend, "web")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/", a.apiV1)
	mux.HandleFunc("/api/", a.api)
	mux.Handle("/", http.FileServer(http.FS(assets)))
	return a.accessGuard(mux, remote)
}

func (a *App) maintenance() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	lastUpdate := time.Now()
	lastSubscribe := time.Now()
	lastGeo := time.Now()
	for {
		select {
		case <-a.shutdown:
			return
		case <-ticker.C:
			a.mu.Lock()
			for engine, c := range a.cores {
				if !c.alive() {
					if err := a.restartEngine(engine); err != nil {
						a.event(coreDisplayName(engine) + " 重启失败：" + err.Error())
					} else {
						a.event(coreDisplayName(engine) + " 已自动恢复")
					}
				}
			}
			autoUpdate := a.cfg.AutoUpdate && time.Since(lastUpdate) > 24*time.Hour
			autoSub := a.cfg.AutoSubscribe && time.Since(lastSubscribe) > time.Duration(a.cfg.SubscribeHours)*time.Hour
			autoGeo := a.cfg.GeoAutoUpdate && time.Since(lastGeo) > 24*time.Hour
			subs := append([]Subscription{}, a.cfg.Subscriptions...)
			a.mu.Unlock()
			if autoUpdate {
				lastUpdate = time.Now()
				for _, engine := range []string{"xray", "sing-box"} {
					_ = a.updateCore(engine)
				}
			}
			if autoSub {
				lastSubscribe = time.Now()
				for _, sub := range subs {
					_, _ = a.refreshSubscription(sub.ID)
				}
			}
			if autoGeo {
				lastGeo = time.Now()
				if err := a.updateGeoFiles(nil, true); err != nil {
					a.mu.Lock()
					a.event("Geo 规则文件自动更新失败：" + err.Error())
					a.mu.Unlock()
				}
			}
		}
	}
}
