package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func freeTestPort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}

func TestCoreAssetsMatchPlatformAndArchive(t *testing.T) {
	for _, tc := range []struct{ engine, os, arch, want string }{
		{"xray", "linux", "arm64", "Xray-linux-arm64-v8a.zip"}, {"xray", "linux", "amd64", "Xray-linux-64.zip"}, {"xray", "windows", "amd64", "Xray-windows-64.zip"},
		{"sing-box", "linux", "arm64", "sing-box-1.14.0-linux-arm64-musl.tar.gz"}, {"sing-box", "windows", "amd64", "sing-box-1.14.0-windows-amd64.zip"},
		{"xray", "darwin", "arm64", "Xray-macos-arm64-v8a.zip"}, {"sing-box", "darwin", "arm64", "sing-box-1.14.0-darwin-arm64.tar.gz"},
	} {
		got, e := coreAssetName(tc.engine, "v1.14.0", tc.os, tc.arch)
		if e != nil || got != tc.want {
			t.Fatalf("%+v: %s %v", tc, got, e)
		}
	}
	if _, e := coreAssetName("xray", "v26", "linux", "386"); e == nil {
		t.Fatal("unsupported CPU accepted")
	}
	var zipped bytes.Buffer
	zw := zip.NewWriter(&zipped)
	f, _ := zw.Create("release/xray")
	f.Write([]byte("executable"))
	zw.Close()
	b, e := extractCorePayload(zipped.Bytes(), "xray")
	if e != nil || string(b) != "executable" {
		t.Fatal(e)
	}
	var packed bytes.Buffer
	gz := gzip.NewWriter(&packed)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "release/sing-box", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink, Mode: 0777})
	tw.Close()
	gz.Close()
	if _, e = extractCorePayload(packed.Bytes(), "sing-box"); e == nil {
		t.Fatal("symlink executable accepted")
	}
	packed.Reset()
	gz = gzip.NewWriter(&packed)
	tw = tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "release/sing-box", Typeflag: tar.TypeReg, Mode: 0755, Size: 3})
	tw.Write([]byte("ELF"))
	tw.Close()
	gz.Close()
	b, e = extractCorePayload(packed.Bytes(), "sing-box")
	if e != nil || string(b) != "ELF" {
		t.Fatal(e)
	}
}

func TestServerInitializationAndPasswordReset(t *testing.T) {
	a := &App{root: t.TempDir(), panelPort: freeTestPort(t)}
	if e := a.initializeServer("192.0.2.10", 15061, false); e != nil {
		t.Fatal(e)
	}
	originalHash := a.cfg.Remote.PasswordHash
	var info string
	if err := a.readState("access", &info); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(info, "http://192.0.2.10:15061/") || !a.cfg.AutoDeploy || !a.cfg.AutoDownload || !a.cfg.Remote.Enabled || a.cfg.Remote.Scheme != "http" {
		t.Fatal("server defaults incomplete")
	}
	if e := a.initializeServer("192.0.2.10", 15061, false); e != nil {
		t.Fatal(e)
	}
	if a.cfg.Remote.PasswordHash != originalHash {
		t.Fatal("reinstallation rotated credentials")
	}
	oldHash := a.cfg.Remote.PasswordHash
	a.cfg.Remote.Port = 18080
	a.cfg.Remote.Scheme = "https"
	a.cfg.AutoDeploy = false
	a.save()
	if e := a.initializeServer("192.0.2.10", 15061, true); e != nil {
		t.Fatal(e)
	}
	if a.cfg.Remote.PasswordHash == oldHash || a.cfg.Remote.Port != 18080 || a.cfg.Remote.Scheme != "https" || a.cfg.AutoDeploy {
		t.Fatal("password reset changed other settings")
	}
}

func TestHTTPRemoteRequiresAuthenticationEvenOnLoopback(t *testing.T) {
	a := &App{root: t.TempDir(), panelPort: freeTestPort(t), token: "ui-token", cores: map[string]*Core{}}
	if e := a.load(); e != nil {
		t.Fatal(e)
	}
	a.cfg.Remote = RemoteSettings{Enabled: true, Scheme: "http", Port: freeTestPort(t), Username: "admin", Salt: "test", PasswordHash: passwordHash("test-long-password", "test")}
	s, l, e := a.prepareRemote(a.cfg.Remote)
	if e != nil {
		t.Fatal(e)
	}
	a.startRemote(s, l)
	defer s.Close()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	address := fmt.Sprintf("http://127.0.0.1:%d/api/state", a.cfg.Remote.Port)
	r, e := client.Get(address)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 401 {
		t.Fatal("HTTP listener bypassed auth")
	}
	req, _ := http.NewRequest("GET", address, nil)
	req.SetBasicAuth("admin", "test-long-password")
	r, e = client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	var state struct {
		LocalAccess, SettingsEditable bool
		Remote                        struct{ Scheme string }
	}
	json.NewDecoder(r.Body).Decode(&state)
	r.Body.Close()
	if r.StatusCode != 200 || state.LocalAccess || state.SettingsEditable != serverPlatform() || state.Remote.Scheme != "http" {
		t.Fatalf("incorrect HTTP admin state %+v", state)
	}
}

func TestImportSavesNodesWhenAutomaticInstallUnavailable(t *testing.T) {
	a := &App{root: t.TempDir(), cores: map[string]*Core{}}
	if e := a.load(); e != nil {
		t.Fatal(e)
	}
	a.cfg.AutoDownload = false
	a.cfg.AutoDeploy = true
	result, e := a.importAndDeploy("http://127.0.0.1:1#missing-core", "", nil)
	if e != nil {
		t.Fatal(e)
	}
	if result["count"] != 1 || result["deployed"] != 0 || len(result["warnings"].([]string)) == 0 || len(a.cfg.Nodes) != 1 || a.cfg.Nodes[0].Enabled {
		t.Fatal("failed automatic deployment lost imported node or reported success")
	}
}

func TestImportAutomaticallyDeploysThroughBundledCore(t *testing.T) {
	core := os.Getenv("MIRROXY_TEST_XRAY")
	if core == "" {
		t.Skip("set installed core path")
	}
	a := &App{root: t.TempDir(), cores: map[string]*Core{}}
	if e := a.load(); e != nil {
		t.Fatal(e)
	}
	a.cfg.AutoDeploy = true
	a.cfg.AutoDownload = false
	// This fixture intentionally bundles only the executable; split routing
	// with bundled geodata is exercised by TestRoutingWithInstalledCores.
	a.cfg.DefaultRoute = "proxy"
	os.MkdirAll(filepath.Dir(a.corePath("xray")), 0700)
	b, e := os.ReadFile(core)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(a.corePath("xray"), b, 0755); e != nil {
		t.Fatal(e)
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			w.WriteHeader(405)
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		rw.WriteString("HTTP/1.1 200 Connection established\r\n\r\n")
		rw.Flush()
		if _, err = http.ReadRequest(rw.Reader); err != nil {
			return
		}
		rw.WriteString("HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n")
		rw.Flush()
	}))
	defer proxy.Close()
	result, e := a.importAndDeploy(proxy.URL+"#auto-deploy", "", nil)
	defer a.stopEngine("xray")
	if e != nil || result["deployed"] != 1 {
		t.Fatalf("automatic deployment failed: %v %v", result, e)
	}
	n := a.cfg.Nodes[0]
	if !n.Enabled || n.Port < 5060 || !a.cores[n.coreKey()].alive() {
		t.Fatal("automatic local port not running")
	}
	if _, e = checkProxyHealth(t.Context(), n.Port, []string{"http://test.invalid/generate_204"}); e != nil {
		t.Fatal(e)
	}
}
