package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func appRouterTestApp(t *testing.T) *App {
	t.Helper()
	return &App{
		root: t.TempDir(), panelPort: 15060, cores: map[string]*Core{}, metricsPorts: map[string]int{},
		cfg: Settings{
			Nodes: []Node{
				{ID: "x", Name: "X", Engine: "xray", Port: 5060, AppPort: 25060, Enabled: true, Route: "bypass-cn", Outbound: M{"protocol": "freedom"}},
				{ID: "s", Name: "S", Engine: "sing-box", Port: 5061, AppPort: 25061, Enabled: true, Route: "bypass-cn", Outbound: M{"type": "direct"}},
			},
			RoutingMode: "bypass-cn",
			LogLevel:    "warning",
			AppRouting: AppRoutingSettings{
				DefaultNode: "x", StrictRoute: true, Secret: "test-secret-value",
				Profiles: []AppRouteProfile{{ID: "game", Name: "Game", Executables: []string{"game.exe"}, Enabled: true}},
			},
		},
	}
}

func TestSavingApplicationProfilesDoesNotChangeExistingNodeBehavior(t *testing.T) {
	if !platformApplicationRoutingSupported() {
		t.Skip("application routing is Windows-only")
	}
	a := appRouterTestApp(t)
	before := cloneSettings(a.cfg)
	err := a.saveAppRouting(AppRoutingSettings{
		DefaultNode: "s",
		Profiles:    []AppRouteProfile{{ID: "browser", Name: "Browser", Executables: []string{"browser.exe"}, Enabled: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.cfg.Nodes, before.Nodes) || a.cfg.RoutingMode != before.RoutingMode || a.cfg.DefaultRoute != before.DefaultRoute {
		t.Fatalf("saving app profiles changed existing node or route settings: before=%+v after=%+v", before, a.cfg)
	}
}

func TestApplicationRouterConfigIsIsolatedAndFailClosedAtNodePort(t *testing.T) {
	a := appRouterTestApp(t)
	cfg, err := a.applicationRouterConfig()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(cfg)
	text := string(b)
	for _, want := range []string{`"type":"tun"`, `"strict_route":true`, `"fdfe:dcba:9876::1/126"`, `"process_name":["game.exe"]`, `"server_port":25060`, `"username":"mirroxy-app"`, `"final":"direct"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("application router config missing %s: %s", want, text)
		}
	}
	if strings.Contains(text, `"server_port":5060`) {
		t.Fatal("TUN uses the public node port, so the node's bypass route could leak selected traffic")
	}
}

func TestApplicationIngressUsesAuthenticatedLoopback(t *testing.T) {
	a := appRouterTestApp(t)
	for _, key := range []string{"xray", "sing-box-s"} {
		cfg, ports := a.coreConfig(key)
		b, _ := json.Marshal(cfg)
		text := string(b)
		if !strings.Contains(text, `app-in-`) || !strings.Contains(text, `test-secret-value`) {
			t.Fatalf("%s missing authenticated application ingress: %s", key, text)
		}
		wantPort := 25060
		if key != "xray" {
			wantPort = 25061
		}
		found := false
		for _, port := range ports {
			found = found || port == wantPort
		}
		if !found {
			t.Fatalf("%s does not wait for application port %d", key, wantPort)
		}
	}
}

func TestApplicationProfileValidationRejectsAmbiguousAndSelfRouting(t *testing.T) {
	a := appRouterTestApp(t)
	_, err := a.normalizeAppRouting(AppRoutingSettings{DefaultNode: "x", Profiles: []AppRouteProfile{
		{Name: "one", Executables: []string{"same.exe"}, Enabled: true},
		{Name: "two", Executables: []string{"same.exe"}, Enabled: true},
	}})
	if err == nil {
		t.Fatal("duplicate executable was accepted")
	}
	_, err = a.normalizeAppRouting(AppRoutingSettings{DefaultNode: "x", Profiles: []AppRouteProfile{{Name: "loop", Executables: []string{"Mirroxy_core_B.exe"}, Enabled: true}}})
	if err == nil {
		t.Fatal("routing the proxy core itself was accepted")
	}
}

func TestApplicationConfigsValidateWithInstalledCores(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows core binaries")
	}
	a := appRouterTestApp(t)
	// Route assets are not part of this isolated temporary root; proxy mode still
	// validates both authenticated app inbounds without external geodata. LAN
	// bypass would reference the geodata files, so it is disabled for this check.
	disabled := false
	a.cfg.BypassLAN = &disabled
	for i := range a.cfg.Nodes {
		a.cfg.Nodes[i].Route = "proxy"
	}
	root := filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Mirroxy")
	for _, tc := range []struct{ key, engine, name string }{
		{"xray", "xray", "Mirroxy_core_A.exe"}, {"sing-box-s", "sing-box", "Mirroxy_core_B.exe"},
	} {
		source := filepath.Join(root, "bin", "runtime", tc.name)
		if _, err := os.Stat(source); err != nil {
			t.Skip("installed core not found: " + source)
		}
		cfg, _ := a.coreConfig(tc.key)
		path := filepath.Join(a.root, tc.engine+"-app-ingress.json")
		if err := writeJSON(path, cfg); err != nil {
			t.Fatal(err)
		}
		if err := a.validateCore(tc.engine, path, source); err != nil {
			t.Fatalf("%s rejected application ingress: %v", tc.engine, err)
		}
	}
	cfg, err := a.applicationRouterConfig()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.root, "tun.json")
	if err = writeJSON(path, cfg); err != nil {
		t.Fatal(err)
	}
	sing := filepath.Join(root, "bin", "runtime", "Mirroxy_core_B.exe")
	if err = a.validateCore("sing-box", path, sing); err != nil {
		t.Fatalf("sing-box rejected TUN application rules: %v", err)
	}
}
