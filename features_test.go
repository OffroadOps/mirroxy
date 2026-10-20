package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalAIAndRemoteAuthentication(t *testing.T) {
	a := &App{root: t.TempDir(), panelPort: 15060, token: "ui-token", cores: map[string]*Core{}, shutdown: make(chan struct{})}
	if e := a.load(); e != nil {
		t.Fatal(e)
	}
	a.cfg.AutoDeploy = false
	a.cfg.Remote = RemoteSettings{Username: "tester", Salt: "test-salt", PasswordHash: passwordHash("strong-test-password", "test-salt")}
	request := func(remote bool, method, path, body, peer, auth, origin, forwarded string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:15060"+path, strings.NewReader(body))
		r.RemoteAddr = peer
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if forwarded != "" {
			r.Header.Set("X-Forwarded-For", forwarded)
		}
		if auth != "" {
			r.SetBasicAuth("tester", auth)
		}
		w := httptest.NewRecorder()
		a.handler(remote).ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		remote                        bool
		peer, auth, origin, forwarded string
		want                          int
	}{
		{false, "127.0.0.1:1234", "", "", "", 200},
		{false, "192.0.2.2:1234", "", "", "", 401},
		{true, "127.0.0.1:1234", "", "", "", 401},
		{true, "192.0.2.2:1234", "wrong-password", "", "", 401},
		{true, "192.0.2.2:1234", "strong-test-password", "", "", 200},
		{false, "127.0.0.1:1234", "", "", "127.0.0.1", 401},
		{true, "192.0.2.2:1234", "", "", "127.0.0.1", 401},
		{false, "127.0.0.1:1234", "", "https://evil.invalid", "", 403},
	} {
		w := request(tc.remote, "GET", "/api/v1/status", "", tc.peer, tc.auth, tc.origin, tc.forwarded)
		if w.Code != tc.want {
			t.Fatalf("auth case %+v: %d", tc, w.Code)
		}
		if w.Code == 200 && (strings.Contains(w.Body.String(), "ui-token") || strings.Contains(w.Body.String(), a.cfg.Remote.PasswordHash)) {
			t.Fatal("API secret leak")
		}
	}
	a.cfg.AutoDeploy = true
	a.cfg.AutoDownload = false
	w := request(false, "POST", "/api/v1/nodes/import", `{"text":"http://alice:SECRET@example.invalid:1234#Test","deploy":false}`, "127.0.0.1:1234", "", "", "")
	if w.Code != 200 || len(a.cfg.Nodes) != 1 {
		t.Fatalf("local AI mutation needs unexpected auth: %s", w.Body.String())
	}
	var imported struct{ Warnings []string }
	json.Unmarshal(w.Body.Bytes(), &imported)
	if len(imported.Warnings) != 0 || a.cfg.Nodes[0].Enabled {
		t.Fatal("API ignored explicit deploy=false")
	}
	w = request(false, "GET", "/api/v1/nodes", "", "127.0.0.1:1234", "", "", "")
	if strings.Contains(w.Body.String(), "SECRET") || strings.Contains(w.Body.String(), "outbound") {
		t.Fatal("node credentials exposed")
	}
	w = request(true, "POST", "/api/settings", `{}`, "192.0.2.2:1234", "strong-test-password", "", "")
	if w.Code != 403 {
		t.Fatal("remote changed local settings")
	}
	a.cfg.Remote.PasswordHash = passwordHash("changed-test-password", a.cfg.Remote.Salt)
	w = request(true, "GET", "/api/v1/status", "", "192.0.2.2:1234", "strong-test-password", "", "")
	if w.Code != 401 {
		t.Fatal("old cached password still accepted after rotation")
	}
}
func TestRemoteTLSListener(t *testing.T) {
	a := &App{root: t.TempDir(), panelPort: 15060, cores: map[string]*Core{}, shutdown: make(chan struct{})}
	if e := a.load(); e != nil {
		t.Fatal(e)
	}
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	a.cfg.Remote = RemoteSettings{Enabled: true, Port: port, Username: "test", Salt: "salt", PasswordHash: passwordHash("test-password123", "salt")}
	s, listener, e := a.prepareRemote(a.cfg.Remote)
	if e != nil {
		t.Fatal(e)
	}
	a.startRemote(s, listener)
	defer s.Close()
	cert, e := a.remoteCertificate(a.cfg.Remote)
	if e != nil {
		t.Fatal(e)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("missing certificate")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	address := "https://127.0.0.1:" + fmtInt(port) + "/api/v1/nodes"
	res, e := client.Get(address)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("loopback bypassed remote listener authentication")
	}
	req, _ := http.NewRequest("GET", address, nil)
	req.SetBasicAuth("test", "test-password123")
	res, e = client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("remote HTTPS failed %d", res.StatusCode)
	}
}
func TestTrafficPersistenceAndCounterEpoch(t *testing.T) {
	a := &App{root: t.TempDir()}
	if e := a.loadTraffic(); e != nil {
		t.Fatal(e)
	}
	c1, c2 := &Core{}, &Core{}
	s := a.trafficFor("node")
	now := time.Now()
	s.record(c1, 100, 200, now)
	s.record(c1, 300, 700, now.Add(time.Second))
	if s.UploadRate != 200 || s.DownloadRate != 500 {
		t.Fatal("incorrect rate")
	}
	s.record(c2, 50, 70, now.Add(2*time.Second))
	if s.Uploaded != 350 || s.Downloaded != 770 {
		t.Fatal("counter reset lost or duplicated bytes")
	}
	if e := writeJSON(a.dataPath("traffic.json"), a.traffic); e != nil {
		t.Fatal(e)
	}
	b := &App{root: a.root}
	if e := b.loadTraffic(); e != nil {
		t.Fatal(e)
	}
	s = b.trafficFor("node")
	if s.Uploaded != 350 || s.Downloaded != 770 || s.UploadRate != 0 {
		t.Fatal("persisted counters did not survive restart")
	}
}
func TestSingboxTrafficIsolation(t *testing.T) {
	core := os.Getenv("MIRROXY_TEST_SINGBOX")
	if core == "" {
		t.Skip("installed core needed")
	}
	a := &App{root: t.TempDir(), panelPort: 15060, cores: map[string]*Core{}, cfg: Settings{StartPort: 26060, Updated: map[string]string{}}}
	path := a.corePath("sing-box")
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(core)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, b, 0700); e != nil {
		t.Fatal(e)
	}
	installTestRules(t, a)
	defer func() {
		for _, c := range a.cores {
			c.stop()
		}
	}()
	for _, id := range []string{"one", "two"} {
		a.cfg.Nodes = append(a.cfg.Nodes, Node{ID: id, Engine: "sing-box", Protocol: "http", Outbound: M{"type": "direct"}, Route: "proxy"})
		if e = a.changeNode(id, "start", ""); e != nil {
			t.Fatal(e)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Write([]byte(strings.Repeat("a", 8192)))
	}))
	defer server.Close()
	// Use SOCKS5 for a real transfer to a local HTTP origin, including upload payload.
	for i, n := range a.cfg.Nodes {
		conn, e := net.Dial("tcp4", "127.0.0.1:"+fmtInt(n.Port))
		if e != nil {
			t.Fatal(e)
		}
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		conn.Write([]byte{5, 1, 0})
		reply := make([]byte, 2)
		if _, e = io.ReadFull(conn, reply); e != nil {
			t.Fatal(e)
		}
		port := server.Listener.Addr().(*net.TCPAddr).Port
		conn.Write([]byte{5, 1, 0, 1, 127, 0, 0, 1, byte(port >> 8), byte(port)})
		reply = make([]byte, 10)
		if _, e = io.ReadFull(conn, reply); e != nil {
			t.Fatal(e)
		}
		if reply[1] != 0 {
			t.Fatal("SOCKS connect failed")
		}
		a.sampleTraffic()
		if a.trafficFor(n.ID).Connections != 1 {
			t.Fatal("sing-box TCP count incorrect")
		}
		payload := strings.Repeat("b", 1024*(i+1))
		io.WriteString(conn, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: "+fmtInt(len(payload))+"\r\n\r\n"+payload)
		response, e := http.ReadResponse(bufio.NewReader(conn), nil)
		if e != nil {
			t.Fatal(e)
		}
		received, e := io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if e != nil {
			t.Fatal(e)
		}
		if response.StatusCode != 200 || received != 8192 {
			t.Fatal("incomplete proxy response")
		}
		conn.Close()
	}
	a.sampleTraffic()
	one, two := a.trafficFor("one"), a.trafficFor("two")
	if !one.Available || one.Uploaded < 1024 || one.Downloaded < 8192 || two.Uploaded <= one.Uploaded || two.Downloaded < 8192 {
		b, _ := json.Marshal(a.traffic)
		t.Fatalf("incorrect sing-box counters: %s", b)
	}
	before := one.Downloaded
	if e = a.changeNode("two", "stop", ""); e != nil {
		t.Fatal(e)
	}
	a.sampleTraffic()
	if a.trafficFor("one").Downloaded != before {
		t.Fatal("unrelated node counters changed")
	}
}
