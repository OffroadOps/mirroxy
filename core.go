package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Core struct {
	cmd  *exec.Cmd
	done chan struct{}
	hash string
	err  error
	log  *os.File
}

func (c *Core) alive() bool {
	if c == nil {
		return false
	}
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}
func (c *Core) stop() {
	if c == nil {
		return
	}
	if c.alive() {
		_ = c.cmd.Process.Kill()
		select {
		case <-c.done:
		case <-time.After(4 * time.Second):
		}
	}
}
func (a *App) corePath(engine string) string {
	engine = coreEngine(engine)
	if engine == "sing-box" {
		return filepath.Join(a.root, "bin", "runtime", executableName("Mirroxy_core_B"))
	}
	return filepath.Join(a.root, "bin", "runtime", executableName("Mirroxy_core_A"))
}
func (a *App) coreEnv() []string {
	return append(os.Environ(), "XRAY_LOCATION_ASSET="+filepath.Join(a.root, "bin", "resources"))
}
func runtimeFileID(key string) string {
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h[:6])
}
func coreDisplayName(engine string) string {
	if coreEngine(engine) == "sing-box" {
		return "网络核心 B"
	}
	return "网络核心 A"
}
func corePublicID(engine string) string {
	if coreEngine(engine) == "sing-box" {
		return "B"
	}
	return "A"
}
func internalEngine(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "a", "core-a", "network-core-a", "xray":
		return "xray", true
	case "b", "core-b", "network-core-b", "sing-box", "sing_box":
		return "sing-box", true
	default:
		return "", false
	}
}
func (a *App) coreConfig(key string) (M, []int) {
	engine := coreEngine(key)
	ins := []any{}
	outs := []any{}
	rules := []any{}
	globalRules := []any{}
	ruleSets := map[string]bool{}
	splitRoutes := false
	ports := []int{}
	if engine == "xray" {
		outs = append(outs, M{"tag": "block", "protocol": "blackhole"}, M{"tag": "direct", "protocol": "freedom"})
	} else {
		outs = append(outs, M{"tag": "direct", "type": "direct"})
	}
	for _, n := range a.cfg.Nodes {
		if !n.Enabled || n.Engine != engine {
			continue
		}
		if engine == "sing-box" && key != "sing-box" && n.coreKey() != key {
			continue
		}
		ports = append(ports, n.Port)
		tag := "node-" + n.ID
		in := "in-" + n.ID
		b, _ := json.Marshal(n.Outbound)
		var out M
		_ = json.Unmarshal(b, &out)
		out["tag"] = tag
		outs = append(outs, out)
		mode := a.effectiveRoute(n)
		if mode != "proxy" {
			splitRoutes = true
		}
		bypassLAN := a.bypassLANEnabled()
		if engine == "xray" {
			ins = append(ins, M{"tag": in, "listen": "127.0.0.1", "port": n.Port, "protocol": "mixed", "settings": M{"auth": "noauth", "udp": true}})
			if n.AppPort > 0 {
				appIn := "app-in-" + n.ID
				ports = append(ports, n.AppPort)
				ins = append(ins, M{"tag": appIn, "listen": "127.0.0.1", "port": n.AppPort, "protocol": "mixed", "settings": M{"auth": "password", "accounts": []any{M{"user": "mirroxy-app", "pass": a.cfg.AppRouting.Secret}}, "udp": true}})
				globalRules = append(globalRules, xrayNodeRules(appIn, tag, mode, bypassLAN)...)
			}
			if mode == "proxy" {
				globalRules = append(globalRules, xrayNodeRules(in, tag, mode, bypassLAN)...)
			} else {
				rules = append(rules, xrayNodeRules(in, tag, mode, bypassLAN)...)
			}
		} else {
			ins = append(ins, M{"type": "mixed", "tag": in, "listen": "127.0.0.1", "listen_port": n.Port})
			if n.AppPort > 0 {
				appIn := "app-in-" + n.ID
				ports = append(ports, n.AppPort)
				ins = append(ins, M{"type": "mixed", "tag": appIn, "listen": "127.0.0.1", "listen_port": n.AppPort, "users": []any{M{"username": "mirroxy-app", "password": a.cfg.AppRouting.Secret}}})
				rules = append(rules, singNodeRules(appIn, tag, mode, ruleSets, bypassLAN)...)
			}
			rules = append(rules, singNodeRules(in, tag, mode, ruleSets, bypassLAN)...)
		}
	}
	if engine == "xray" {
		strategy := "AsIs"
		if splitRoutes {
			strategy = "IPOnDemand"
		}
		cfg := M{"log": M{"loglevel": first(a.cfg.LogLevel, "warning"), "access": "none"}, "inbounds": ins, "outbounds": outs, "routing": M{"domainStrategy": strategy, "rules": append(globalRules, rules...)}}
		if p := a.metricsPorts[key]; p > 0 {
			cfg["metrics"] = M{"listen": fmt.Sprintf("127.0.0.1:%d", p)}
			cfg["stats"] = M{}
			cfg["policy"] = M{"system": M{"statsInboundUplink": true, "statsInboundDownlink": true}}
		}
		return cfg, ports
	}
	rules = append(rules, M{"action": "reject"})
	route := M{"rules": rules, "default_domain_resolver": "local"}
	if len(ruleSets) > 0 {
		route["rule_set"] = a.singRuleSets(ruleSets)
	}
	level := first(a.cfg.LogLevel, "warning")
	if level == "warning" {
		level = "warn"
	}
	cfg := M{"log": M{"level": level}, "inbounds": ins, "outbounds": outs, "dns": M{"servers": []any{M{"type": "local", "tag": "local"}}}, "route": route}
	if p := a.metricsPorts[key]; p > 0 {
		cfg["experimental"] = M{"clash_api": M{"external_controller": fmt.Sprintf("127.0.0.1:%d", p), "secret": a.metricsSecret}}
	}
	return cfg, ports
}
func (a *App) validateCore(engine, path, exe string) error {
	engine = coreEngine(engine)
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("%s 不存在，请使用完整安装包或在面板更新", coreDisplayName(engine))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := []string{"run", "-test", "-config", path}
	if engine == "sing-box" {
		args = []string{"check", "-c", path}
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	hideCommand(cmd)
	cmd.Env = a.coreEnv()
	cmd.Dir = a.root
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 1500 {
			msg = msg[len(msg)-1500:]
		}
		return fmt.Errorf("%s 配置校验失败：%s", coreDisplayName(engine), msg)
	}
	return nil
}
func (a *App) launch(key, path, hash string, ports []int) (*Core, error) {
	engine := coreEngine(key)
	args := []string{"run", "-config", path}
	if engine == "sing-box" {
		args = []string{"run", "-c", path}
	}
	cmd := exec.Command(a.corePath(engine), args...)
	hideCommand(cmd)
	cmd.Env = a.coreEnv()
	cmd.Dir = a.root
	runtimeDir := a.dataPath("runtime")
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		return nil, err
	}
	logPath := filepath.Join(runtimeDir, "Mirroxy_worker_"+runtimeFileID(key)+".log")
	if st, e := os.Stat(logPath); e == nil && st.Size() > 2<<20 {
		_ = os.Rename(logPath, logPath+".previous")
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	cmd.Stdout = f
	cmd.Stderr = f
	if err = cmd.Start(); err != nil {
		f.Close()
		return nil, err
	}
	assignJob(a.job, cmd.Process.Pid)
	c := &Core{cmd: cmd, done: make(chan struct{}), hash: hash, log: f}
	go func() { c.err = cmd.Wait(); f.Close(); close(c.done) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !c.alive() {
			return nil, fmt.Errorf("%s 退出，请查看运行日志", coreDisplayName(engine))
		}
		ready := true
		for _, p := range ports {
			conn, e := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", p), 100*time.Millisecond)
			if e != nil {
				ready = false
				break
			}
			conn.Close()
		}
		if ready {
			return c, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	c.stop()
	return nil, errors.New("代理端口启动超时，请检查是否被其他程序占用")
}
func (a *App) reconcile() error {
	return a.reconcileEngine("")
}
func (a *App) reconcileEngine(only string) error {
	type pending struct {
		engine, path, hash string
		ports              []int
	}
	todo := []pending{}
	cleanup := []string{}
	defer func() {
		for _, path := range cleanup {
			_ = os.Remove(path)
		}
	}()
	keys := a.coreKeys()
	for _, engine := range keys {
		if only != "" && coreEngine(engine) != only {
			continue
		}
		if err := a.ensureMetricsPort(engine); err != nil {
			return err
		}
		cfg, ports := a.coreConfig(engine)
		b, _ := json.Marshal(cfg)
		hash := string(b)
		c := a.cores[engine]
		if len(ports) > 0 && c.alive() && c.hash == hash {
			continue
		}
		path := a.dataPath(filepath.Join("runtime", "Mirroxy_session_"+runtimeFileID(engine)+".json"))
		candidate := strings.TrimSuffix(path, ".json") + ".pending.json"
		cleanup = append(cleanup, path, candidate)
		if len(ports) > 0 {
			if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
				return e
			}
			_ = os.Remove(path)
			_ = os.Remove(candidate)
			if e := writeJSON(candidate, cfg); e != nil {
				return e
			}
			if e := a.validateCore(engine, candidate, a.corePath(engine)); e != nil {
				return e
			}
		}
		todo = append(todo, pending{engine, path, hash, ports})
	}
	for _, p := range todo {
		a.sampleTrafficKey(p.engine)
		a.cores[p.engine].stop()
		delete(a.cores, p.engine)
		if len(p.ports) == 0 {
			continue
		}
		candidate := strings.TrimSuffix(p.path, ".json") + ".pending.json"
		if err := os.Rename(candidate, p.path); err != nil {
			return err
		}
		c, err := a.launch(p.engine, p.path, p.hash, p.ports)
		if err != nil {
			return err
		}
		a.cores[p.engine] = c
	}
	return nil
}
func (a *App) restartEngine(engine string) error {
	a.stopEngine(engine)
	return a.reconcile()
}

func (a *App) coreKeys() []string {
	keys := []string{"xray"}
	seen := map[string]bool{"xray": true}
	for _, n := range a.cfg.Nodes {
		if !seen[n.coreKey()] {
			seen[n.coreKey()] = true
			keys = append(keys, n.coreKey())
		}
	}
	for k := range a.cores {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	return keys
}
func (a *App) stopEngine(engine string) {
	for k, c := range a.cores {
		if k == engine || coreEngine(k) == engine {
			a.sampleTrafficKey(k)
			c.stop()
			delete(a.cores, k)
		}
	}
}
func (a *App) changeNode(id, action, route string) error {
	if action == "move-bottom" || action == "auto-order" {
		return a.setNodeListBottom(id, action == "move-bottom")
	}
	if a.busyUpdate {
		return errors.New("核心更新中，请稍后")
	}
	before := cloneSettings(a.cfg)
	n := a.node(id)
	if n == nil {
		return errors.New("节点不存在")
	}
	switch action {
	case "start":
		if n.Port == 0 {
			p, e := a.allocatePort()
			if e != nil {
				return e
			}
			n.Port = p
		} else if !n.Enabled && !portAvailable(n.Port) {
			return fmt.Errorf("固定端口 %d 已被占用；请关闭占用程序或重新分配端口", n.Port)
		}
		if n.AppPort > 0 && !n.Enabled && !portAvailable(n.AppPort) {
			n.AppPort = 0
			p, e := a.allocateAppPort()
			if e != nil {
				a.cfg = before
				return e
			}
			n.AppPort = p
		}
		n.Enabled = true
	case "stop":
		if a.appRouterUsesNode(id) {
			return errors.New("此节点正被应用代理使用，请先停止应用代理")
		}
		n.Enabled = false
	case "route":
		if !validNodeRoute(route) {
			return errors.New("无效路由模式")
		}
		n.Route = route
	case "reassign":
		if n.Enabled {
			return errors.New("请先停止节点再分配端口")
		}
		n.Port = 0
		p, e := a.allocatePort()
		if e != nil {
			a.cfg = before
			return e
		}
		n.Port = p
	case "delete":
		if a.appRouterUsesNode(id) {
			return errors.New("此节点正被应用代理使用，请先停止应用代理")
		}
		for i := range a.cfg.Nodes {
			if a.cfg.Nodes[i].ID == id {
				a.cfg.Nodes = append(a.cfg.Nodes[:i], a.cfg.Nodes[i+1:]...)
				break
			}
		}
	default:
		return errors.New("不支持的操作")
	}
	if err := a.reconcile(); err != nil {
		a.cfg = before
		if e := a.reconcile(); e != nil {
			a.event("恢复旧代理失败：" + e.Error())
		}
		return err
	}
	if err := a.save(); err != nil {
		a.cfg = before
		_ = a.reconcile()
		return err
	}
	a.syncProxy()
	a.event("节点操作完成：" + action)
	return nil
}
