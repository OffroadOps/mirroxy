package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"
)

// Availability is independent from process status and traffic sampling. Unknown
// nodes retain the default green name, but are explicitly labelled pending.
type Health struct {
	Status    string    `json:"status"`
	Checking  bool      `json:"checking"`
	CheckedAt time.Time `json:"checkedAt"`
	LatencyMs int64     `json:"latencyMs"`
	Error     string    `json:"error,omitempty"`
}

type healthRecord struct {
	Health
	revision string
	core     *Core
}

type healthProbe struct {
	node   Node
	record *healthRecord
	route  string
}

func healthRevision(n Node) string {
	return fmt.Sprintf("%s/%t/%d/%s", fingerprint(n), n.Enabled, n.Port, n.Route)
}

// Caller holds App.mu. Never publish results for an obsolete node/core instance.
func (a *App) healthFor(n Node) Health {
	h := a.health[n.ID]
	if h == nil || h.revision != healthRevision(n) || (n.Enabled && h.core != a.cores[n.coreKey()]) {
		return Health{Status: "unknown"}
	}
	return h.Health
}

func (a *App) healthState() M {
	nodes := map[string]Health{}
	online, offline, unknown, checking := 0, 0, 0, 0
	for _, n := range a.cfg.Nodes {
		h := a.healthFor(n)
		nodes[n.ID] = h
		switch h.Status {
		case "online":
			online++
		case "offline":
			offline++
		default:
			unknown++
		}
		if h.Checking {
			checking++
		}
	}
	return M{"nodes": nodes, "enabled": a.cfg.HealthMonitor, "intervalSeconds": a.cfg.HealthInterval,
		"online": online, "offline": offline, "unknown": unknown, "checking": checking}
}

// Caller holds App.mu. Limit both automatic and manual probes together, and
// reserve the node before releasing the lock so a slow probe cannot overlap.
func (a *App) beginHealth(id string) (*healthProbe, error) {
	select {
	case <-a.shutdown:
		return nil, errors.New("后台正在退出")
	default:
	}
	n := a.node(id)
	if n == nil {
		return nil, errors.New("节点不存在")
	}
	if a.busyUpdate {
		return nil, errors.New("核心更新中，请稍后检测")
	}
	active := 0
	for key, h := range a.health {
		if h.Checking {
			active++
			if key == id {
				return nil, errors.New("该节点正在检测，请稍后查看结果")
			}
		}
	}
	if active >= 3 {
		return nil, errors.New("正在检测其他节点，请稍后重试")
	}
	if a.health == nil {
		a.health = map[string]*healthRecord{}
	}
	h := &healthRecord{Health: a.healthFor(*n), revision: healthRevision(*n), core: a.cores[n.coreKey()]}
	h.Checking = true
	a.health[id] = h
	// The subscription updater may replace the outbound while the probe is running.
	b, _ := json.Marshal(n)
	var snapshot Node
	_ = json.Unmarshal(b, &snapshot)
	a.healthWorkers.Add(1)
	return &healthProbe{node: snapshot, record: h, route: a.effectiveRoute(snapshot)}, nil
}

func (a *App) finishHealth(p *healthProbe, result Health, err error) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := a.node(p.node.ID)
	h := a.health[p.node.ID]
	if h != p.record {
		return false
	}
	h.Checking = false
	if n == nil {
		delete(a.health, p.node.ID)
		return false
	}
	if errors.Is(err, context.Canceled) || healthRevision(*n) != h.revision || (n.Enabled && a.cores[n.coreKey()] != h.core) {
		return false
	}
	previous := h.Status
	h.Health = result
	if result.Status == "offline" && previous != "offline" {
		a.event("节点不可用：" + n.Name)
	} else if result.Status == "online" && previous == "offline" {
		a.event("节点已恢复：" + n.Name)
	}
	return true
}

func (a *App) healthLoop() {
	defer close(a.healthDone)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		a.mu.Lock()
		if a.cfg.HealthMonitor && !a.busyUpdate {
			for _, n := range a.cfg.Nodes {
				h := a.healthFor(n)
				if h.Checking || (!h.CheckedAt.IsZero() && time.Since(h.CheckedAt) < time.Duration(a.cfg.HealthInterval)*time.Second) {
					continue
				}
				if p, err := a.beginHealth(n.ID); err == nil {
					go a.runHealth(p)
				}
			}
		}
		a.mu.Unlock()
		select {
		case <-a.shutdown:
			// beginHealth rejects new work after shutdown; finish all owned probes.
			a.mu.Lock()
			a.mu.Unlock()
			a.healthWorkers.Wait()
			return
		case <-ticker.C:
		}
	}
}

func (a *App) runHealth(p *healthProbe) (Health, error) {
	defer a.healthWorkers.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go func() {
		select {
		case <-a.shutdown:
			cancel()
		case <-ctx.Done():
		}
	}()
	// A pending core update takes priority. Cancellation must still work while
	// waiting for the updater, so shutdown never waits for a network download.
	for !a.healthGate.TryRLock() {
		select {
		case <-ctx.Done():
			a.finishHealth(p, Health{}, context.Canceled)
			return Health{}, errors.New("检测已取消，请稍后重试")
		case <-time.After(50 * time.Millisecond):
		}
	}
	result, err := a.probeHealth(ctx, p, []string{"https://www.gstatic.com/generate_204", "https://cp.cloudflare.com/generate_204"})
	a.healthGate.RUnlock()
	if !a.finishHealth(p, result, err) {
		return result, errors.New("节点配置或运行状态已变化，请重新检测")
	}
	return result, err
}

func (a *App) probeHealth(ctx context.Context, p *healthProbe, targets []string) (Health, error) {
	port := p.node.Port
	var err error
	if p.node.Enabled && !p.record.core.alive() {
		err = errors.New("节点核心未运行")
	} else if !p.node.Enabled || first(p.route, p.node.Route) != "proxy" {
		var cleanup func()
		port, cleanup, err = a.temporaryHealthCore(ctx, p.node)
		if cleanup != nil {
			defer cleanup()
		}
	}
	latency := int64(0)
	if err == nil {
		latency, err = checkProxyHealth(ctx, port, targets)
	}
	h := Health{Status: "online", CheckedAt: time.Now(), LatencyMs: latency}
	if ctx.Err() == context.Canceled {
		return h, context.Canceled
	}
	if err != nil {
		h.Status = "offline"
		h.Error = err.Error()
	}
	return h, err
}

// An open TCP port alone is not evidence of working Internet access. Require a
// 204 response through the chosen node, then retry an independent endpoint.
func checkProxyHealth(ctx context.Context, port int, targets []string) (int64, error) {
	u, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	transport := &http.Transport{Proxy: http.ProxyURL(u), DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, target := range targets {
		if ctx.Err() != nil {
			break
		}
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			continue
		}
		res, err := client.Do(req)
		if err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusNoContent {
				return time.Since(start).Milliseconds(), nil
			}
		}
	}
	if ctx.Err() == context.Canceled {
		return 0, context.Canceled
	}
	return 0, errors.New("主、备用站点均未通过联网检测")
}

// A stopped node or a node with bypass rules is tested in an isolated temporary
// core, forced through its outbound. Saved ports, enabled state, routing and the
// live shared Xray process are never modified by a probe.
func (a *App) temporaryHealthCore(ctx context.Context, n Node) (int, func(), error) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, nil, errors.New("无法分配临时检测端口")
	}
	n.Port = l.Addr().(*net.TCPAddr).Port
	l.Close()
	n.Enabled, n.Route = true, "proxy"
	probeApp := &App{root: a.root, cfg: Settings{Nodes: []Node{n}, LogLevel: "error"}}
	cfg, _ := probeApp.coreConfig(n.coreKey())
	runtimeDir := a.dataPath("runtime")
	if err = os.MkdirAll(runtimeDir, 0700); err != nil {
		return 0, nil, errors.New("无法创建临时检测目录")
	}
	f, err := os.CreateTemp(runtimeDir, "Mirroxy_probe_*.json")
	if err != nil {
		return 0, nil, errors.New("无法创建临时检测配置")
	}
	path := f.Name()
	err = json.NewEncoder(f).Encode(cfg)
	f.Close()
	if err != nil {
		os.Remove(path)
		return 0, nil, errors.New("无法写入临时检测配置")
	}
	args := []string{"run", "-config", path}
	if n.Engine == "sing-box" {
		args = []string{"run", "-c", path}
	}
	cmd := exec.CommandContext(ctx, a.corePath(n.Engine), args...)
	hideCommand(cmd)
	cmd.Dir, cmd.Env = a.root, a.coreEnv()
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err = cmd.Start(); err != nil {
		os.Remove(path)
		return 0, nil, errors.New("无法启动临时检测核心，请检查核心是否已安装")
	}
	assignJob(a.job, cmd.Process.Pid)
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	cleanup := func() { _ = cmd.Process.Kill(); <-done; _ = os.Remove(path) }
	deadline := time.NewTimer(4 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return 0, cleanup, ctx.Err()
		case <-done:
			return 0, cleanup, errors.New("临时检测核心退出，请检查节点配置")
		case <-deadline.C:
			return 0, cleanup, errors.New("临时检测核心启动超时")
		case <-tick.C:
			conn, e := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", n.Port), 100*time.Millisecond)
			if e == nil {
				conn.Close()
				_ = os.Remove(path)
				return n.Port, cleanup, nil
			}
		}
	}
}
