package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthFailureFallbackAndRecovery(t *testing.T) {
	var mode atomic.Int32
	var calls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !r.URL.IsAbs() {
			t.Error("request did not go through HTTP proxy")
		}
		if mode.Load() == 1 || (mode.Load() == 0 && r.URL.Host == "primary.invalid") {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	}))
	defer proxy.Close()
	port := proxy.Listener.Addr().(*net.TCPAddr).Port
	node := Node{ID: "one", Engine: "xray", Enabled: true, Port: port, Route: "proxy", Outbound: M{"protocol": "freedom"}}
	a := &App{cores: map[string]*Core{node.coreKey(): {done: make(chan struct{})}}, shutdown: make(chan struct{}), cfg: Settings{Nodes: []Node{node}}}
	targets := []string{"http://primary.invalid/generate_204", "http://secondary.invalid/generate_204"}
	for _, step := range []struct {
		mode     int32
		status   string
		requests int32
		most     int32
	}{{0, "online", 2, 4}, {1, "offline", 2, 4}, {2, "online", 1, 2}} {
		mode.Store(step.mode)
		calls.Store(0)
		p, err := a.beginHealth("one")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = a.beginHealth("one"); err == nil {
			t.Fatal("overlapping probe accepted")
		}
		h, probeErr := a.probeHealth(context.Background(), p, targets)
		if !a.finishHealth(p, h, probeErr) {
			t.Fatal("current probe discarded")
		}
		a.healthWorkers.Done()
		// Every target is tried, and a dropped connection may be repeated
		// once, so the number of requests is checked as a range.
		if h.Status != step.status || calls.Load() < step.requests || calls.Load() > step.most {
			t.Fatalf("mode %d: health %+v, requests %d", step.mode, h, calls.Load())
		}
		if h.CheckedAt.IsZero() || a.healthFor(a.cfg.Nodes[0]).Checking {
			t.Fatal("probe did not complete")
		}
	}
	if len(a.events) != 2 {
		t.Fatalf("expected failure and recovery events, got %v", a.events)
	}
	// Neither cancelled probes nor results from a replaced core may paint a node red.
	p, _ := a.beginHealth("one")
	if a.finishHealth(p, Health{Status: "offline"}, context.Canceled) {
		t.Fatal("cancellation applied")
	}
	a.healthWorkers.Done()
	if a.healthFor(a.cfg.Nodes[0]).Status != "online" {
		t.Fatal("cancellation overwrote health")
	}
	p, _ = a.beginHealth("one")
	a.cores[a.cfg.Nodes[0].coreKey()] = &Core{done: make(chan struct{})}
	if a.finishHealth(p, Health{Status: "offline"}, errors.New("old core")) {
		t.Fatal("stale core result applied")
	}
	a.healthWorkers.Done()
	if a.healthFor(a.cfg.Nodes[0]).Status != "unknown" {
		t.Fatal("new core reused old health")
	}
}

func TestHealthDefaultsLimitsAndAggregation(t *testing.T) {
	a := &App{root: t.TempDir(), shutdown: make(chan struct{})}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}
	if !a.cfg.HealthMonitor || a.cfg.HealthInterval != 60 {
		t.Fatal("monitor defaults missing")
	}
	for i := 0; i < 4; i++ {
		a.cfg.Nodes = append(a.cfg.Nodes, Node{ID: string(rune('a' + i)), Engine: "xray", Outbound: M{"protocol": "freedom"}})
	}
	probes := []*healthProbe{}
	for i, n := range a.cfg.Nodes {
		p, e := a.beginHealth(n.ID)
		if i < 3 {
			if e != nil {
				t.Fatal(e)
			}
			probes = append(probes, p)
		} else if e == nil {
			t.Fatal("concurrency limit bypassed")
		}
	}
	for _, p := range probes {
		a.finishHealth(p, Health{}, context.Canceled)
		a.healthWorkers.Done()
	}
	n := a.cfg.Nodes[0]
	a.health[n.ID] = &healthRecord{Health: Health{Status: "offline", CheckedAt: time.Now()}, revision: healthRevision(n)}
	a.cfg.Nodes[0].Outbound = M{"protocol": "blackhole"}
	if a.healthFor(a.cfg.Nodes[0]).Status != "unknown" {
		t.Fatal("edited node reused old failure")
	}
	a.trafficFor("a").Uploaded = 100
	a.trafficFor("a").Downloaded = 400
	a.trafficFor("a").UploadRate = 12
	a.trafficFor("a").DownloadRate = 30
	a.trafficFor("b").Uploaded = 200
	a.trafficFor("b").Downloaded = 500
	a.trafficFor("b").UploadRate = 7
	a.trafficFor("b").DownloadRate = 20
	totals := a.trafficState()
	if totals["uploadedBytes"] != uint64(300) || totals["downloadedBytes"] != uint64(900) || totals["uploadBytesPerSecond"] != float64(19) || totals["downloadBytesPerSecond"] != float64(50) {
		t.Fatal("incorrect all-node totals")
	}
	if totals["health"].(M)["unknown"] != 4 {
		t.Fatal("health summary incorrect")
	}
	interval := 5
	if a.saveSettings(SettingsPatch{HealthInterval: &interval}) == nil || a.cfg.HealthInterval != 60 {
		t.Fatal("invalid interval accepted or not rolled back")
	}
	a.requestQuit()
	if _, e := a.beginHealth("a"); e == nil {
		t.Fatal("probe accepted after shutdown")
	}
}

func TestHealthCancellationWhileUpdaterBusy(t *testing.T) {
	a := &App{shutdown: make(chan struct{}), cfg: Settings{Nodes: []Node{{ID: "one"}}}}
	p, _ := a.beginHealth("one")
	a.healthGate.Lock()
	defer a.healthGate.Unlock()
	done := make(chan struct{})
	go func() { a.runHealth(p); close(done) }()
	a.requestQuit()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown blocked on core updater")
	}
	if a.healthFor(a.cfg.Nodes[0]).Status != "unknown" {
		t.Fatal("cancellation changed node health")
	}
}

func TestHealthTemporaryCoresPreserveRunningNodes(t *testing.T) {
	for _, engine := range []string{"xray", "sing-box"} {
		t.Run(engine, func(t *testing.T) {
			env := "MIRROXY_TEST_XRAY"
			if engine == "sing-box" {
				env = "MIRROXY_TEST_SINGBOX"
			}
			core := os.Getenv(env)
			if core == "" {
				t.Skip("set " + env + " for native core integration")
			}
			a := &App{root: t.TempDir(), cores: map[string]*Core{}, shutdown: make(chan struct{})}
			if e := a.load(); e != nil {
				t.Fatal(e)
			}
			b, e := os.ReadFile(core)
			if e != nil {
				t.Fatal(e)
			}
			if e = os.MkdirAll(filepath.Dir(a.corePath(engine)), 0700); e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(a.corePath(engine), b, 0700); e != nil {
				t.Fatal(e)
			}
			installTestRules(t, a)
			out := M{"protocol": "freedom"}
			if engine == "sing-box" {
				out = M{"type": "direct"}
			}
			port, e := a.allocatePort()
			if e != nil {
				t.Fatal(e)
			}
			a.cfg.Nodes = []Node{{ID: "live", Engine: engine, Enabled: true, Port: port, Route: "proxy", Outbound: out}, {ID: "paused", Engine: engine, Route: "bypass-cn", Outbound: out}}
			if e = a.reconcile(); e != nil {
				t.Fatal(e)
			}
			defer a.stopEngine(engine)
			before := cloneSettings(a.cfg)
			live := a.cores[a.cfg.Nodes[0].coreKey()]
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
			defer server.Close()
			p, e := a.beginHealth("paused")
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			h, e := a.probeHealth(ctx, p, []string{server.URL})
			a.finishHealth(p, h, e)
			a.healthWorkers.Done()
			if e != nil || h.Status != "online" {
				t.Fatalf("stopped node not tested: %+v %v", h, e)
			}
			if !reflect.DeepEqual(a.cfg, before) || !live.alive() || a.cores[a.cfg.Nodes[0].coreKey()] != live {
				t.Fatal("probe changed live proxy or saved settings")
			}
			files, _ := filepath.Glob(a.dataPath(filepath.Join("runtime", "Mirroxy_*.*json")))
			if len(files) != 0 {
				t.Fatal("temporary credentials left on disk")
			}
			// A bypass node is forced through its own broken outbound, never allowed
			// to report success just because the local test target could go direct.
			bad := Node{ID: "broken", Engine: engine, Route: "bypass-cn", Outbound: M{"protocol": "blackhole"}}
			if engine == "sing-box" {
				bad.Outbound = M{"type": "http", "server": "127.0.0.1", "server_port": 1}
			}
			a.cfg.Nodes = append(a.cfg.Nodes, bad)
			p, _ = a.beginHealth("broken")
			h, e = a.probeHealth(ctx, p, []string{server.URL})
			a.finishHealth(p, h, e)
			a.healthWorkers.Done()
			if e == nil || h.Status != "offline" {
				t.Fatal("failed outbound passed via direct route")
			}
			exported, _ := json.Marshal(a.healthState())
			if len(exported) == 0 {
				t.Fatal("health API missing")
			}
		})
	}
}
