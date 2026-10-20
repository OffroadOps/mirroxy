//go:build windows

package main

import (
	"golang.org/x/sys/windows/registry"
	"testing"
)

func TestWindowsProxyModesUseOwnedBackup(t *testing.T) {
	// Exercise real registry APIs in a disposable test key, never Internet Settings.
	path := `Software\Mirroxy-tests\` + newID()
	k, _, e := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { k.Close(); registry.DeleteKey(registry.CURRENT_USER, path) }()
	before := proxySettingsPath
	proxySettingsPath = path
	defer func() { proxySettingsPath = before }()
	k.SetDWordValue("ProxyEnable", 1)
	k.SetStringValue("ProxyServer", "127.0.0.1:10808")
	k.SetStringValue("AutoConfigURL", "")
	a := &App{root: t.TempDir(), panelPort: 15060, cores: map[string]*Core{"xray-n": {done: make(chan struct{})}, "xray-other": {done: make(chan struct{})}}, cfg: Settings{Nodes: []Node{{ID: "n", Name: "test", Engine: "xray", Port: 5060, Enabled: true}, {ID: "other", Name: "other", Engine: "xray", Port: 5061, Enabled: true}}}}
	assertActive := func(id string, active bool) {
		t.Helper()
		for _, raw := range a.state()["nodes"].([]any) {
			n := raw.(M)
			if n["systemProxy"] != (n["id"] == id && active) {
				t.Fatalf("incorrect applied proxy flag on node %s", n["id"])
			}
		}
	}
	if e = a.applySystemProxy("unchanged", ""); e != nil {
		t.Fatal(e)
	}
	en, server := currentProxy()
	if en != 1 || server != "127.0.0.1:10808" {
		t.Fatal("unchanged overwrote the existing proxy")
	}
	assertActive("", false)
	if e = a.applySystemProxy("auto", "n"); e != nil {
		t.Fatal(e)
	}
	en, server = currentProxy()
	if en != 1 || server != "127.0.0.1:5060" {
		t.Fatal("automatic proxy not applied")
	}
	assertActive("n", true)
	a.cfg.SystemProxyNode = "other" // A saved selection alone does not prove OS state.
	assertActive("n", true)
	if e = a.applySystemProxy("auto", "other"); e != nil {
		t.Fatal(e)
	}
	assertActive("other", true)
	a.restoreProxy()
	en, server = currentProxy()
	if en != 1 || server != "127.0.0.1:10808" {
		t.Fatal("prior v2rayN proxy not restored")
	}
	assertActive("", false)
	if e = a.applySystemProxy("auto", "n"); e != nil {
		t.Fatal(e)
	}
	k.SetStringValue("ProxyServer", "127.0.0.1:10808") // Another client regains control.
	assertActive("", false)
	a.restoreProxy()
	_, server = currentProxy()
	if server != "127.0.0.1:10808" {
		t.Fatal("restore overwrote another client's change")
	}
	if e = a.applySystemProxy("clear", ""); e != nil {
		t.Fatal(e)
	}
	en, _ = currentProxy()
	if en != 0 {
		t.Fatal("clear did not disable the proxy")
	}
}
