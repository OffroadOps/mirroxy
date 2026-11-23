package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func configTestApp(t *testing.T) *App {
	t.Helper()
	a := &App{root: t.TempDir(), panelPort: 19950, token: "ui-token", cores: map[string]*Core{}}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}
	a.cfg.RoutingMode = "bypass-cn"
	xray, err := parseLink("http://user:password@xray.example.invalid:24443#Xray")
	if err != nil {
		t.Fatal(err)
	}
	xray.ID, xray.Port, xray.Enabled, xray.Route = "x", 5060, true, "inherit"
	sing, err := parseLink("hysteria2://password@sing.example.invalid:443?sni=sing.example.invalid#Sing")
	if err != nil {
		t.Fatal(err)
	}
	sing.ID, sing.Port, sing.Enabled, sing.Route = "b", 5061, true, "inherit"
	a.cfg.Nodes = []Node{xray, sing}
	return a
}

func hasRuleSet(route M, tag string) bool {
	sets, _ := route["rule_set"].([]any)
	for _, item := range sets {
		if set, ok := item.(M); ok && set["tag"] == tag {
			return true
		}
	}
	return false
}

func TestXraySniffingAndDNSConfig(t *testing.T) {
	a := configTestApp(t)
	cfg, _ := a.coreConfig("xray-x")
	inbounds, _ := cfg["inbounds"].([]any)
	if len(inbounds) == 0 {
		t.Fatal("no xray inbounds")
	}
	in, _ := inbounds[0].(M)
	sniff, _ := in["sniffing"].(M)
	if sniff["enabled"] != true || sniff["routeOnly"] != true {
		t.Fatalf("xray sniffing missing: %+v", in["sniffing"])
	}
	dns, _ := cfg["dns"].(M)
	if dns["tag"] != "dns-in" {
		t.Fatalf("xray dns tag = %v", dns["tag"])
	}
	servers, _ := dns["servers"].([]any)
	if len(servers) != 3 {
		t.Fatalf("xray dns servers = %d", len(servers))
	}
	domestic, _ := servers[0].(M)
	if !strings.Contains(stringVal(domestic["address"]), "223.5.5.5") || domestic["skipFallback"] != true {
		t.Fatalf("xray domestic DoH not first: %+v", domestic)
	}
	if got, _ := domestic["expectedIPs"].([]string); len(got) == 0 {
		t.Fatal("xray domestic DNS missing expectedIPs")
	}
	routing, _ := cfg["routing"].(M)
	rules, _ := routing["rules"].([]any)
	dnsDirect, dnsProxy := false, false
	for _, item := range rules {
		rule, _ := item.(M)
		tags, _ := rule["inboundTag"].([]string)
		if len(tags) != 1 || tags[0] != "dns-in" {
			continue
		}
		switch rule["outboundTag"] {
		case "direct":
			dnsDirect = true
		case "node-x":
			dnsProxy = true
		}
	}
	if !dnsDirect || !dnsProxy {
		t.Fatalf("xray dns-in routing missing: direct=%v proxy=%v", dnsDirect, dnsProxy)
	}
}

func TestSingBoxSniffingAndDNSConfig(t *testing.T) {
	a := configTestApp(t)
	cfg, _ := a.coreConfig("sing-box-b")
	route, _ := cfg["route"].(M)
	rules, _ := route["rules"].([]any)
	if len(rules) == 0 {
		t.Fatal("sing-box route has no rules")
	}
	first, _ := rules[0].(M)
	if first["action"] != "sniff" {
		t.Fatalf("sing-box first rule is not sniff: %+v", first)
	}
	if route["default_domain_resolver"] != "system" {
		t.Fatalf("sing-box bootstrap resolver = %v", route["default_domain_resolver"])
	}
	if !hasRuleSet(route, "geosite-cn") || !hasRuleSet(route, "geosite-private") {
		t.Fatalf("sing-box DNS rule sets missing from route: %+v", route["rule_set"])
	}
	dns, _ := cfg["dns"].(M)
	if dns["final"] != "remote" {
		t.Fatalf("sing-box DNS final = %v", dns["final"])
	}
	servers, _ := dns["servers"].([]any)
	tags := map[string]M{}
	for _, item := range servers {
		server, _ := item.(M)
		tags[stringVal(server["tag"])] = server
	}
	if _, hasDetour := tags["local"]["detour"]; hasDetour {
		t.Fatalf("domestic DoH must use the default direct dialer, not a detour: %+v", tags["local"])
	}
	if tags["remote"]["detour"] != "node-b" || tags["system"]["type"] != "local" {
		t.Fatalf("sing-box DNS servers wrong: %+v", servers)
	}
	dnsRules, _ := dns["rules"].([]any)
	if len(dnsRules) == 0 {
		t.Fatal("sing-box DNS rules missing")
	}
	resolveRemote := false
	for _, item := range rules {
		rule, _ := item.(M)
		if rule["action"] == "resolve" && rule["server"] == "remote" {
			resolveRemote = true
		}
	}
	if !resolveRemote {
		t.Fatal("sing-box bypass-cn resolve does not use remote DNS")
	}
}

func stringIndex(items []string, want string) int {
	for i, item := range items {
		if item == want {
			return i
		}
	}
	return -1
}

func TestCustomRulesDNSMapping(t *testing.T) {
	a := configTestApp(t)
	a.cfg.CustomRules = CustomRoutingRules{Block: []string{"ads.example"}, Direct: []string{"internal.example"}, Proxy: []string{"blocked.example"}}

	xcfg, _ := a.coreConfig("xray-x")
	xdns, _ := xcfg["dns"].(M)
	hosts, _ := xdns["hosts"].(M)
	if hosts["domain:ads.example"] != "#3" {
		t.Fatalf("blocked domain not rejected in xray DNS: %+v", xdns["hosts"])
	}
	xservers, _ := xdns["servers"].([]any)
	directIdx, proxyIdx, builtinIdx := -1, -1, -1
	for i, item := range xservers {
		server, _ := item.(M)
		domains, _ := server["domains"].([]string)
		switch {
		case stringIndex(domains, "domain:internal.example") >= 0:
			directIdx = i
		case stringIndex(domains, "domain:blocked.example") >= 0:
			proxyIdx = i
		case stringIndex(domains, xDomain("cn")) >= 0:
			builtinIdx = i
		}
	}
	if directIdx < 0 || proxyIdx < 0 || builtinIdx < 0 || !(directIdx < proxyIdx && proxyIdx < builtinIdx) {
		t.Fatalf("xray custom DNS priority wrong: direct=%d proxy=%d builtin=%d %+v", directIdx, proxyIdx, builtinIdx, xservers)
	}

	scfg, _ := a.coreConfig("sing-box-b")
	sdns, _ := scfg["dns"].(M)
	srules, _ := sdns["rules"].([]any)
	rejectIdx, directRuleIdx, proxyRuleIdx, builtinRuleIdx := -1, -1, -1, -1
	for i, item := range srules {
		rule, _ := item.(M)
		suffix, _ := rule["domain_suffix"].([]string)
		switch {
		case rule["action"] == "reject" && stringIndex(suffix, "ads.example") >= 0:
			if _, hasServer := rule["server"]; hasServer {
				t.Fatalf("DNS reject rule must not select a server: %+v", rule)
			}
			rejectIdx = i
		case rule["action"] == "route" && rule["server"] == "local" && stringIndex(suffix, "internal.example") >= 0:
			directRuleIdx = i
		case rule["action"] == "route" && rule["server"] == "remote" && stringIndex(suffix, "blocked.example") >= 0:
			proxyRuleIdx = i
		case rule["action"] == "route" && rule["rule_set"] != nil:
			if builtinRuleIdx < 0 {
				builtinRuleIdx = i
			}
		}
	}
	if rejectIdx < 0 || directRuleIdx < 0 || proxyRuleIdx < 0 || builtinRuleIdx < 0 || !(rejectIdx < directRuleIdx && directRuleIdx < proxyRuleIdx && proxyRuleIdx < builtinRuleIdx) {
		t.Fatalf("sing-box custom DNS priority wrong: reject=%d direct=%d proxy=%d builtin=%d %+v", rejectIdx, directRuleIdx, proxyRuleIdx, builtinRuleIdx, srules)
	}
}

func TestSingDNSNoDetourForDirectOutbound(t *testing.T) {
	a := configTestApp(t)
	a.cfg.Nodes[1].Outbound = M{"type": "direct"}
	cfg, _ := a.coreConfig("sing-box-b")
	dns, _ := cfg["dns"].(M)
	servers, _ := dns["servers"].([]any)
	for _, item := range servers {
		server, _ := item.(M)
		if _, hasDetour := server["detour"]; hasDetour {
			t.Fatalf("direct outbound must not be used as a DNS detour: %+v", server)
		}
	}
}

// TestGeneratedCoreConfigsValidate is opt-in: set MIRROXY_TEST_CORE_A,
// MIRROXY_TEST_CORE_B and MIRROXY_TEST_RESOURCES to the bundled cores and the
// bin/resources directory to run the real core config checkers.
func TestGeneratedCoreConfigsValidate(t *testing.T) {
	coreA, coreB := os.Getenv("MIRROXY_TEST_CORE_A"), os.Getenv("MIRROXY_TEST_CORE_B")
	resources := os.Getenv("MIRROXY_TEST_RESOURCES")
	if coreA == "" || coreB == "" || resources == "" {
		t.Skip("real core paths not configured")
	}
	a := configTestApp(t)
	a.cfg.CustomRules = CustomRoutingRules{Block: []string{"ads.example"}, Direct: []string{"internal.example"}, Proxy: []string{"blocked.example"}}
	target := filepath.Join(a.root, "bin", "resources")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(resources)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(resources, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(target, entry.Name()), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ engine, key, exe string }{
		{"xray", "xray-x", coreA},
		{"sing-box", "sing-box-b", coreB},
	} {
		cfg, _ := a.coreConfig(tc.key)
		path := filepath.Join(t.TempDir(), "config.json")
		if err = writeJSON(path, cfg); err != nil {
			t.Fatal(err)
		}
		if err = a.validateCore(tc.engine, path, tc.exe); err != nil {
			t.Fatal(err)
		}
	}
}
