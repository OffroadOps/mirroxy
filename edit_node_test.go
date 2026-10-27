package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func editTestApp(t *testing.T) *App {
	t.Helper()
	a := &App{root: t.TempDir(), panelPort: 15060, token: "ui-token", cores: map[string]*Core{}}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}
	a.cfg.SystemProxyMode = "unchanged"
	n, err := parseLink("vless://11111111-2222-3333-4444-555555555555@example.invalid:24443?security=reality&pbk=pub-key&sid=ab12&fp=chrome&flow=xtls-rprx-vision&type=tcp&sni=cdn.example.invalid#Provider")
	if err != nil {
		t.Fatal(err)
	}
	n.ID, n.Port, n.Route = "edit-test", 5066, "bypass-cn"
	a.cfg.Nodes = []Node{n}
	return a
}

func TestEditNodeUpdatesOutboundKeepsRuntime(t *testing.T) {
	a := editTestApp(t)
	stats := a.trafficFor("edit-test")
	stats.Uploaded = 999
	if err := a.editNode("edit-test", M{"name": "编辑后", "server": "new.example.invalid", "port": "34443", "password": "rotated-uuid", "publicKey": "new-pbk", "shortId": "ff99", "sni": "new-sni.example.invalid", "flow": "xtls-rprx-vision", "network": "raw", "security": "reality", "fingerprint": "chrome"}); err != nil {
		t.Fatal(err)
	}
	n := a.cfg.Nodes[0]
	if n.Name != "编辑后" || n.Server != "new.example.invalid" || n.Port != 5066 {
		t.Fatal("edit changed name/server/remote-port incorrectly:", n.Name, n.Server, n.Port)
	}
	if n.Enabled || stats.Uploaded != 999 {
		t.Fatal("edit disrupted runtime or traffic")
	}
	o, _ := json.Marshal(n.Outbound)
	out := string(o)
	for _, want := range []string{"new.example.invalid", "34443", "rotated-uuid", "new-pbk", "ff99", "new-sni.example.invalid", "reality"} {
		if !strings.Contains(out, want) {
			t.Fatalf("outbound missing %s: %s", want, out)
		}
	}
	if n.OriginalName != "Provider" {
		t.Fatal("edit should retain the provider name like rename does")
	}
	// Spec API returns the edited values.
	form, err := a.nodeEditableSpec("edit-test")
	if err != nil {
		t.Fatal(err)
	}
	if form["server"] != "new.example.invalid" || form["port"] != float64(34443) {
		t.Fatal("spec API did not reflect the edit:", form["server"], form["port"])
	}
}

func TestEditNodeValidationAndRollback(t *testing.T) {
	a := editTestApp(t)
	before, _ := json.Marshal(a.cfg.Nodes[0].Outbound)
	for _, tc := range []struct{ name string; form M }{
		{"empty name", M{"name": "  "}},
		{"bad port", M{"port": "70000"}},
		{"empty server", M{"server": " "}},
		{"missing password", M{"password": ""}},
		{"reality without pbk", M{"security": "reality", "publicKey": ""}},
	} {
		if err := a.editNode("edit-test", tc.form); err == nil {
			t.Fatalf("%s: edit was accepted", tc.name)
		}
	}
	after, _ := json.Marshal(a.cfg.Nodes[0].Outbound)
	if string(before) != string(after) {
		t.Fatal("rejected edit changed the outbound")
	}
	if err := a.editNode("missing", M{"name": "x"}); err == nil {
		t.Fatal("missing node accepted")
	}
}

func TestEditNodeSubscriptionSurvivesRefresh(t *testing.T) {
	a := editTestApp(t)
	const original = "vless://11111111-2222-3333-4444-555555555555@example.invalid:24443?security=reality&pbk=pub-key&sid=ab12&fp=chrome&flow=xtls-rprx-vision&type=tcp&sni=cdn.example.invalid#Provider"
	const providerChanged = "vless://11111111-2222-3333-4444-555555555555@example.invalid:24443?security=reality&pbk=DIFFERENT-KEY&sid=zz99&fp=chrome&flow=xtls-rprx-vision&type=tcp&sni=cdn.example.invalid#Provider"
	var body atomic.Value
	body.Store(original)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body.Load().(string)))
	}))
	defer srv.Close()
	a.cfg.Subscriptions = []Subscription{{ID: "sub", Name: "test", URL: srv.URL}}
	a.cfg.Nodes[0].Source = "sub"
	if err := a.editNode("edit-test", M{"name": "我的节点", "server": "edited.example.invalid", "port": "34443", "password": "edited-uuid", "publicKey": "edited-pbk", "security": "reality", "network": "raw", "fingerprint": "chrome", "shortId": "ab12", "sni": "cdn.example.invalid", "flow": "xtls-rprx-vision"}); err != nil {
		t.Fatal(err)
	}
	// Provider rotates the REALITY key; the user's edit must win.
	body.Store(providerChanged)
	if count, err := a.refreshSubscriptionStored("sub"); err != nil || count != 1 {
		t.Fatal("refresh failed", count, err)
	}
	if len(a.cfg.Nodes) != 1 {
		t.Fatal("edit created a duplicate node on refresh")
	}
	n := a.cfg.Nodes[0]
	if n.ID != "edit-test" || n.Name != "我的节点" || n.Port != 5066 || n.Source != "sub" {
		t.Fatal("refresh lost identity, name, port or source")
	}
	o, _ := json.Marshal(n.Outbound)
	out := string(o)
	if !strings.Contains(out, "edited.example.invalid") || !strings.Contains(out, "edited-pbk") {
		t.Fatalf("provider values overwrote the edit: %s", out)
	}
}

// HTTP and SOCKS nodes authenticate with a username and a password. The editor
// must expose both, and saving a new password must reach the outbound.
func TestEditNodeUpdatesHTTPAndSocksCredentials(t *testing.T) {
	for _, tc := range []struct{ name, link string }{
		{"http", "http://alice:secret@proxy.example.invalid:8080#Provider"},
		{"socks", "socks://alice:secret@proxy.example.invalid:1080#Provider"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &App{root: t.TempDir(), panelPort: 15060, token: "ui-token", cores: map[string]*Core{}}
			if err := a.load(); err != nil {
				t.Fatal(err)
			}
			a.cfg.SystemProxyMode = "unchanged"
			n, err := parseLink(tc.link)
			if err != nil {
				t.Fatal(err)
			}
			n.ID, n.Port, n.Route = "edit-test", 5066, "proxy"
			a.cfg.Nodes = []Node{n}

			// The stored form must carry both credential fields.
			form, err := a.nodeEditableSpec("edit-test")
			if err != nil {
				t.Fatal(err)
			}
			if form["username"] != "alice" || form["password"] != "secret" {
				t.Fatalf("credentials missing from the edit form: %v", form)
			}

			// Rotating the password must reach the outbound.
			if err = a.editNode("edit-test", M{"username": "bob", "password": "rotated"}); err != nil {
				t.Fatal(err)
			}
			o, _ := json.Marshal(a.cfg.Nodes[0].Outbound)
			out := string(o)
			if !strings.Contains(out, "bob") || !strings.Contains(out, "rotated") {
				t.Fatalf("the new credentials did not reach the outbound: %s", out)
			}
			if strings.Contains(out, "secret") {
				t.Fatalf("the old password survived the edit: %s", out)
			}
		})
	}
}

// A SOCKS password without a username would be dropped silently, so the edit
// must be refused instead of losing the credential.
func TestEditNodeRejectsPasswordWithoutUsername(t *testing.T) {
	a := &App{root: t.TempDir(), panelPort: 15060, token: "ui-token", cores: map[string]*Core{}}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}
	a.cfg.SystemProxyMode = "unchanged"
	n, err := parseLink("socks://alice:secret@proxy.example.invalid:1080#Provider")
	if err != nil {
		t.Fatal(err)
	}
	n.ID, n.Port, n.Route = "edit-test", 5066, "proxy"
	a.cfg.Nodes = []Node{n}
	if err = a.editNode("edit-test", M{"username": "", "password": "rotated"}); err == nil {
		t.Fatal("a password without a username was accepted")
	}
}

// A protocol that carries no password at all must still refuse an empty one.
func TestEditNodeKeepsPasswordRequirementForOtherProtocols(t *testing.T) {
	a := editTestApp(t)
	if err := a.editNode("edit-test", M{"password": ""}); err == nil {
		t.Fatal("an empty password was accepted for a VLESS node")
	}
}

func TestEditNodeAPI(t *testing.T) {
	a := editTestApp(t)
	for _, tc := range []struct {
		path, body string
		want       int
	}{
		{"/api/node/spec", `{"id":"edit-test"}`, 200},
		{"/api/node/spec", `{"id":"missing"}`, 400},
		{"/api/node/edit", `{"id":"edit-test","form":{"name":"API 名称","server":"api.example.invalid","port":34443,"password":"api-uuid","security":"reality","network":"raw","fingerprint":"chrome","publicKey":"pub-key","shortId":"ab12","sni":"cdn.example.invalid","flow":"xtls-rprx-vision"}}`, 200},
		{"/api/node/edit", `{"id":"missing","form":{"name":"x"}}`, 400},
		{"/api/node/edit", `{"id":"edit-test","form":{"port":70000}}`, 400},
	} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:15060"+tc.path, strings.NewReader(tc.body))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-App-Token", a.token)
		w := httptest.NewRecorder()
		a.handler(false).ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s %s: got %d, want %d (body %s)", tc.path, tc.body, w.Code, tc.want, w.Body.String())
		}
	}
	if a.cfg.Nodes[0].Name != "API 名称" || a.cfg.Nodes[0].Server != "api.example.invalid" {
		t.Fatal("edit API did not apply changes")
	}
}
