package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func downloadTestApp(t *testing.T) *App {
	t.Helper()
	return &App{root: t.TempDir(), cores: map[string]*Core{}, shutdown: make(chan struct{})}
}
func TestDownloadValidationAndProxyIsolation(t *testing.T) {
	a := downloadTestApp(t)
	d, err := a.downloads()
	if err != nil {
		t.Fatal(err)
	}
	base := DownloadSpec{URL: "https://example.invalid/archive.zip", Dir: filepath.Join(a.root, "downloads"), Connections: 8, ProxyMode: "direct"}
	if _, err = d.validate(base); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*DownloadSpec){
		func(s *DownloadSpec) { s.URL = "file:///etc/passwd" }, func(s *DownloadSpec) { s.Name = "../Mirroxy.exe" }, func(s *DownloadSpec) { s.Dir = a.root }, func(s *DownloadSpec) { s.Dir = filepath.Join(a.root, "data") }, func(s *DownloadSpec) { s.Connections = 17 }, func(s *DownloadSpec) { s.LimitKiB = -1 }, func(s *DownloadSpec) { s.ProxyMode = "custom"; s.ProxyURL = "socks5://127.0.0.1:10808" }, func(s *DownloadSpec) { s.ProxyMode = "custom"; s.ProxyURL = "http://localhost:80/path" },
	} {
		spec := base
		change(&spec)
		if _, err = d.validate(spec); err == nil {
			t.Fatalf("accepted invalid spec: %#v", spec)
		}
	}
	task := &DownloadTask{DownloadSpec: base}
	opts, err := d.options(task)
	if err != nil {
		t.Fatal(err)
	}
	if opts["all-proxy"] != "" || opts["http-proxy"] != "" || opts["no-proxy"] != "" || opts["check-certificate"] != "true" {
		t.Fatal(opts)
	}
	task.ProxyMode = "node"
	task.NodeID = "missing"
	if _, err = d.options(task); err == nil {
		t.Fatal("missing node silently fell back to direct")
	}
	t.Setenv("HTTP_PROXY", "http://must-not-inherit.invalid:1234")
	for _, env := range downloadEnv() {
		if strings.HasPrefix(strings.ToLower(env), "http_proxy=") {
			t.Fatal("inherited system/environment proxy")
		}
	}
}
func TestDownloadJournalEncryptedAndRemoteDenied(t *testing.T) {
	a := downloadTestApp(t)
	d, err := a.downloads()
	if err != nil {
		t.Fatal(err)
	}
	d.journal.Tasks = append(d.journal.Tasks, &DownloadTask{ID: "history", DownloadSpec: DownloadSpec{URL: "https://example.invalid/PRIVATE-TOKEN", ProxyMode: "custom", ProxyURL: "http://private-user:PRIVATE-PASSWORD@127.0.0.1:10808"}})
	if err = d.save(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(a.dataPath("Mirroxy.db"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("PRIVATE-TOKEN")) || bytes.Contains(b, []byte("PRIVATE-PASSWORD")) {
		t.Fatal("download journal leaked plaintext")
	}
	a2 := &App{root: a.root}
	d2, err := a2.downloads()
	if err != nil {
		t.Fatal(err)
	}
	if len(d2.journal.Tasks) != 1 || d2.journal.Tasks[0].URL != d.journal.Tasks[0].URL {
		t.Fatal("history not restored")
	}
	w := httptest.NewRecorder()
	a.downloadAPI(w, httptest.NewRequest("GET", "/api/downloads", nil))
	if w.Code != 403 {
		t.Fatal("remote request accepted")
	}
}
func TestDownloadRPCSecretAndErrorRedaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Params []any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.Params[0] != "token:private-secret" {
			t.Error("missing RPC secret")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error":{"code":1,"message":"https://PRIVATE-URL password=PRIVATE-PASSWORD"}}`))
	}))
	defer server.Close()
	d := &DownloadManager{endpoint: server.URL, secret: "private-secret", client: server.Client()}
	err := d.rpc("getVersion", nil)
	if err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("upstream error leaked")
	}
}

type downloadSlowWriter struct{ http.ResponseWriter }

func (w downloadSlowWriter) Write(b []byte) (int, error) {
	time.Sleep(8 * time.Millisecond)
	return w.ResponseWriter.Write(b)
}
func downloadWait(t *testing.T, d *DownloadManager, id string, status string) *DownloadTask {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := d.snapshot()
		if err != nil {
			t.Fatal(err)
		}
		for _, task := range snapshot["tasks"].([]*DownloadTask) {
			if task.ID == id {
				if task.Status == status {
					return task
				}
				if task.Status == "error" {
					t.Fatalf("download failed: %s", task.Error)
				}
			}
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("download did not reach %s", status)
	return nil
}
func TestDownloadRealAria2Lifecycle(t *testing.T) {
	engine := os.Getenv("MIRROXY_TEST_ARIA2")
	if engine == "" {
		t.Skip("set MIRROXY_TEST_ARIA2 to an official aria2 executable for isolated engine integration")
	}
	a := downloadTestApp(t)
	dir := filepath.Join(a.root, "bin", "runtime")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	engineBytes, err := os.ReadFile(engine)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, executableName("Mirroxy_downloader")), engineBytes, 0755); err != nil {
		t.Fatal(err)
	}
	var ranges atomic.Int32
	payload := bytes.Repeat([]byte("Mirroxy-download-test-0123456789\n"), 300000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			ranges.Add(1)
		}
		http.ServeContent(downloadSlowWriter{w}, r, "fixture.bin", time.Unix(1700000000, 0), bytes.NewReader(payload))
	}))
	defer server.Close()
	d, err := a.downloads()
	if err != nil {
		t.Fatal(err)
	}
	defer a.stopDownloads()
	spec := DownloadSpec{URL: server.URL + "/fixture.bin", Name: "fixture.bin", Dir: filepath.Join(a.root, "downloads"), Connections: 4, ProxyMode: "direct"}
	id, err := d.add(spec, true)
	if err != nil {
		t.Fatal(err)
	}
	var global map[string]string
	if err = d.rpc("getGlobalOption", &global); err != nil || global["max-concurrent-downloads"] != "30" {
		t.Fatalf("aria2 concurrent capacity: %v %v", global["max-concurrent-downloads"], err)
	}
	downloadWait(t, d, id, "paused")
	if err = d.action(id, "resume", DownloadSpec{}); err != nil {
		t.Fatal(err)
	}
	downloadWait(t, d, id, "active")
	if err = d.action(id, "reconnect", DownloadSpec{}); err != nil {
		t.Fatal(err)
	}
	downloadWait(t, d, id, "active")
	if err = d.action(id, "pause", DownloadSpec{}); err != nil {
		t.Fatal(err)
	}
	downloadWait(t, d, id, "paused")
	spec.Connections = 8
	spec.LimitKiB = 4096
	if err = d.action(id, "edit", spec); err != nil {
		t.Fatal(err)
	}
	if err = d.action(id, "resume", DownloadSpec{}); err != nil {
		t.Fatal(err)
	}
	completed := downloadWait(t, d, id, "complete")
	actual, err := os.ReadFile(completed.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, payload) {
		t.Fatal("segmented download content differs")
	}
	if ranges.Load() < 2 {
		t.Fatalf("multiconnection was not exercised: %d ranges", ranges.Load())
	}
	// A new proxy task must use its selected HTTP proxy, even for a target that
	// has no real DNS entry; its authentication remains in the encrypted journal.
	var proxyAuth atomic.Bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			proxyAuth.Store(true)
		}
		http.ServeContent(w, r, "proxy.bin", time.Unix(1700000000, 0), bytes.NewReader([]byte("private-proxy-download")))
	}))
	defer proxy.Close()
	proxySpec := DownloadSpec{URL: "http://download-fixture.invalid/proxy.bin", Name: "proxy.bin", Dir: spec.Dir, Connections: 2, ProxyMode: "custom", ProxyURL: strings.Replace(proxy.URL, "http://", "http://test-user:test-password@", 1)}
	proxyID, err := d.add(proxySpec, false)
	if err != nil {
		t.Fatal(err)
	}
	proxied := downloadWait(t, d, proxyID, "complete")
	actual, err = os.ReadFile(proxied.Path)
	if err != nil || string(actual) != "private-proxy-download" || !proxyAuth.Load() {
		t.Fatal("custom authenticated HTTP proxy not exercised", err)
	}
	a.stopDownloads()
	a2 := &App{root: a.root, cores: map[string]*Core{}}
	d2, err := a2.downloads()
	if err != nil {
		t.Fatal(err)
	}
	defer a2.stopDownloads()
	snapshot, err := d2.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["tasks"].([]*DownloadTask)[0].Status != "complete" {
		t.Fatal("completed history lost on restart")
	}
	if err = d2.action(id, "remove", DownloadSpec{}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(completed.Path); err != nil {
		t.Fatal("remove deleted downloaded file")
	}
}

func TestDownloadRealAria2RestartPartial(t *testing.T) {
	engine := os.Getenv("MIRROXY_TEST_ARIA2")
	if engine == "" {
		t.Skip("set MIRROXY_TEST_ARIA2")
	}
	a := downloadTestApp(t)
	dir := filepath.Join(a.root, "bin", "runtime")
	_ = os.MkdirAll(dir, 0700)
	b, err := os.ReadFile(engine)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, executableName("Mirroxy_downloader")), b, 0755); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("restart-fixture-1234567890\n"), 300000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(downloadSlowWriter{w}, r, "resume.bin", time.Unix(1700000000, 0), bytes.NewReader(payload))
	}))
	defer server.Close()
	d, err := a.downloads()
	if err != nil {
		t.Fatal(err)
	}
	defer a.stopDownloads()
	id, err := d.add(DownloadSpec{URL: server.URL + "/resume.bin", Name: "resume.bin", Dir: filepath.Join(a.root, "downloads"), Connections: 4, ProxyMode: "direct"}, false)
	if err != nil {
		t.Fatal(err)
	}
	downloadWait(t, d, id, "active")
	time.Sleep(150 * time.Millisecond)
	a.stopDownloads()
	a2 := &App{root: a.root, cores: map[string]*Core{}}
	d2, err := a2.downloads()
	if err != nil {
		t.Fatal(err)
	}
	defer a2.stopDownloads()
	if !d2.journal.Tasks[0].Wanted {
		t.Fatal("running intent not preserved")
	}
	d2.mu.Lock()
	err = d2.start()
	d2.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	complete := downloadWait(t, d2, id, "complete")
	actual, err := os.ReadFile(complete.Path)
	if err != nil || !bytes.Equal(actual, payload) {
		t.Fatal("restarted partial download differs", err)
	}
}

func TestDownloadAPIWriteGuard(t *testing.T) {
	a := downloadTestApp(t)
	a.panelPort = 15060
	a.token = "panel-token"
	for _, test := range []struct {
		token, origin, peer string
		code                int
	}{{"", "", "127.0.0.1:1000", 403}, {"panel-token", "https://evil.invalid", "127.0.0.1:1000", 403}, {"panel-token", "", "127.0.0.1:1000", 400}, {"panel-token", "", "192.0.2.1:1000", 401}} {
		req := httptest.NewRequest("POST", "http://127.0.0.1:15060/api/downloads/add", strings.NewReader(`{"url":"file:///forbidden"}`))
		req.RemoteAddr = test.peer
		req.Header.Set("X-App-Token", test.token)
		if test.origin != "" {
			req.Header.Set("Origin", test.origin)
		}
		w := httptest.NewRecorder()
		a.handler(false).ServeHTTP(w, req)
		if w.Code != test.code {
			t.Fatalf("guard status=%d want=%d", w.Code, test.code)
		}
	}
}

func TestDownloadReadableErrorsRemainPrivate(t *testing.T) {
	message := downloadError("1", "SSL/TLS handshake failure: https://user:PRIVATE-PASSWORD@example.invalid/PRIVATE-TOKEN")
	if !strings.Contains(message, "TLS 握手失败") || strings.Contains(message, "PRIVATE") || strings.Contains(message, "example.invalid") {
		t.Fatal(message)
	}
	if !strings.Contains(downloadError("9", "arbitrary secret"), "空间不足") {
		t.Fatal("disk error not described")
	}
}
