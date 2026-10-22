package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestScanDirectoryExecutablesRanksMainProgramsFirst(t *testing.T) {
	root := t.TempDir()
	write := func(relative string, size int) {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, size), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write("Game.exe", 3000)
	write("Uninstall Game.exe", 500)
	write(filepath.Join("helper", "setup.exe"), 100)
	write(filepath.Join("sub", "deep", "tool.exe"), 200)
	write(filepath.Join("sub", "a", "b", "too-deep.exe"), 100)
	write(filepath.Join("sub", "a", "b", "c", "buried.exe"), 100)
	write("notes.txt", 10)

	found, err := scanDirectoryExecutables(root)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]scannedExecutable{}
	for _, item := range found {
		paths[item.Path] = item
	}
	game := paths[filepath.Join(root, "Game.exe")]
	if !game.Main || game.Size != 3000 {
		t.Fatalf("top-level program not flagged as main: %+v", game)
	}
	if junk, ok := paths[filepath.Join(root, "Uninstall Game.exe")]; !ok || junk.Main {
		t.Fatalf("uninstaller must not be flagged as main: %+v", junk)
	}
	if nested, ok := paths[filepath.Join(root, "sub", "deep", "tool.exe")]; !ok || nested.Main {
		t.Fatalf("nested exe should be found but not main: %+v", nested)
	}
	if deep, ok := paths[filepath.Join(root, "sub", "a", "b", "too-deep.exe")]; !ok || deep.Main {
		t.Fatalf("exe inside three folder levels should be found but not main: %+v", deep)
	}
	if buried, ok := paths[filepath.Join(root, "sub", "a", "b", "c", "buried.exe")]; ok {
		t.Fatalf("exe beyond the depth limit was returned: %+v", buried)
	}
	if len(found) != 5 {
		t.Fatalf("unexpected scan result: %+v", found)
	}
	if found[0].Path != game.Path {
		t.Fatalf("main program must sort first: %+v", found)
	}
	if _, err := scanDirectoryExecutables(filepath.Join(root, "notes.txt")); err == nil {
		t.Fatal("file path was accepted as a scan root")
	}
	if _, err := scanDirectoryExecutables("relative\\path"); err == nil {
		t.Fatal("relative path was accepted as a scan root")
	}
}

func TestDetectAcceleratorConflicts(t *testing.T) {
	hits := detectAcceleratorConflicts([]applicationProcess{
		{PID: 1, Name: "uu.exe", Path: `C:\UU\uu.exe`},
		{PID: 2, Name: "Leiguan.exe", Path: `C:\Leiguan\Leiguan.exe`},
		{PID: 3, Name: "chrome.exe"},
		{PID: 4, Name: "uu_service.exe"},
	})
	products := map[string]bool{}
	for _, hit := range hits {
		products[hit["product"].(string)] = true
	}
	if !products["网易 UU 加速器"] || !products["雷神加速器"] {
		t.Fatalf("expected UU and Leiguan detection: %+v", hits)
	}
	if len(hits) != 2 {
		t.Fatalf("unexpected conflict hits: %+v", hits)
	}
}

func TestApplicationRouterConfigCompatModeRelaxesStrictRoute(t *testing.T) {
	a := appRouterTestApp(t)
	normalized, err := a.normalizeAppRouting(AppRoutingSettings{DefaultNode: "x", CompatMode: true,
		Profiles: []AppRouteProfile{{Name: "g", Executables: []string{"g.exe"}, Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if !normalized.CompatMode {
		t.Fatal("compat mode was dropped during normalization")
	}
	a.cfg.AppRouting = normalized
	cfg, err := a.applicationRouterConfig()
	if err != nil {
		t.Fatal(err)
	}
	tun := cfg["inbounds"].([]any)[0].(M)
	if tun["strict_route"] != false {
		t.Fatalf("compat mode must disable strict_route: %+v", tun)
	}
	a.cfg.AppRouting.CompatMode = false
	cfg, _ = a.applicationRouterConfig()
	tun = cfg["inbounds"].([]any)[0].(M)
	if tun["strict_route"] != true {
		t.Fatalf("default must keep strict_route: %+v", tun)
	}
	if !a.cfg.AppRouting.StrictRoute {
		t.Fatal("the stored StrictRoute flag must stay true")
	}
}

func TestBrowseDirectoryListsVisibleSubdirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Games", "Tools", ".hidden", "$RECYCLE.BIN"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	result, err := browseDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	directories := map[string]bool{}
	for _, value := range result["directories"].([]string) {
		directories[value] = true
	}
	if !directories["Games"] || !directories["Tools"] {
		t.Fatalf("visible subdirectories missing: %+v", result["directories"])
	}
	if directories[".hidden"] || directories["$RECYCLE.BIN"] {
		t.Fatalf("hidden or system directories leaked: %+v", result["directories"])
	}
	if result["parent"] == "" {
		t.Fatal("parent directory missing")
	}
	if runtime.GOOS == "windows" && len(result["drives"].([]string)) == 0 {
		t.Fatal("windows browse result missing drive list")
	}
	file := filepath.Join(root, "readme.txt")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if result, err = browseDirectory(file); err != nil {
		t.Fatal(err)
	}
	if result["path"] != root {
		t.Fatalf("file path was not resolved to its folder: %+v", result["path"])
	}
	if _, err := browseDirectory(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing folder was accepted")
	}
}

func TestApplicationIngressFollowsNodeExitPolicy(t *testing.T) {
	a := appRouterTestApp(t)
	// sing-box: bypass-cn node means app ingress keeps mainland traffic direct.
	cfg, _ := a.coreConfig("sing-box-s")
	rules := cfg["route"].(M)["rules"].([]any)
	appRules := []M{}
	for _, raw := range rules {
		rule := raw.(M)
		if inbound, ok := rule["inbound"].([]string); ok && len(inbound) == 1 && inbound[0] == "app-in-s" {
			appRules = append(appRules, rule)
		}
	}
	directSets, routedToNode := false, false
	for _, rule := range appRules {
		sets, _ := rule["rule_set"].([]string)
		for _, name := range sets {
			if name == "geosite-cn" || name == "geoip-cn" {
				directSets = rule["outbound"] == "direct" || directSets
			}
		}
		if rule["outbound"] == "node-s" && rule["rule_set"] == nil && rule["ip_is_private"] == nil {
			routedToNode = true
		}
	}
	if !directSets || !routedToNode {
		b, _ := json.Marshal(appRules)
		t.Fatalf("app ingress does not follow bypass-cn policy: %s", b)
	}
	sets := map[string]bool{}
	for _, raw := range cfg["route"].(M)["rule_set"].([]any) {
		sets[raw.(M)["tag"].(string)] = true
	}
	if !sets["geosite-cn"] || !sets["geoip-cn"] {
		t.Fatalf("bypass rule sets missing from app ingress config: %+v", sets)
	}

	// xray: the last app-ingress rule still sends unmatched traffic to the bound node.
	cfg, _ = a.coreConfig("xray")
	rules = cfg["routing"].(M)["rules"].([]any)
	var last M
	for _, raw := range rules {
		rule := raw.(M)
		if tags, ok := rule["inboundTag"].([]string); ok && len(tags) == 1 && tags[0] == "app-in-x" {
			last = rule
		}
	}
	if last["outboundTag"] != "node-x" {
		b, _ := json.Marshal(rules)
		t.Fatalf("app ingress catch-all must stay on the bound node: %s", b)
	}

	// proxy nodes keep the historical forced behavior: with LAN bypass off, a
	// single catch-all sends everything to the node.
	disabled := false
	a.cfg.BypassLAN = &disabled
	for i := range a.cfg.Nodes {
		a.cfg.Nodes[i].Route = "proxy"
	}
	cfg, _ = a.coreConfig("sing-box-s")
	rules = cfg["route"].(M)["rules"].([]any)
	count, forced := 0, false
	for _, raw := range rules {
		rule := raw.(M)
		if inbound, ok := rule["inbound"].([]string); ok && len(inbound) == 1 && inbound[0] == "app-in-s" {
			count++
			if rule["outbound"] == "node-s" && rule["rule_set"] == nil {
				forced = true
			}
		}
	}
	if count != 1 || !forced {
		b, _ := json.Marshal(rules)
		t.Fatalf("proxy-mode app ingress must be a single forced rule: %s", b)
	}

	// With the default LAN bypass on, proxy-mode app ingress keeps LAN direct
	// before the forced node catch-all.
	a.cfg.BypassLAN = nil
	cfg, _ = a.coreConfig("sing-box-s")
	rules = cfg["route"].(M)["rules"].([]any)
	count, forced, lanDirect := 0, false, false
	for _, raw := range rules {
		rule := raw.(M)
		if inbound, ok := rule["inbound"].([]string); ok && len(inbound) == 1 && inbound[0] == "app-in-s" {
			count++
			if rule["outbound"] == "node-s" && rule["rule_set"] == nil {
				forced = true
			}
			if names, ok := rule["rule_set"].([]string); ok && len(names) == 1 && names[0] == "geosite-private" && rule["outbound"] == "direct" {
				lanDirect = true
			}
		}
	}
	if count != 4 || !forced || !lanDirect {
		b, _ := json.Marshal(rules)
		t.Fatalf("proxy-mode app ingress must bypass LAN before the node catch-all: %s", b)
	}
}
