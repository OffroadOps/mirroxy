package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// installTestRules copies the shared routing rule files into the temporary
// run directory so that the real cores can load them. MIRROXY_TEST_RULES is
// documented in the README but was never read by the tests.
func installTestRules(t *testing.T, a *App) {
	t.Helper()
	source := os.Getenv("MIRROXY_TEST_RULES")
	if source == "" {
		return
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(a.root, "bin", "resources")
	if err = os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(source, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if err = os.WriteFile(filepath.Join(target, entry.Name()), payload, 0700); err != nil {
			t.Fatal(err)
		}
	}
}

// The configuration of a node must only describe that node. A shared core
// would put every node into one process, so starting one node would restart
// the core of all the others and drop their connections.
func TestCoreConfigIsScopedToItsOwnNode(t *testing.T) {
	a := &App{cores: map[string]*Core{}, cfg: Settings{Nodes: []Node{
		{ID: "alpha", Engine: "xray", Enabled: true, Port: 5060, Route: "bypass-cn", Outbound: M{"protocol": "freedom"}},
		{ID: "bravo", Engine: "xray", Enabled: true, Port: 5061, Route: "bypass-cn", Outbound: M{"protocol": "freedom"}},
		{ID: "charlie", Engine: "sing-box", Enabled: true, Port: 5062, Route: "bypass-cn", Outbound: M{"type": "direct"}},
	}}}
	a.cfg.RoutingMode = "bypass-cn"
	keys := map[string]bool{}
	for _, n := range a.cfg.Nodes {
		key := n.coreKey()
		if keys[key] {
			t.Fatalf("nodes share the core key %s", key)
		}
		keys[key] = true
	}
	if len(a.coreKeys()) != 3 {
		t.Fatalf("expected one core per node, got %v", a.coreKeys())
	}
	for _, n := range a.cfg.Nodes {
		cfg, ports := a.coreConfig(n.coreKey())
		text := string(mustJSON(t, cfg))
		if !strings.Contains(text, `"in-`+n.ID+`"`) {
			t.Fatalf("%s configuration misses its own inbound: %s", n.ID, text)
		}
		for _, other := range a.cfg.Nodes {
			if other.ID == n.ID {
				continue
			}
			if strings.Contains(text, `"in-`+other.ID+`"`) {
				t.Fatalf("%s configuration contains the inbound of %s: %s", n.ID, other.ID, text)
			}
		}
		if len(ports) != 1 || ports[0] != n.Port {
			t.Fatalf("%s waits for the wrong ports: %v", n.ID, ports)
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// proxyThrough sends a request through the local proxy port of a node. The
// loopback path of a busy machine occasionally drops a connection, so a
// failure is retried before it counts.
func proxyThrough(t *testing.T, port int, target string) error {
	t.Helper()
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		client := &http.Client{
			Transport: &http.Transport{Proxy: func(*http.Request) (*url.URL, error) {
				return url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
			}},
			Timeout: 10 * time.Second,
		}
		response, err := client.Get(target)
		if err != nil {
			last = err
			time.Sleep(200 * time.Millisecond)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			last = fmt.Errorf("unexpected status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
			time.Sleep(200 * time.Millisecond)
			continue
		}
		return nil
	}
	return last
}

// Starting a second node must not disturb a node that is already running.
func TestStartingOneNodeKeepsAnotherNodeConnected(t *testing.T) {
	binary := os.Getenv("MIRROXY_TEST_XRAY")
	if binary == "" {
		t.Skip("set MIRROXY_TEST_XRAY for native core integration")
	}
	a := &App{root: t.TempDir(), cores: map[string]*Core{}, shutdown: make(chan struct{})}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(a.corePath("xray")), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(a.corePath("xray"), payload, 0700); err != nil {
		t.Fatal(err)
	}
	installTestRules(t, a)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer origin.Close()
	firstPort, err := a.allocatePort()
	if err != nil {
		t.Fatal(err)
	}
	a.cfg.Nodes = []Node{
		{ID: "alpha", Name: "alpha", Engine: "xray", Enabled: true, Port: firstPort, Route: "proxy", Outbound: M{"protocol": "freedom"}},
	}
	secondPort, err := a.allocatePort()
	if err != nil {
		t.Fatal(err)
	}
	a.cfg.Nodes = append(a.cfg.Nodes, Node{ID: "bravo", Name: "bravo", Engine: "xray", Enabled: false, Port: secondPort, Route: "proxy", Outbound: M{"protocol": "freedom"}})
	if err = a.reconcile(); err != nil {
		t.Fatal(err)
	}
	defer a.stopEngine("xray")
	defer func() {
		if t.Failed() {
			logs, _ := filepath.Glob(a.dataPath(filepath.Join("runtime", "Mirroxy_worker_*.log")))
			for _, path := range logs {
				t.Log(string(mustReadFile(path)))
			}
		}
	}()
	firstCore := a.cores[a.cfg.Nodes[0].coreKey()]
	if firstCore == nil || !firstCore.alive() {
		t.Fatal("the first node is not running")
	}
	if err = proxyThrough(t, firstPort, origin.URL); err != nil {
		t.Fatal("the first node does not serve traffic: " + err.Error())
	}
	if err = a.startNode("bravo"); err != nil {
		t.Fatal(err)
	}
	if a.cores[a.cfg.Nodes[0].coreKey()] != firstCore || !firstCore.alive() {
		t.Fatal("starting the second node restarted the core of the first node")
	}
	if err = proxyThrough(t, firstPort, origin.URL); err != nil {
		t.Fatal("the running node lost its traffic: " + err.Error())
	}
}
