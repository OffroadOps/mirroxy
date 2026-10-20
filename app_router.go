package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type applicationProcess struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

type appRouterRuntime struct {
	ControlPort int
	Secret      string
	StartedAt   time.Time
}

func (a *App) appRoutingState() M {
	profiles := append([]AppRouteProfile{}, a.cfg.AppRouting.Profiles...)
	phase := a.appRouterPhase
	if phase == "" {
		phase = "stopped"
	}
	return M{
		"supported":     platformApplicationRoutingSupported(),
		"enabled":       a.cfg.AppRouting.Enabled,
		"running":       phase == "running",
		"active":        a.appRouter != nil,
		"phase":         phase,
		"error":         a.appRouterError,
		"defaultNode":   a.cfg.AppRouting.DefaultNode,
		"strictRoute":   !a.cfg.AppRouting.CompatMode,
		"compatMode":    a.cfg.AppRouting.CompatMode,
		"profiles":      profiles,
		"requiresAdmin": runtime.GOOS == "windows",
		"capabilities":  a.capabilities,
	}
}

func (a *App) normalizeAppRouting(input AppRoutingSettings) (AppRoutingSettings, error) {
	result := AppRoutingSettings{
		DefaultNode: strings.TrimSpace(input.DefaultNode),
		StrictRoute: true,
		CompatMode:  input.CompatMode,
		Secret:      a.cfg.AppRouting.Secret,
		Profiles:    []AppRouteProfile{},
	}
	if result.Secret == "" {
		result.Secret = newID() + newID()
	}
	if len(input.Profiles) > 128 {
		return result, errors.New("应用配置最多 128 个")
	}
	nodes := map[string]bool{}
	for _, n := range a.cfg.Nodes {
		nodes[n.ID] = true
	}
	if result.DefaultNode != "" && !nodes[result.DefaultNode] {
		return result, errors.New("默认应用节点不存在")
	}
	seenIDs := map[string]bool{}
	seenExecutables := map[string]string{}
	blocked := map[string]bool{
		"mirroxy.exe": true, "mirroxy_core_a.exe": true,
		"mirroxy_core_b.exe": true, "upgrade.exe": true,
	}
	for _, raw := range input.Profiles {
		p := AppRouteProfile{ID: strings.TrimSpace(raw.ID), Name: strings.TrimSpace(raw.Name), NodeID: strings.TrimSpace(raw.NodeID), Enabled: raw.Enabled}
		if p.ID == "" || seenIDs[p.ID] {
			p.ID = newID()
		}
		seenIDs[p.ID] = true
		if p.NodeID != "" && !nodes[p.NodeID] {
			return result, fmt.Errorf("%s 选择的节点不存在", first(p.Name, "应用配置"))
		}
		if p.Enabled && first(p.NodeID, result.DefaultNode) == "" {
			return result, fmt.Errorf("%s 尚未选择节点", first(p.Name, "应用配置"))
		}
		if len(raw.Executables) > 32 {
			return result, fmt.Errorf("%s 最多匹配 32 个程序", first(p.Name, "应用配置"))
		}
		seenProfile := map[string]bool{}
		for _, value := range raw.Executables {
			value = strings.Trim(strings.TrimSpace(value), `"`)
			if value == "" {
				continue
			}
			if len(value) > 1024 || strings.ContainsAny(value, "*?\n\r") {
				return result, fmt.Errorf("%s 包含无效程序路径", first(p.Name, "应用配置"))
			}
			if strings.ContainsAny(value, `\/`) {
				if !filepath.IsAbs(value) {
					return result, fmt.Errorf("程序路径必须为完整路径：%s", value)
				}
				value = filepath.Clean(value)
			} else {
				value = filepath.Base(value)
			}
			base := strings.ToLower(filepath.Base(value))
			if filepath.Ext(base) != ".exe" {
				return result, fmt.Errorf("目前仅支持 Windows .exe：%s", value)
			}
			if blocked[base] {
				return result, fmt.Errorf("不能代理 Mirroxy 自身进程：%s", value)
			}
			key := strings.ToLower(value)
			if seenProfile[key] {
				continue
			}
			if owner := seenExecutables[key]; owner != "" {
				return result, fmt.Errorf("%s 已被配置 %s 使用", value, owner)
			}
			seenProfile[key] = true
			seenExecutables[key] = first(p.Name, "另一个应用")
			p.Executables = append(p.Executables, value)
		}
		if len(p.Executables) == 0 {
			return result, fmt.Errorf("%s 至少需要一个程序", first(p.Name, "应用配置"))
		}
		if p.Name == "" {
			p.Name = strings.TrimSuffix(filepath.Base(p.Executables[0]), filepath.Ext(p.Executables[0]))
		}
		if len([]rune(p.Name)) > 128 {
			return result, errors.New("应用名称最多 128 个字符")
		}
		result.Profiles = append(result.Profiles, p)
	}
	return result, nil
}

func (a *App) saveAppRouting(input AppRoutingSettings) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !platformApplicationRoutingSupported() {
		return errors.New("应用代理目前仅支持 Windows")
	}
	if a.appRouter != nil || a.appRouterPhase == "starting" {
		return errors.New("请先停止应用代理再修改配置")
	}
	normalized, err := a.normalizeAppRouting(input)
	if err != nil {
		return err
	}
	before := a.cfg.AppRouting
	a.cfg.AppRouting = normalized
	if err = a.save(); err != nil {
		a.cfg.AppRouting = before
		return err
	}
	a.appRouterError = ""
	a.event("应用代理配置已保存")
	return nil
}

func (a *App) selectedAppRouteNodes() ([]string, error) {
	seen := map[string]bool{}
	ids := []string{}
	for _, p := range a.cfg.AppRouting.Profiles {
		if !p.Enabled {
			continue
		}
		id := first(p.NodeID, a.cfg.AppRouting.DefaultNode)
		if id == "" || a.node(id) == nil {
			return nil, fmt.Errorf("%s 尚未选择有效节点", p.Name)
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("请至少启用一个应用配置")
	}
	return ids, nil
}

func (a *App) applicationRouterConfig() (M, error) {
	rules := []any{M{"ip_is_private": true, "action": "route", "outbound": "direct"}}
	outbounds := []any{}
	seenOutbounds := map[string]bool{}
	for _, p := range a.cfg.AppRouting.Profiles {
		if !p.Enabled {
			continue
		}
		id := first(p.NodeID, a.cfg.AppRouting.DefaultNode)
		n := a.node(id)
		if n == nil || !n.Enabled || n.AppPort == 0 {
			return nil, fmt.Errorf("%s 的节点尚未就绪", p.Name)
		}
		tag := "node-" + id
		if !seenOutbounds[tag] {
			seenOutbounds[tag] = true
			outbounds = append(outbounds, M{
				"type": "socks", "tag": tag, "server": "127.0.0.1", "server_port": n.AppPort,
				"version": "5", "username": "mirroxy-app", "password": a.cfg.AppRouting.Secret,
			})
		}
		names, paths := []string{}, []string{}
		for _, executable := range p.Executables {
			if filepath.IsAbs(executable) {
				paths = append(paths, executable)
			} else {
				names = append(names, executable)
			}
		}
		rule := M{"network": []string{"tcp", "udp"}, "action": "route", "outbound": tag}
		if len(names) > 0 {
			rule["process_name"] = names
		}
		if len(paths) > 0 {
			rule["process_path"] = paths
		}
		rules = append(rules, rule)
	}
	outbounds = append(outbounds, M{"type": "direct", "tag": "direct"})
	return M{
		"log": M{"level": "warn"},
		"inbounds": []any{M{
			"type": "tun", "tag": "app-tun", "interface_name": "Mirroxy Apps",
			"address": []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"}, "mtu": 9000, "auto_route": true,
			"strict_route": !a.cfg.AppRouting.CompatMode, "stack": "mixed", "dns_mode": "disabled",
		}},
		"outbounds": outbounds,
		"route":     M{"auto_detect_interface": true, "rules": rules, "final": "direct"},
	}, nil
}

func (a *App) startApplicationRouting() error {
	if !platformApplicationRoutingSupported() {
		return errors.New("应用代理目前仅支持 Windows")
	}
	a.appRouterMu.Lock()
	defer a.appRouterMu.Unlock()
	if err := a.ensureCore("sing-box"); err != nil {
		return err
	}

	a.mu.Lock()
	if a.appRouter != nil {
		a.mu.Unlock()
		return errors.New("应用路由助手仍在运行，请先停止后重试")
	}
	normalized, err := a.normalizeAppRouting(a.cfg.AppRouting)
	if err != nil {
		a.mu.Unlock()
		return err
	}
	a.cfg.AppRouting = normalized
	ids, err := a.selectedAppRouteNodes()
	if err != nil {
		a.mu.Unlock()
		return err
	}
	before := cloneSettings(a.cfg)
	for _, id := range ids {
		n := a.node(id)
		if n.Port == 0 {
			n.Port, err = a.allocatePort()
			if err != nil {
				a.cfg = before
				a.mu.Unlock()
				return err
			}
		}
		if n.AppPort == 0 || (!n.Enabled && !portAvailable(n.AppPort)) {
			n.AppPort, err = a.allocateAppPort()
			if err != nil {
				a.cfg = before
				a.mu.Unlock()
				return err
			}
		}
		if !n.Enabled && !portAvailable(n.Port) {
			a.cfg = before
			a.mu.Unlock()
			return fmt.Errorf("%s 的固定端口 %d 已被占用", n.Name, n.Port)
		}
		n.Enabled = true
	}
	config, err := a.applicationRouterConfig()
	if err != nil {
		a.cfg = before
		a.mu.Unlock()
		return err
	}
	path := a.dataPath(filepath.Join("runtime", "Mirroxy_app_router.json"))
	if err = writeJSON(path, config); err == nil {
		err = a.validateCore("sing-box", path, a.corePath("sing-box"))
	}
	if err == nil {
		err = a.reconcile()
	}
	if err == nil {
		err = a.save()
	}
	if err != nil {
		a.cfg = before
		_ = a.reconcile()
		_ = a.save()
		_ = os.Remove(path)
		a.mu.Unlock()
		return err
	}
	a.appRouterPhase = "starting"
	a.appRouterError = ""
	controlPort, err := reserveApplicationRouterPort()
	secret := newID() + newID()
	launched := false
	if err == nil {
		err = startElevatedApplicationRouter(a.root, path, controlPort, secret, os.Getpid())
		launched = err == nil
	}
	if err == nil {
		err = waitApplicationRouter(controlPort, secret, 35*time.Second)
	}
	_ = os.Remove(path)
	if err != nil {
		var cleanupRuntime *appRouterRuntime
		if launched {
			if stopErr := applicationRouterRequest(controlPort, secret, http.MethodPost, "/stop", 4*time.Second); stopErr != nil {
				cleanupRuntime = &appRouterRuntime{ControlPort: controlPort, Secret: secret, StartedAt: time.Now()}
			}
		}
		a.cfg = before
		a.appRouter = cleanupRuntime
		a.cfg.AppRouting.Enabled = cleanupRuntime != nil
		a.appRouterPhase = "error"
		a.appRouterError = err.Error()
		_ = a.reconcile()
		_ = a.save()
		a.event("应用代理启动失败：" + err.Error())
		a.mu.Unlock()
		if cleanupRuntime != nil {
			go a.monitorApplicationRouter(cleanupRuntime)
		}
		return err
	}
	rt := &appRouterRuntime{ControlPort: controlPort, Secret: secret, StartedAt: time.Now()}
	a.appRouter = rt
	a.appRouterPhase = "running"
	a.cfg.AppRouting.Enabled = true
	err = a.save()
	if err != nil {
		a.event("应用代理已启动，但运行状态保存失败：" + err.Error())
	} else {
		a.event("应用代理已启动")
	}
	a.mu.Unlock()
	go a.monitorApplicationRouter(rt)
	return nil
}

func reserveApplicationRouterPort() (int, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port, nil
}

func applicationRouterRequest(port int, secret, method, path string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, method, fmt.Sprintf("http://127.0.0.1:%d%s", port, path), nil)
	req.Header.Set("X-Mirroxy-Router-Token", secret)
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: timeout}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("应用路由助手返回 HTTP %d", res.StatusCode)
	}
	return nil
}

func waitApplicationRouter(port int, secret string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := applicationRouterRequest(port, secret, http.MethodGet, "/status", 350*time.Millisecond); err == nil {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return errors.New("等待管理员授权或虚拟网卡启动超时")
}

func (a *App) monitorApplicationRouter(expected *appRouterRuntime) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	failures := 0
	for range ticker.C {
		err := applicationRouterRequest(expected.ControlPort, expected.Secret, http.MethodGet, "/status", 700*time.Millisecond)
		if err == nil {
			failures = 0
			continue
		}
		failures++
		if failures < 3 {
			continue
		}
		a.mu.Lock()
		if a.appRouter == expected {
			a.appRouter = nil
			a.appRouterPhase = "error"
			a.appRouterError = "应用路由核心已退出，请重新开启"
			a.cfg.AppRouting.Enabled = false
			_ = a.save()
			a.event("应用代理意外停止")
		}
		a.mu.Unlock()
		return
	}
}

func (a *App) stopApplicationRouting() error {
	a.appRouterMu.Lock()
	defer a.appRouterMu.Unlock()
	a.mu.Lock()
	rt := a.appRouter
	if rt == nil && (a.appRouterPhase == "" || a.appRouterPhase == "stopped") {
		a.cfg.AppRouting.Enabled = false
		a.mu.Unlock()
		return nil
	}
	a.appRouterPhase = "stopping"
	a.mu.Unlock()
	var err error
	if rt != nil {
		err = applicationRouterRequest(rt.ControlPort, rt.Secret, http.MethodPost, "/stop", 4*time.Second)
	}
	a.mu.Lock()
	a.appRouter = nil
	a.cfg.AppRouting.Enabled = false
	if err != nil {
		a.appRouter = rt
		a.cfg.AppRouting.Enabled = rt != nil
		a.appRouterPhase = "error"
		a.appRouterError = "无法确认应用路由助手已退出：" + err.Error()
		a.event(a.appRouterError)
	} else {
		a.appRouterPhase = "stopped"
		a.appRouterError = ""
		a.event("应用代理已停止")
	}
	saveErr := a.save()
	a.mu.Unlock()
	if err != nil {
		return err
	}
	return saveErr
}

func (a *App) appRouterUsesNode(id string) bool {
	if a.appRouter == nil && a.appRouterPhase != "starting" {
		return false
	}
	for _, p := range a.cfg.AppRouting.Profiles {
		if p.Enabled && first(p.NodeID, a.cfg.AppRouting.DefaultNode) == id {
			return true
		}
	}
	return false
}

func sortedApplicationProcesses(processes []applicationProcess) {
	sort.Slice(processes, func(i, j int) bool {
		left, right := strings.ToLower(processes[i].Name), strings.ToLower(processes[j].Name)
		if left == right {
			return processes[i].PID < processes[j].PID
		}
		return left < right
	})
}

func encodeAppRoutingForTest(value AppRoutingSettings) string {
	b, _ := json.Marshal(value)
	return string(b)
}
