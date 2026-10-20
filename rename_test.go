package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func renameTestApp(t *testing.T) *App {
	t.Helper()
	a := &App{root: t.TempDir(), panelPort: 15060, token: "ui-token", cores: map[string]*Core{}}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}
	a.cfg.SystemProxyMode = "unchanged"
	n, err := parseLink("http://user:password@example.invalid:24443#Provider")
	if err != nil {
		t.Fatal(err)
	}
	n.ID, n.Port, n.Route = "rename-test", 5066, "bypass-cn"
	a.cfg.Nodes = []Node{n}
	return a
}

func TestRenamePreservesRuntimeAndPersists(t *testing.T) {
	a := renameTestApp(t)
	a.cfg.Nodes[0].Enabled = true
	core := &Core{}
	a.cores[a.cfg.Nodes[0].coreKey()] = core
	stats := a.trafficFor("rename-test")
	stats.Uploaded, stats.Downloaded = 12345, 67890
	before, _ := json.Marshal(a.cfg.Nodes[0].Outbound)
	if err := a.renameNode("rename-test", "  下载 🖊 <专用>  "); err != nil {
		t.Fatal(err)
	}
	n := a.cfg.Nodes[0]
	if a.cores[a.cfg.Nodes[0].coreKey()] != core || !n.Enabled || n.Port != 5066 || stats.Downloaded != 67890 || stats.Uploaded != 12345 {
		t.Fatal("rename disrupted runtime or traffic")
	}
	after, _ := json.Marshal(n.Outbound)
	if string(before) != string(after) || n.Name != "下载 🖊 <专用>" || n.OriginalName != "Provider" {
		t.Fatal("rename changed connection settings or lost original name")
	}
	reloaded := &App{root: a.root}
	if err := reloaded.load(); err != nil {
		t.Fatal("custom name did not persist", err)
	}
	savedNode, _ := json.Marshal(n)
	loadedNode, _ := json.Marshal(reloaded.cfg.Nodes[0])
	if string(savedNode) != string(loadedNode) {
		t.Fatal("custom name or node settings changed on reload")
	}
	uri, err := shareLink(n)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := parseLink(uri)
	if err != nil || exported.Name != n.Name {
		t.Fatal("share did not use custom name", err)
	}
	if err := a.renameNode(n.ID, "Provider"); err != nil || a.cfg.Nodes[0].OriginalName != "" {
		t.Fatal("restoring provider name failed", err)
	}
}

func TestRenameSubscriptionKeepsAliasWhenProviderChanges(t *testing.T) {
	a := renameTestApp(t)
	var body atomic.Value
	body.Store("http://user:password@example.invalid:24443#Provider")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body.Load().(string)))
	}))
	defer srv.Close()
	a.cfg.Subscriptions = []Subscription{{ID: "sub", Name: "test", URL: srv.URL}}
	a.cfg.Nodes[0].Source = "sub"
	a.cfg.Nodes[0].ListBottom = true
	if err := a.renameNode("rename-test", "下载专用"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ link, provider string }{
		{"http://user:password@example.invalid:24443#Provider", "Provider"},
		{"http://user:password@example.invalid:24443#RenamedProvider", "RenamedProvider"},
		{"http://user:rotated-password@example.invalid:24443#RenamedProvider", "RenamedProvider"},
	} {
		body.Store(tc.link)
		if count, err := a.refreshSubscriptionStored("sub"); err != nil || count != 1 {
			t.Fatal("refresh failed", count, err)
		}
		if len(a.cfg.Nodes) != 1 {
			t.Fatal("custom name created a duplicate on refresh")
		}
		n := a.cfg.Nodes[0]
		if n.ID != "rename-test" || n.Name != "下载专用" || n.OriginalName != tc.provider || n.Port != 5066 || n.Route != "bypass-cn" || !n.ListBottom {
			t.Fatal("refresh lost custom name or stable node settings")
		}
		fresh, _ := parseLink(tc.link)
		if fingerprint(n) != fingerprint(fresh) {
			t.Fatal("subscription credentials were not refreshed")
		}
	}
}

func TestRenameValidationAndFailedSaveRollback(t *testing.T) {
	a := renameTestApp(t)
	before := a.cfg.Nodes[0]
	for _, name := range []string{"", "  ", "a\nb", "a\x00b", strings.Repeat("名", 129)} {
		if err := a.renameNode("rename-test", name); err == nil {
			t.Fatal("accepted invalid name")
		}
	}
	if err := a.renameNode("missing", "valid"); err == nil {
		t.Fatal("accepted missing node")
	}
	// Force the encrypted database open to fail, independently of OS permissions.
	a.closeStore()
	dbPath := filepath.Join(a.root, "data", "Mirroxy.db")
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dbPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := a.renameNode("rename-test", "New name"); err == nil {
		t.Fatal("save should fail")
	}
	if !reflect.DeepEqual(before, a.cfg.Nodes[0]) {
		t.Fatal("failed rename changed the in-memory name")
	}
}

func TestRenameAPIAuthorization(t *testing.T) {
	a := renameTestApp(t)
	a.cfg.Remote = RemoteSettings{Username: "admin", Salt: "test", PasswordHash: passwordHash("test-password", "test")}
	for _, tc := range []struct {
		path, body, origin string
		remote, auth       bool
		want               int
	}{
		{"/api/node/rename", `{"id":"rename-test","name":"Local alias"}`, "", false, false, 200},
		{"/api/v1/nodes/rename-test/rename", `{"name":"AI alias"}`, "", false, false, 200},
		{"/api/v1/nodes/rename-test/rename", `{"name":"Remote alias"}`, "", true, false, 401},
		{"/api/v1/nodes/rename-test/rename", `{"name":"Remote alias"}`, "", true, true, 200},
		{"/api/v1/nodes/rename-test/rename", `{"name":"Attack"}`, "https://evil.invalid", false, false, 403},
		{"/api/v1/nodes/rename-test/rename", `{}`, "", false, false, 400},
		{"/api/v1/nodes/missing/rename", `{"name":"Missing"}`, "", false, false, 400},
	} {
		before := a.cfg.Nodes[0].Name
		r := httptest.NewRequest("POST", "http://127.0.0.1:15060"+tc.path, strings.NewReader(tc.body))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-App-Token", a.token)
		r.Header.Set("Origin", tc.origin)
		if tc.auth {
			r.SetBasicAuth("admin", "test-password")
		}
		w := httptest.NewRecorder()
		a.handler(tc.remote).ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.path, w.Code, tc.want)
		}
		if tc.want != 200 && before != a.cfg.Nodes[0].Name {
			t.Fatal("rejected request changed name")
		}
	}
}
