package main

import (
	"context"
	"fmt"
	"io"
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

func TestRoutingMigrationAndOverrides(t *testing.T) {
	a := &App{root: t.TempDir(), cores: map[string]*Core{}}
	old := Settings{DefaultRoute: "proxy", Nodes: []Node{{ID: "a", Name: "alias", OriginalName: "provider", Port: 5067, Route: "proxy"}, {ID: "b", Route: "bypass-cn"}}}
	if err := writeJSON(a.dataPath("settings.json"), old); err != nil {
		t.Fatal(err)
	}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}
	n := a.cfg.Nodes[0]
	if a.cfg.RoutingMode != "bypass-cn" || a.cfg.DefaultRoute != "inherit" || n.Route != "inherit" || n.ID != "a" || n.Name != "alias" || n.OriginalName != "provider" || n.Port != 5067 {
		t.Fatal("legacy migration lost identity or default")
	}
	if err := a.setRoutingMode("proxy"); err != nil {
		t.Fatal(err)
	}
	if a.effectiveRoute(n) != "proxy" || a.effectiveRoute(a.cfg.Nodes[1]) != "bypass-cn" {
		t.Fatal("global routing overwrote node override")
	}
	if err := a.setRoutingMode("invalid"); err == nil || a.cfg.RoutingMode != "proxy" {
		t.Fatal("invalid route accepted")
	}
}

func TestRoutingWithInstalledCores(t *testing.T) {
	rules := os.Getenv("MIRROXY_TEST_RULES")
	if rules == "" {
		t.Skip("set MIRROXY_TEST_RULES for real split routing")
	}
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "DIRECT") }))
	defer direct.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" {
			// A plain request is forwarded to the upstream proxy in absolute
			// form instead of being tunnelled.
			w.Header().Set("Content-Length", "5")
			w.WriteHeader(200)
			fmt.Fprint(w, "PROXY")
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		fmt.Fprint(rw, "HTTP/1.1 200 Connection established\r\n\r\n")
		rw.Flush()
		if _, err = http.ReadRequest(rw.Reader); err != nil {
			return
		}
		fmt.Fprint(rw, "HTTP/1.1 200 OK\r\nContent-Length: 5\r\nConnection: close\r\n\r\nPROXY")
		rw.Flush()
		// Drain the tunnel before it is closed. Closing with unread data
		// makes the peer see a reset instead of the answer.
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		io.Copy(io.Discard, conn)
	}))
	defer proxy.Close()
	for _, engine := range []string{"xray", "sing-box"} {
		t.Run(engine, func(t *testing.T) {
			source := os.Getenv(map[string]string{"xray": "MIRROXY_TEST_XRAY", "sing-box": "MIRROXY_TEST_SINGBOX"}[engine])
			if source == "" {
				t.Skip("missing core")
			}
			a := &App{root: t.TempDir(), panelPort: 15060, cores: map[string]*Core{}}
			if err := a.load(); err != nil {
				t.Fatal(err)
			}
			// Global mode must really send every request through the node, so
			// the LAN bypass is switched off for this proof.
			bypassLAN := false
			a.cfg.BypassLAN = &bypassLAN
			copyFile := func(src, dst string) {
				t.Helper()
				data, err := os.ReadFile(src)
				if err != nil {
					t.Fatal(err)
				}
				os.MkdirAll(filepath.Dir(dst), 0700)
				if err = os.WriteFile(dst, data, 0700); err != nil {
					t.Fatal(err)
				}
			}
			copyFile(source, a.corePath(engine))
			for _, name := range []string{"geoip.dat", "geosite.dat"} {
				key := name[:len(name)-len(filepath.Ext(name))]
				source := filepath.Join(rules, name)
				if _, statErr := os.Stat(source); os.IsNotExist(statErr) {
					source = filepath.Join(rules, ruleAssetName(key))
				}
				copyFile(source, filepath.Join(a.root, "bin", "resources", ruleAssetName(key)))
			}
			entries, _ := filepath.Glob(filepath.Join(rules, "srss", "*.srs"))
			if len(entries) > 0 {
				for _, p := range entries {
					name := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
					asset := ruleAssetName(name)
					if asset != "" {
						copyFile(p, filepath.Join(a.root, "bin", "resources", asset))
					}
				}
			} else {
				entries, _ = filepath.Glob(filepath.Join(rules, "Mirroxy_set_*.bin"))
				for _, p := range entries {
					copyFile(p, filepath.Join(a.root, "bin", "resources", filepath.Base(p)))
				}
			}
			n, err := parseLink(proxy.URL)
			if err != nil {
				t.Fatal(err)
			}
			n.ID = "routing-proof"
			n.Engine = engine
			n.Route = "inherit"
			if engine == "sing-box" {
				u, _ := url.Parse(proxy.URL)
				p, _ := strconv.Atoi(u.Port())
				n.Outbound = M{"type": "http", "server": u.Hostname(), "server_port": p}
			}
			a.cfg.Nodes = []Node{n}
			a.cfg.LogLevel = "debug"
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
			if err = a.changeNode(n.ID, "start", ""); err != nil {
				t.Fatal(err)
			}
			port := a.cfg.Nodes[0].Port
			get := func(target, want string) {
				t.Helper()
				p, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
				tr := &http.Transport{Proxy: http.ProxyURL(p), DisableKeepAlives: true}
				defer tr.CloseIdleConnections()
				c := &http.Client{Transport: tr, Timeout: 8 * time.Second}
				// The mock upstream tears its tunnels down on its own schedule,
				// so a dropped connection is retried before it counts as a
				// failure.
				var body []byte
				var status int
				var last error
				for attempt := 0; attempt < 4; attempt++ {
					res, err := c.Get(target)
					if err != nil {
						last = err
						time.Sleep(150 * time.Millisecond)
						continue
					}
					body, err = io.ReadAll(res.Body)
					res.Body.Close()
					if err != nil {
						last = err
						time.Sleep(150 * time.Millisecond)
						continue
					}
					status = res.StatusCode
					last = nil
					break
				}
				if last != nil || string(body) != want {
					t.Fatalf("%s %s %s: status %d got %q, %v", engine, a.cfg.RoutingMode, target, status, body, last)
				}
			}
			for _, mode := range []string{"bypass-cn", "blacklist", "proxy"} {
				if err = a.setRoutingMode(mode); err != nil {
					t.Fatal(err)
				}
				want := "DIRECT"
				if mode == "proxy" {
					want = "PROXY"
				}
				get(direct.URL, want)
				if mode == "blacklist" {
					get("http://www.google.com/", "PROXY")
				}
				if a.cfg.Nodes[0].Port != port {
					t.Fatal("routing changed assigned port")
				}
			}
		})
	}
}

func TestGlobalModeBypassLANDirect(t *testing.T) {
	sets := map[string]bool{}
	rules := singNodeRules("in-1", "node-x", "proxy", sets, true)
	last := rules[len(rules)-1].(M)
	if last["outbound"] != "node-x" {
		t.Fatalf("global catch-all must stay on the node: %+v", last)
	}
	directPrivate := false
	for _, raw := range rules {
		rule := raw.(M)
		if names, ok := rule["rule_set"].([]string); ok && len(names) == 1 && names[0] == "geosite-private" && rule["outbound"] == "direct" {
			directPrivate = true
		}
	}
	if !directPrivate {
		t.Fatalf("geosite-private direct rule missing: %+v", rules)
	}
	if !sets["geosite-private"] {
		t.Fatal("geosite-private must be registered in the rule-set map")
	}

	sets = map[string]bool{}
	rules = singNodeRules("in-1", "node-x", "proxy", sets, false)
	if len(rules) != 1 || rules[0].(M)["outbound"] != "node-x" {
		t.Fatalf("global mode without LAN bypass must be a single rule: %+v", rules)
	}

	xrules := xrayNodeRules("in-1", "node-x", "proxy", true)
	xlast := xrules[len(xrules)-1].(M)
	if xlast["outboundTag"] != "node-x" {
		t.Fatalf("xray global catch-all must stay on the node: %+v", xlast)
	}
	privateDirect := false
	for _, raw := range xrules {
		rule := raw.(M)
		if rule["outboundTag"] == "direct" && rule["domain"] != nil {
			privateDirect = true
		}
	}
	if !privateDirect {
		t.Fatalf("xray LAN bypass direct rule missing: %+v", xrules)
	}
	xrules = xrayNodeRules("in-1", "node-x", "proxy", false)
	if len(xrules) != 1 || xrules[0].(M)["outboundTag"] != "node-x" {
		t.Fatalf("xray global mode without bypass must be a single rule: %+v", xrules)
	}
}

func TestCustomRoutingNormalizationAndValidation(t *testing.T) {
	in := CustomRoutingRules{
		Proxy:  []string{"", " # comment ", "domain:Example.COM.", "*.example.com", ".example.com", "full:API.Test.com.", "keyword:OpenAI"},
		Direct: []string{"// note", "10.0.0.5/8", "ip:1.1.1.1", "1.1.1.1", "geosite:CN", "geoip:Private"},
		Block:  []string{`regexp:^ads\.`, "bad.example"},
	}
	got, err := normalizeCustomRules(in)
	if err != nil {
		t.Fatalf("normalizeCustomRules failed: %v", err)
	}
	wantProxy := []string{"example.com", "full:api.test.com", "keyword:openai"}
	wantDirect := []string{"10.0.0.0/8", "1.1.1.1", "geosite:cn", "geoip:private"}
	wantBlock := []string{`regexp:^ads\.`, "bad.example"}
	if fmt.Sprint(got.Proxy) != fmt.Sprint(wantProxy) {
		t.Fatalf("normalized Proxy = %v, want %v", got.Proxy, wantProxy)
	}
	if fmt.Sprint(got.Direct) != fmt.Sprint(wantDirect) {
		t.Fatalf("normalized Direct = %v, want %v", got.Direct, wantDirect)
	}
	if fmt.Sprint(got.Block) != fmt.Sprint(wantBlock) {
		t.Fatalf("normalized Block = %v, want %v", got.Block, wantBlock)
	}
	if !got.hasIP() {
		t.Fatal("expected hasIP() to be true for CIDR/IP/geoip rules")
	}
	if (CustomRoutingRules{Proxy: []string{"example.com"}}).hasIP() {
		t.Fatal("expected domain-only custom rules hasIP() to be false")
	}

	invalidCases := []string{
		"https://example.com",
		"ip:999.999.1.1",
		"regexp:[unclosed",
		"foo bar.com",
		"geosite:unknown-xyz",
		"geoip:unknown-xyz",
		"example.com/path",
	}
	for _, bad := range invalidCases {
		if _, err := normalizeCustomRules(CustomRoutingRules{Proxy: []string{bad}}); err == nil {
			t.Fatalf("expected error for invalid rule %q", bad)
		}
	}
}

func TestCustomRoutingPrecedenceAndCoreConfigs(t *testing.T) {
	custom := CustomRoutingRules{
		Block:  []string{"ads.example.com", "9.9.9.9"},
		Direct: []string{"internal.corp", "8.8.8.0/24", "keyword:cnki"},
		Proxy:  []string{"full:override.cn", "114.114.114.114"},
	}

	// Xray priority: custom domains (block -> direct -> proxy) before bypass-cn domains,
	// custom IPs (block -> direct -> proxy) before bypass-cn IPs.
	xr := xrayNodeRulesWithCustom("in-1", "node-1", "bypass-cn", true, custom)
	findXrayRule := func(target string, hasIP bool) int {
		for i, raw := range xr {
			r := raw.(M)
			if r["outboundTag"] != target {
				continue
			}
			if hasIP && r["ip"] != nil {
				return i
			}
			if !hasIP && r["domain"] != nil {
				return i
			}
		}
		return -1
	}
	bDom, dDom, pDom := findXrayRule("block", false), findXrayRule("direct", false), findXrayRule("node-1", false)
	bIP, dIP, pIP := findXrayRule("block", true), findXrayRule("direct", true), findXrayRule("node-1", true)
	if !(bDom >= 0 && bDom < dDom && dDom < pDom) {
		t.Fatalf("expected xray domain order block < direct < proxy, got %d %d %d (%+v)", bDom, dDom, pDom, xr)
	}
	if !(bIP >= 0 && bIP < dIP && dIP < pIP && pDom < bIP) {
		t.Fatalf("expected xray IP order block < direct < proxy after domain rules, got %d %d %d (%+v)", bIP, dIP, pIP, xr)
	}

	// Sing-box: never mix distinct match fields in one rule object; block uses action="reject".
	sets := map[string]bool{}
	sr := singNodeRulesWithCustom("in-1", "node-1", "proxy", sets, false, custom)
	resolveIdx := -1
	rejectDomainIdx, rejectIPIdx := -1, -1
	for i, raw := range sr {
		r := raw.(M)
		if r["action"] == "resolve" {
			resolveIdx = i
		}
		if r["action"] == "reject" {
			if _, hasOut := r["outbound"]; hasOut {
				t.Fatalf("sing-box reject rule must not contain outbound: %+v", r)
			}
			if r["domain_suffix"] != nil {
				rejectDomainIdx = i
			}
			if r["ip_cidr"] != nil {
				rejectIPIdx = i
			}
		}
		matchFields := 0
		for _, k := range []string{"domain", "domain_suffix", "domain_keyword", "domain_regex", "rule_set", "ip_cidr", "ip_is_private"} {
			if r[k] != nil {
				matchFields++
			}
		}
		if matchFields > 1 {
			t.Fatalf("sing-box rule combined multiple AND match fields: %+v", r)
		}
	}
	if resolveIdx <= rejectDomainIdx || rejectIPIdx <= resolveIdx {
		t.Fatalf("expected sing-box domain_reject (%d) < resolve (%d) < ip_reject (%d)", rejectDomainIdx, resolveIdx, rejectIPIdx)
	}

	// coreConfig sets IPOnDemand in proxy mode when custom IP rules are present.
	bypassLAN := false
	a := &App{
		root: t.TempDir(), cores: map[string]*Core{}, traffic: map[string]*Traffic{},
		cfg: Settings{
			RoutingMode: "proxy",
			BypassLAN:   &bypassLAN,
			CustomRules: custom,
			Nodes:       []Node{{ID: "x1", Engine: "xray", Enabled: true, Port: 5061, Outbound: M{"protocol": "freedom"}}},
			Updated:     map[string]string{},
		},
	}
	cfg, _ := a.coreConfig("xray-x1")
	routingCfg := cfg["routing"].(M)
	if routingCfg["domainStrategy"] != "IPOnDemand" {
		t.Fatalf("expected IPOnDemand in global mode with custom IP rules, got %v", routingCfg["domainStrategy"])
	}
}

func TestCustomRoutingPersistenceAndAPI(t *testing.T) {
	root := t.TempDir()
	a := &App{root: root, panelPort: 15060, cores: map[string]*Core{}, traffic: map[string]*Traffic{}}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}

	localCtx := context.WithValue(context.Background(), localAccessKey{}, true)
	// Save via /api/routing/rules
	req := httptest.NewRequest(http.MethodPost, "/api/routing/rules", strings.NewReader(`{"proxy":["*.OpenAI.com"],"direct":["10.0.0.0/8"],"block":["ads.test"]}`)).WithContext(localCtx)
	rec := httptest.NewRecorder()
	a.api(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/routing/rules failed: %d %s", rec.Code, rec.Body.String())
	}

	// Verify persistence after reload
	b := &App{root: root, panelPort: 15060, cores: map[string]*Core{}, traffic: map[string]*Traffic{}}
	if err := b.load(); err != nil {
		t.Fatal(err)
	}
	if len(b.cfg.CustomRules.Proxy) != 1 || b.cfg.CustomRules.Proxy[0] != "openai.com" || len(b.cfg.CustomRules.Direct) != 1 || b.cfg.CustomRules.Direct[0] != "10.0.0.0/8" || len(b.cfg.CustomRules.Block) != 1 || b.cfg.CustomRules.Block[0] != "ads.test" {
		t.Fatalf("reloaded custom rules mismatch: %+v", b.cfg.CustomRules)
	}

	// GET & POST /api/v1/routing/rules
	v1Get := httptest.NewRecorder()
	b.apiV1(v1Get, httptest.NewRequest(http.MethodGet, "/api/v1/routing/rules", nil).WithContext(localCtx))
	if v1Get.Code != http.StatusOK || !strings.Contains(v1Get.Body.String(), "openai.com") {
		t.Fatalf("GET /api/v1/routing/rules unexpected: %d %s", v1Get.Code, v1Get.Body.String())
	}
	v1PostReq := httptest.NewRequest(http.MethodPost, "/api/v1/routing/rules", strings.NewReader(`{"proxy":["claude.ai"],"direct":["127.0.0.1"],"block":[]}`)).WithContext(localCtx)
	v1PostReq.Header.Set("Content-Type", "application/json")
	v1Post := httptest.NewRecorder()
	b.apiV1(v1Post, v1PostReq)
	if v1Post.Code != http.StatusOK || len(b.cfg.CustomRules.Proxy) != 1 || b.cfg.CustomRules.Proxy[0] != "claude.ai" {
		t.Fatalf("POST /api/v1/routing/rules failed: %d %s", v1Post.Code, v1Post.Body.String())
	}

	// Non-local remote modification rejected on non-server platforms.
	if !serverPlatform() {
		remoteReq := httptest.NewRequest(http.MethodPost, "/api/routing/rules", strings.NewReader(`{"proxy":["evil.com"]}`))
		remoteReq = remoteReq.WithContext(context.WithValue(remoteReq.Context(), localAccessKey{}, false))
		remoteRec := httptest.NewRecorder()
		b.api(remoteRec, remoteReq)
		if !strings.Contains(remoteRec.Body.String(), "本机面板") {
			t.Fatalf("expected remote custom rules write rejection, got %d %s", remoteRec.Code, remoteRec.Body.String())
		}
	}
}
