package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNodeParserPreservesReality(t *testing.T) {
	n, e := parseLink("vless://11111111-1111-4111-8111-111111111111@example.invalid:443?security=reality&type=tcp&sni=example.org&fp=chrome&pbk=test-public-key&sid=1234&flow=xtls-rprx-vision#Example")
	if e != nil {
		t.Fatal(e)
	}
	stream := n.Outbound["streamSettings"].(M)
	reality := stream["realitySettings"].(M)
	if reality["serverName"] != "example.org" || reality["publicKey"] != "test-public-key" || stream["network"] != "raw" {
		t.Fatal("REALITY transport fields changed")
	}
	for _, link := range []string{"vless://id@example.invalid:443?security=reality", "ss://YWVzLTI1Ni1nY206c2VjcmV0@example.invalid:1234?plugin=obfs-local"} {
		if _, e := parseLink(link); e == nil {
			t.Fatal("Unsupported/incomplete configuration was silently accepted")
		}
	}
}

func TestUpdateNeverDowngrades(t *testing.T) {
	for _, p := range [][2]string{{"v26.3.27", "26.7.28"}, {"v1.13.16", "1.13.16"}, {"v1.9.0", "1.13.16"}} {
		if newerVersion(p[0], p[1]) {
			t.Fatalf("unsafe update: %v", p)
		}
	}
	if !newerVersion("v1.14.0", "1.13.16") {
		t.Fatal("newer release not detected")
	}
}
func TestAPIRejectsCrossSiteAndHidesCredentials(t *testing.T) {
	a := &App{panelPort: 15060, token: "private-token", cores: map[string]*Core{}, cfg: Settings{Nodes: []Node{{ID: "test", Name: "example", Outbound: M{"password": "NEVER-EXPOSE-THIS"}}}}}
	handler := a.guard(http.HandlerFunc(a.api))
	for _, tc := range []struct{ host, origin, token string }{{"evil.invalid", "", "private-token"}, {"127.0.0.1:15060", "https://evil.invalid", "private-token"}, {"127.0.0.1:15060", "", "wrong"}} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:15060/api/node", strings.NewReader(`{}`))
		r.Host = tc.host
		r.RemoteAddr = "127.0.0.1:12345"
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-App-Token", tc.token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("cross-site request accepted: %d", w.Code)
		}
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:15060/api/state", nil)
	r.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), "NEVER-EXPOSE-THIS") {
		t.Fatal("state API leaked credentials")
	}
}

// This integration test exercises the real installed Xray and only talks to local mock proxies.
func TestXrayPortIsolationAndFailure(t *testing.T) {
	corePath := os.Getenv("MIRROXY_TEST_XRAY")
	if corePath == "" {
		t.Skip("set MIRROXY_TEST_XRAY to run the installed-core integration test")
	}
	root := t.TempDir()
	target := filepath.Join(root, "bin", "runtime", executableName("Mirroxy_core_A"))
	if e := os.MkdirAll(filepath.Dir(target), 0700); e != nil {
		t.Fatal(e)
	}
	in, e := os.Open(corePath)
	if e != nil {
		t.Fatal(e)
	}
	defer in.Close()
	out, e := os.Create(target)
	if e != nil {
		t.Fatal(e)
	}
	_, e = io.Copy(out, in)
	out.Close()
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(target, 0700); e != nil {
		t.Fatal(e)
	}
	mock := func(marker string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Logf("mock %s received %s", marker, r.Method)
			if r.Method != "CONNECT" {
				// A plain request is forwarded to the upstream proxy in
				// absolute form instead of being tunnelled.
				w.Header().Set("Content-Length", strconv.Itoa(len(marker)))
				w.WriteHeader(200)
				_, _ = w.Write([]byte(marker))
				return
			}
			conn, rw, e := w.(http.Hijacker).Hijack()
			if e != nil {
				return
			}
			defer conn.Close()
			_, _ = rw.WriteString("HTTP/1.1 200 Connection established\r\n\r\n")
			_ = rw.Flush()
			_, readErr := http.ReadRequest(rw.Reader)
			e = readErr
			if e != nil {
				t.Logf("mock %s inner read: %v", marker, e)
				return
			}
			_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: " + strconv.Itoa(len(marker)) + "\r\nConnection: close\r\n\r\n" + marker)
			_ = rw.Flush()
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _ = io.Copy(io.Discard, conn)
		}))
	}
	firstProxy := mock("alpha")
	defer firstProxy.Close()
	secondProxy := mock("bravo")
	defer secondProxy.Close()
	a := &App{root: root, panelPort: 15060, cores: map[string]*Core{}, cfg: Settings{StartPort: 25060, LogLevel: "debug", Updated: map[string]string{}}}
	a.job = createJob()
	defer closeJob(a.job)
	installTestRules(t, a)
	// The proof host sits in the private geosite category, so the LAN bypass
	// has to be off for the request to reach the upstream proxy.
	bypassLAN := false
	a.cfg.BypassLAN = &bypassLAN
	defer func() {
		if t.Failed() {
			logs, _ := filepath.Glob(a.dataPath(filepath.Join("runtime", "Mirroxy_worker_*.log")))
			for _, logPath := range logs {
				t.Log(string(mustReadFile(logPath)))
			}
		}
	}()
	defer func() {
		for _, c := range a.cores {
			c.stop()
		}
	}()
	for _, address := range []string{firstProxy.URL, secondProxy.URL} {
		n, e := parseLink(address)
		if e != nil {
			t.Fatal(e)
		}
		n.ID = newID()
		a.cfg.Nodes = append(a.cfg.Nodes, n)
		if e = a.changeNode(n.ID, "start", ""); e != nil {
			t.Fatal(e)
		}
	}
	ports := []int{a.cfg.Nodes[0].Port, a.cfg.Nodes[1].Port}
	request := func(port int) (string, error) {
		u, _ := url.Parse("http://" + net.JoinHostPort("127.0.0.1", fmtInt(port)))
		transport := &http.Transport{Proxy: http.ProxyURL(u), DisableKeepAlives: true}
		defer transport.CloseIdleConnections()
		c := &http.Client{Transport: transport, Timeout: 3 * time.Second}
		req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://proof.invalid/", nil)
		// The mock upstream tears its tunnels down on its own schedule, so a
		// dropped connection is retried before it counts as a failure.
		var last error
		for attempt := 0; attempt < 4; attempt++ {
			r, e := c.Do(req)
			if e != nil {
				last = e
				time.Sleep(150 * time.Millisecond)
				continue
			}
			b, e := io.ReadAll(r.Body)
			r.Body.Close()
			if e != nil {
				last = e
				time.Sleep(150 * time.Millisecond)
				continue
			}
			return string(b), nil
		}
		return "", last
	}
	for i, want := range []string{"alpha", "bravo"} {
		got, e := request(ports[i])
		if e != nil || got != want {
			time.Sleep(250 * time.Millisecond)
			t.Fatalf("wrong upstream on port %d: %q %v", ports[i], got, e)
		}
	}
	a.sampleTraffic()
	for _, n := range a.cfg.Nodes {
		s := a.trafficFor(n.ID)
		if !s.Available || s.Uploaded == 0 || s.Downloaded == 0 {
			t.Fatalf("missing native counters: %+v", s)
		}
	}
	baseline := a.trafficFor(a.cfg.Nodes[1].ID).Downloaded
	conn, e := net.Dial("tcp4", fmt.Sprintf("127.0.0.1:%d", ports[1]))
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	// Dial can return before the core accepts the socket and owns its fd.
	// /proc correctly excludes not-yet-accepted sockets from per-PID counts.
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		a.sampleTraffic()
		if a.trafficFor(a.cfg.Nodes[1].ID).Connections > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if a.trafficFor(a.cfg.Nodes[1].ID).Connections < 1 {
		t.Fatal("active inbound TCP connection not counted")
	}
	conn.Close()
	firstProxy.Close()
	if got, e := request(ports[0]); e == nil && (got == "alpha" || got == "bravo") {
		t.Fatal("failed upstream used fallback")
	}
	if got, e := request(ports[1]); e != nil || got != "bravo" {
		t.Fatalf("healthy node affected: %s %v", got, e)
	}
	id := a.cfg.Nodes[1].ID
	if e = a.changeNode(id, "stop", ""); e != nil {
		t.Fatal(e)
	}
	if e = a.changeNode(id, "start", ""); e != nil {
		t.Fatal(e)
	}
	if a.node(id).Port != ports[1] {
		t.Fatal("port assignment changed after restart")
	}
	a.sampleTraffic()
	if a.trafficFor(id).Downloaded < baseline {
		t.Fatal("traffic total reset after core restart")
	}
	b := &App{root: root}
	if e = b.load(); e != nil {
		t.Fatal(e)
	}
	if b.cfg.Nodes[1].Port != ports[1] || !b.cfg.Nodes[1].Enabled {
		t.Fatal("persistent node state lost")
	}
}
func fmtInt(n int) string { return fmt.Sprintf("%d", n) }

func TestSingboxConfiguration(t *testing.T) {
	core := os.Getenv("MIRROXY_TEST_SINGBOX")
	if core == "" {
		t.Skip("set MIRROXY_TEST_SINGBOX to check installed sing-box")
	}
	a := &App{root: t.TempDir(), cfg: Settings{}}
	installTestRules(t, a)
	for i, link := range []string{"hysteria2://password@127.0.0.1:443?sni=example.org", "tuic://11111111-1111-4111-8111-111111111111:password@127.0.0.1:443?sni=example.org", "anytls://password@127.0.0.1:443?sni=example.org"} {
		n, e := parseLink(link)
		if e != nil {
			t.Fatal(e)
		}
		n.ID = newID()
		n.Enabled = true
		n.Port = 26060 + i
		a.cfg.Nodes = append(a.cfg.Nodes, n)
	}
	cfg, _ := a.coreConfig("sing-box")
	path := a.dataPath("test.json")
	if e := writeJSON(path, cfg); e != nil {
		t.Fatal(e)
	}
	if e := a.validateCore("sing-box", path, core); e != nil {
		t.Fatal(e)
	}
}
