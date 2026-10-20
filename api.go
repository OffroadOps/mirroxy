package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
)

func (a *App) guard(next http.Handler) http.Handler { return a.accessGuard(next, false) }
func respond(w http.ResponseWriter, value any, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(M{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}
func (a *App) state() M {
	nodes := []any{}
	en, proxy := currentProxy()
	for _, n := range a.cfg.Nodes {
		status := "stopped"
		if n.Enabled {
			status = "error"
			if a.cores[n.coreKey()].alive() {
				status = "running"
			}
		}
		nodes = append(nodes, M{"id": n.ID, "name": n.Name, "protocol": n.Protocol, "engine": corePublicID(n.Engine), "server": n.Server, "port": n.Port, "enabled": n.Enabled, "listBottom": n.ListBottom, "route": n.Route, "effectiveRoute": a.effectiveRoute(n), "source": n.Source, "status": status, "systemProxy": n.Port > 0 && en == 1 && proxy == fmt.Sprintf("127.0.0.1:%d", n.Port), "stats": *a.trafficFor(n.ID), "health": a.healthFor(n)})
	}
	cores := []any{}
	for _, engine := range []string{"xray", "sing-box"} {
		_, e := os.Stat(a.corePath(engine))
		cores = append(cores, M{"engine": corePublicID(engine), "installed": e == nil, "running": a.engineRunning(engine), "version": a.cfg.Updated[engine]})
	}
	subs := []any{}
	for _, s := range a.cfg.Subscriptions {
		subs = append(subs, M{"id": s.ID, "name": s.Name, "updated": s.Updated, "error": s.Error})
	}
	geoFiles := a.geoFilesState()
	return M{"version": version, "appUpdate": a.appUpdate, "platform": platformState(), "token": a.token, "nodes": nodes, "cores": cores, "events": a.events, "subscriptions": subs, "settings": M{"startPort": a.cfg.StartPort, "autoUpdate": a.cfg.AutoUpdate, "autoSubscribe": a.cfg.AutoSubscribe, "autostart": a.cfg.Autostart, "openBrowser": a.cfg.OpenBrowser, "resumeNodes": a.cfg.ResumeNodes, "defaultRoute": a.cfg.DefaultRoute, "routingMode": a.cfg.RoutingMode, "logLevel": a.cfg.LogLevel, "subscribeHours": a.cfg.SubscribeHours, "healthMonitor": a.cfg.HealthMonitor, "healthInterval": a.cfg.HealthInterval, "autoDownload": a.cfg.AutoDownload, "autoDeploy": a.cfg.AutoDeploy, "systemProxyMode": first(a.cfg.SystemProxyMode, "unchanged"), "systemProxyNode": a.cfg.SystemProxyNode, "pauseSystemProxyOnAccelerator": a.acceleratorGuardEnabled(), "acceleratorPaused": a.cfg.PausedSystemProxy != nil, "bypassLAN": a.bypassLANEnabled(), "geoAutoUpdate": a.cfg.GeoAutoUpdate, "panelPort": a.configuredPanelPort(), "panelTLS": a.panelTLS}, "updating": a.busyUpdate, "geoFiles": geoFiles, "systemProxy": proxy, "systemProxyEnabled": en == 1, "root": a.root, "remote": a.remoteState(), "traffic": a.trafficState(), "tray": a.trayWindow != 0, "panelPort": a.panelPort}
}
func (a *App) api(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/downloads") {
		a.downloadAPI(w, r)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/traffic" {
		a.mu.Lock()
		defer a.mu.Unlock()
		respond(w, a.trafficState(), nil)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/state" {
		a.mu.Lock()
		defer a.mu.Unlock()
		s := a.state()
		s["localAccess"] = localAccess(r)
		s["settingsEditable"] = localAccess(r) || serverPlatform()
		if localAccess(r) {
			s["appRouting"] = a.appRoutingState()
		}
		respond(w, s, nil)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/processes" {
		if !localAccess(r) {
			respond(w, nil, errors.New("只能在本机读取运行中的程序"))
			return
		}
		processes, err := listApplicationProcesses()
		respond(w, M{"processes": processes}, err)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/app-routing/browse" {
		if !localAccess(r) {
			respond(w, nil, errors.New("只能在本机浏览目录"))
			return
		}
		result, err := browseDirectory(r.URL.Query().Get("path"))
		respond(w, result, err)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/app-routing/scan" {
		if !localAccess(r) {
			respond(w, nil, errors.New("只能在本机扫描程序目录"))
			return
		}
		result, err := scanDirectoryExecutables(r.URL.Query().Get("path"))
		respond(w, M{"executables": result}, err)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/node/qr" {
		if !localAccess(r) {
			respond(w, nil, errors.New("二维码只能在本机面板生成"))
			return
		}
		a.mu.Lock()
		n := a.node(r.URL.Query().Get("id"))
		var link string
		var err error
		if n == nil {
			err = errors.New("节点不存在")
		} else {
			link, err = shareLink(*n)
		}
		a.mu.Unlock()
		if err != nil {
			respond(w, nil, err)
			return
		}
		var png []byte
		if r.URL.Query().Get("theme") == "dark" {
			// Inverted QR (light modules on the dark card colour) so the dark
			// theme does not need a glaring white plate.
			code, err := qrcode.New(link, qrcode.Medium)
			if err != nil {
				respond(w, nil, fmt.Errorf("二维码生成失败：%s", err))
				return
			}
			code.BackgroundColor = color.RGBA{R: 0x1b, G: 0x28, B: 0x2d, A: 0xff}
			code.ForegroundColor = color.RGBA{R: 0xd7, G: 0xe7, B: 0xdf, A: 0xff}
			png, err = code.PNG(320)
			if err != nil {
				respond(w, nil, fmt.Errorf("二维码生成失败：%s", err))
				return
			}
		} else {
			png, err = qrcode.Encode(link, qrcode.Medium, 320)
			if err != nil {
				respond(w, nil, fmt.Errorf("二维码生成失败：%s", err))
				return
			}
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(png)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/app-routing/conflicts" {
		if !localAccess(r) {
			respond(w, nil, errors.New("只能在本机面板检测加速器冲突"))
			return
		}
		conflicts := []M{}
		if platformApplicationRoutingSupported() {
			if processes, err := listApplicationProcesses(); err == nil {
				conflicts = detectAcceleratorConflicts(processes)
			}
		}
		respond(w, M{"conflicts": conflicts}, nil)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/panel/ca" {
		if !localAccess(r) {
			respond(w, nil, errors.New("只能在本机下载根证书"))
			return
		}
		caPEM, err := os.ReadFile(a.dataPath("Mirroxy_ca.pem"))
		if err != nil {
			respond(w, nil, errors.New("根证书尚未生成，请先在设置中生成或开启 HTTPS 面板"))
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", "attachment; filename=Mirroxy-root-ca.pem")
		_, _ = w.Write(caPEM)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/panel/cert-status" {
		if !localAccess(r) {
			respond(w, nil, errors.New("只能在本机查看证书状态"))
			return
		}
		a.mu.Lock()
		status := a.panelCAStatus()
		a.mu.Unlock()
		status["trusted"] = a.verifyPanelTrust()
		respond(w, status, nil)
		return
	}
	if r.Method == "POST" && r.URL.Path == "/api/panel/trust-ca" {
		if !localAccess(r) {
			respond(w, nil, errors.New("只能在本机安装根证书"))
			return
		}
		a.mu.Lock()
		_, err := a.ensurePanelCertificate()
		caPath := a.dataPath("Mirroxy_ca.pem")
		a.mu.Unlock()
		if err == nil {
			err = installPanelTrust(caPath)
		}
		trusted := false
		if err == nil {
			trusted = a.waitPanelTrust(2 * time.Second)
			if !trusted {
				err = errors.New("根证书已导入，但 HTTPS 校验仍未通过；请完全退出浏览器后重试")
			}
		}
		if err == nil {
			a.mu.Lock()
			a.event("本地根证书已安装到当前用户信任存储并通过 HTTPS 校验")
			a.mu.Unlock()
		}
		respond(w, M{"ok": err == nil, "trusted": trusted}, err)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var body struct {
		ID, Action, Text, Path, Route, Name, URL, Engine, Mode string
		SettingsPatch
		Remote     RemoteSettings
		AppRouting AppRoutingSettings `json:"appRouting"`
		Password   string
		Deploy     *bool    `json:"deploy"`
		Files      []string `json:"files"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	defer r.Body.Close()
	if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
		respond(w, nil, errors.New("请求内容不是有效 JSON"))
		return
	}
	switch r.URL.Path {
	case "/api/node/capabilities":
		if !localAccess(r) {
			respond(w, nil, errors.New("节点能力只能从本机面板检测"))
			return
		}
		result, err := a.probeNodeCapabilities(body.ID)
		respond(w, result, err)
		return
	case "/api/app-routing/config":
		if !localAccess(r) {
			respond(w, nil, errors.New("应用代理只能从本机面板修改"))
			return
		}
		respond(w, M{"ok": true}, a.saveAppRouting(body.AppRouting))
		return
	case "/api/app-routing/start":
		if !localAccess(r) {
			respond(w, nil, errors.New("应用代理只能从本机面板开启"))
			return
		}
		respond(w, M{"ok": true}, a.startApplicationRouting())
		return
	case "/api/app-routing/stop":
		if !localAccess(r) {
			respond(w, nil, errors.New("应用代理只能从本机面板停止"))
			return
		}
		respond(w, M{"ok": true}, a.stopApplicationRouting())
		return
	case "/api/stop-all":
		if !localAccess(r) {
			a.mu.Lock()
			appRoutingActive := a.appRouter != nil || a.appRouterPhase == "starting" || a.appRouterPhase == "running"
			a.mu.Unlock()
			if appRoutingActive {
				respond(w, nil, errors.New("应用代理正在运行，请在本机面板停止后再全部停止节点"))
				return
			}
		}
		if err := a.stopApplicationRouting(); err != nil {
			respond(w, nil, err)
			return
		}
		a.mu.Lock()
		before := cloneSettings(a.cfg)
		for i := range a.cfg.Nodes {
			a.cfg.Nodes[i].Enabled = false
		}
		err := a.reconcile()
		if err == nil {
			err = a.save()
		}
		if err != nil {
			a.cfg = before
			_ = a.reconcile()
		}
		a.syncProxy()
		a.mu.Unlock()
		respond(w, M{"ok": true}, err)
		return
	case "/api/app-update/check":
		result, e := a.checkAppUpdate()
		respond(w, result, e)
		return
	case "/api/app-update/apply":
		respond(w, M{"ok": true}, a.startAppUpdate())
		return
	case "/api/import":
		result, e := a.importAndDeploy(body.Text, body.Path, body.Deploy)
		respond(w, result, e)
		return
	case "/api/node":
		if body.Action == "start" {
			respond(w, M{"ok": true}, a.startNode(body.ID))
			return
		}
	case "/api/test":
		result, e := a.testNode(body.ID)
		respond(w, result, e)
		return
	case "/api/core/check":
		engine, ok := internalEngine(body.Engine)
		if !ok {
			respond(w, nil, errors.New("无效核心"))
			return
		}
		result, e := a.latestRelease(engine)
		if e != nil {
			respond(w, nil, e)
		} else {
			respond(w, M{"version": result.Tag, "engine": corePublicID(engine)}, nil)
		}
		return
	case "/api/core/update":
		engine, ok := internalEngine(body.Engine)
		if !ok {
			respond(w, nil, errors.New("无效核心"))
			return
		}
		e := a.updateCore(engine)
		respond(w, M{"ok": true}, e)
		return
	case "/api/geofiles/check":
		tag, e := a.geoReleaseTag()
		if e != nil {
			respond(w, nil, e)
			return
		}
		respond(w, M{"version": tag}, nil)
		return
	case "/api/geofiles/update":
		e := a.updateGeoFiles(body.Files, false)
		respond(w, M{"ok": true}, e)
		return
	case "/api/subscription/refresh":
		result, e := a.refreshSubscriptionAndDeploy(body.ID)
		respond(w, result, e)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	switch r.URL.Path {
	case "/api/routing":
		respond(w, M{"ok": true}, a.setRoutingMode(body.Mode))
	case "/api/node/rename":
		respond(w, M{"ok": true}, a.renameNode(body.ID, body.Name))
	case "/api/node/share":
		n := a.node(body.ID)
		if n == nil {
			respond(w, nil, errors.New("节点不存在"))
			return
		}
		link, e := shareLink(*n)
		respond(w, M{"name": n.Name, "protocol": n.Protocol, "url": link}, e)
	case "/api/node":
		e := a.changeNode(body.ID, body.Action, body.Route)
		respond(w, M{"ok": true}, e)
	case "/api/settings":
		respond(w, M{"ok": true}, a.saveSettings(body.SettingsPatch))
	case "/api/remote":
		respond(w, M{"ok": true}, a.saveRemote(body.Remote, body.Password))
	case "/api/subscription/add":
		u, e := url.Parse(body.URL)
		if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			respond(w, nil, errors.New("请填写有效的 HTTP / HTTPS 订阅地址"))
			return
		}
		s := Subscription{ID: newID(), Name: first(strings.TrimSpace(body.Name), "我的订阅"), URL: body.URL}
		a.cfg.Subscriptions = append(a.cfg.Subscriptions, s)
		if e = a.save(); e != nil {
			a.cfg.Subscriptions = a.cfg.Subscriptions[:len(a.cfg.Subscriptions)-1]
			respond(w, nil, e)
			return
		}
		respond(w, M{"id": s.ID}, nil)
	case "/api/proxy":
		if body.Action == "restore" {
			a.restoreProxy()
			a.cfg.SystemProxyMode = "unchanged"
			a.cfg.SystemProxyNode = ""
			a.cfg.PausedSystemProxy = nil
			_ = a.save()
			respond(w, M{"ok": true}, nil)
		} else if body.Action == "mode" {
			respond(w, M{"ok": true}, a.applySystemProxy(body.Mode, body.ID))
		} else {
			mode := "auto"
			if body.Action == "clear" {
				mode = "clear"
			}
			respond(w, M{"ok": true}, a.applySystemProxy(mode, body.ID))
		}
	case "/api/log":
		engine, ok := internalEngine(body.Engine)
		if !ok {
			respond(w, nil, errors.New("无效核心"))
			return
		}
		key := engine
		if engine == "sing-box" {
			if n := a.node(body.ID); n != nil && n.Engine == "sing-box" {
				key = n.coreKey()
			} else {
				for _, n := range a.cfg.Nodes {
					if n.Engine == "sing-box" {
						key = n.coreKey()
						break
					}
				}
			}
		}
		f, e := os.Open(a.dataPath(filepath.Join("runtime", "Mirroxy_worker_"+runtimeFileID(key)+".log")))
		if e != nil {
			respond(w, M{"text": "暂无日志"}, nil)
			return
		}
		defer f.Close()
		st, _ := f.Stat()
		if st.Size() > 16000 {
			_, _ = f.Seek(-16000, io.SeekEnd)
		}
		b, _ := io.ReadAll(f)
		respond(w, M{"text": string(b)}, nil)
	case "/api/shutdown":
		respond(w, M{"ok": true}, nil)
		a.requestQuit()
	default:
		http.NotFound(w, r)
	}
}
func (a *App) testNode(id string) (M, error) {
	a.mu.Lock()
	p, err := a.beginHealth(id)
	a.mu.Unlock()
	if err != nil {
		return nil, err
	}
	h, err := a.runHealth(p)
	return M{"ok": err == nil, "latencyMs": h.LatencyMs, "health": h, "port": p.node.Port}, err
}
func (a *App) refreshSubscriptionStored(id string) (int, error) {
	a.mu.Lock()
	var sub Subscription
	for _, s := range a.cfg.Subscriptions {
		if s.ID == id {
			sub = s
			break
		}
	}
	a.mu.Unlock()
	if sub.ID == "" {
		return 0, errors.New("订阅不存在")
	}
	content, e := a.fetch(sub.URL, 8<<20, 25*time.Second)
	var nodes []Node
	var warnings []string
	if e == nil {
		nodes, warnings = parseLinks(string(content))
		if len(nodes) == 0 {
			e = fmt.Errorf("订阅没有支持的节点：%s", strings.Join(warnings, "；"))
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if e != nil {
		for i := range a.cfg.Subscriptions {
			if a.cfg.Subscriptions[i].ID == id {
				a.cfg.Subscriptions[i].Error = "订阅下载或解析失败，请检查地址与网络"
			}
		}
		_ = a.save()
		return 0, errors.New("订阅下载或解析失败，请检查地址与网络")
	}
	if a.busyUpdate {
		return 0, errors.New("核心更新中，请稍后更新订阅")
	}
	before := cloneSettings(a.cfg)
	matched := make([]bool, len(nodes))
	count := 0
	for i := range a.cfg.Nodes {
		old := &a.cfg.Nodes[i]
		if old.Source != id {
			continue
		}
		match := -1
		for j, n := range nodes {
			if !matched[j] && fingerprint(n) == fingerprint(*old) {
				match = j
				break
			}
		}
		if match < 0 {
			candidates := []int{}
			for j, n := range nodes {
				if !matched[j] && n.Name == first(old.OriginalName, old.Name) && n.Protocol == old.Protocol && n.Server == old.Server {
					candidates = append(candidates, j)
				}
			}
			if len(candidates) == 1 {
				match = candidates[0]
			}
		}
		if match < 0 {
			old.Enabled = false
			continue
		}
		fresh := nodes[match]
		matched[match] = true
		fresh.ID = old.ID
		fresh.Port = old.Port
		fresh.ListBottom = old.ListBottom
		fresh.AppPort = old.AppPort
		fresh.Enabled = old.Enabled
		fresh.Source = id
		fresh.Route = old.Route
		if old.OriginalName != "" {
			fresh.OriginalName = fresh.Name
			fresh.Name = old.Name
		}
		*old = fresh
		count++
	}
	for j, n := range nodes {
		if matched[j] {
			continue
		}
		n.ID = newID()
		n.Source = id
		n.Route = first(a.cfg.DefaultRoute, "inherit")
		a.cfg.Nodes = append(a.cfg.Nodes, n)
		count++
	}
	for i := range a.cfg.Subscriptions {
		if a.cfg.Subscriptions[i].ID == id {
			a.cfg.Subscriptions[i].Updated = time.Now().Format("2006-01-02 15:04")
			a.cfg.Subscriptions[i].Error = ""
		}
	}
	if e = a.reconcile(); e == nil {
		e = a.save()
	}
	if e != nil {
		a.cfg = before
		_ = a.reconcile()
		return 0, e
	}
	a.syncProxy()
	a.event(fmt.Sprintf("订阅已更新：%s，%d 个节点", sub.Name, count))
	return count, nil
}
