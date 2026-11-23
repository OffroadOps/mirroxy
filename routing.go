package main

import (
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func (c CustomRoutingRules) sanitized() CustomRoutingRules {
	nonNil := func(items []string) []string {
		if items == nil {
			return []string{}
		}
		out := make([]string, len(items))
		copy(out, items)
		return out
	}
	return CustomRoutingRules{
		Proxy:  nonNil(c.Proxy),
		Direct: nonNil(c.Direct),
		Block:  nonNil(c.Block),
	}
}

type parsedCustomRule struct {
	Canonical string
	DedupKey  string
	IsIP      bool
	XrayVal   string
	SingField string
	SingVal   any
}

func tryParseCustomIP(raw string) (parsedCustomRule, bool) {
	s := strings.TrimSpace(raw)
	isAddr := !strings.Contains(s, "/")
	if isAddr {
		trimmed := strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
		if addr, err := netip.ParseAddr(trimmed); err == nil {
			addr = addr.Unmap()
			prefix := netip.PrefixFrom(addr, addr.BitLen()).Masked()
			return parsedCustomRule{
				Canonical: addr.String(),
				DedupKey:  "ip:" + prefix.String(),
				IsIP:      true,
				XrayVal:   prefix.String(),
				SingField: "ip_cidr",
				SingVal:   prefix.String(),
			}, true
		}
		return parsedCustomRule{}, false
	}
	if prefix, err := netip.ParsePrefix(s); err == nil {
		if prefix.Addr().Is4In6() {
			addr := prefix.Addr().Unmap()
			bits := prefix.Bits() - 96
			if bits < 0 {
				bits = 0
			}
			prefix = netip.PrefixFrom(addr, bits)
		}
		prefix = prefix.Masked()
		return parsedCustomRule{
			Canonical: prefix.String(),
			DedupKey:  "ip:" + prefix.String(),
			IsIP:      true,
			XrayVal:   prefix.String(),
			SingField: "ip_cidr",
			SingVal:   prefix.String(),
		}, true
	}
	return parsedCustomRule{}, false
}

func normalizeCustomDomain(raw string, allowSingleLabel bool) (string, error) {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "*.") {
		s = s[2:]
	}
	s = strings.TrimPrefix(s, ".")
	s = strings.TrimSuffix(s, ".")
	s = strings.ToLower(s)
	if s == "" || len(s) > 253 {
		return "", fmt.Errorf("无效域名规则：%s", raw)
	}
	if strings.ContainsAny(s, " \t\r\n/\\:?#@[]") {
		return "", fmt.Errorf("无效域名规则（不能包含路径、端口或空白）：%s", raw)
	}
	labels := strings.Split(s, ".")
	if !allowSingleLabel && len(labels) < 2 && s != "localhost" {
		return "", fmt.Errorf("无效域名规则（请使用完整域名或 keyword: 前缀）：%s", raw)
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", fmt.Errorf("无效域名规则：%s", raw)
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
				return "", fmt.Errorf("无效域名规则：%s", raw)
			}
		}
	}
	return s, nil
}

func parseCustomRule(raw string) (parsedCustomRule, bool, error) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
		return parsedCustomRule{}, false, nil
	}
	if strings.ContainsAny(line, "\r\n") {
		return parsedCustomRule{}, false, errors.New("规则不能包含换行符")
	}
	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "ip:") || strings.HasPrefix(lower, "cidr:") {
		idx := strings.IndexByte(line, ':')
		rest := strings.TrimSpace(line[idx+1:])
		if rule, ok := tryParseCustomIP(rest); ok {
			return rule, true, nil
		}
		return parsedCustomRule{}, false, fmt.Errorf("无效 IP/CIDR 规则：%s", line)
	}
	if rule, ok := tryParseCustomIP(line); ok {
		return rule, true, nil
	}
	if strings.HasPrefix(lower, "regexp:") {
		pat := strings.TrimSpace(line[len("regexp:"):])
		if pat == "" {
			return parsedCustomRule{}, false, fmt.Errorf("空正则规则：%s", line)
		}
		if _, err := regexp.Compile(pat); err != nil {
			return parsedCustomRule{}, false, fmt.Errorf("无效正则规则 %s：%v", line, err)
		}
		return parsedCustomRule{
			Canonical: "regexp:" + pat,
			DedupKey:  "regexp:" + pat,
			XrayVal:   "regexp:" + pat,
			SingField: "domain_regex",
			SingVal:   pat,
		}, true, nil
	}
	if strings.ContainsAny(line, " \t") {
		return parsedCustomRule{}, false, fmt.Errorf("规则不能包含空白字符：%s", line)
	}
	if idx := strings.IndexByte(line, ':'); idx >= 0 {
		prefix := lower[:idx]
		rest := strings.TrimSpace(line[idx+1:])
		switch prefix {
		case "domain":
			dom, err := normalizeCustomDomain(rest, true)
			if err != nil {
				return parsedCustomRule{}, false, err
			}
			return parsedCustomRule{
				Canonical: dom,
				DedupKey:  "domain:" + dom,
				XrayVal:   "domain:" + dom,
				SingField: "domain_suffix",
				SingVal:   dom,
			}, true, nil
		case "full":
			dom, err := normalizeCustomDomain(rest, true)
			if err != nil {
				return parsedCustomRule{}, false, err
			}
			return parsedCustomRule{
				Canonical: "full:" + dom,
				DedupKey:  "full:" + dom,
				XrayVal:   "full:" + dom,
				SingField: "domain",
				SingVal:   dom,
			}, true, nil
		case "keyword":
			kw := strings.ToLower(rest)
			if kw == "" || strings.ContainsAny(kw, "/\\?#") {
				return parsedCustomRule{}, false, fmt.Errorf("无效关键词规则：%s", line)
			}
			return parsedCustomRule{
				Canonical: "keyword:" + kw,
				DedupKey:  "keyword:" + kw,
				XrayVal:   "keyword:" + kw,
				SingField: "domain_keyword",
				SingVal:   kw,
			}, true, nil
		case "geosite":
			tag := strings.ToLower(rest)
			if ruleAssetName("geosite-"+tag) == "" {
				return parsedCustomRule{}, false, fmt.Errorf("不支持的 geosite 标签（支持 private/cn/google/gfw/greatfire）：%s", line)
			}
			return parsedCustomRule{
				Canonical: "geosite:" + tag,
				DedupKey:  "geosite:" + tag,
				XrayVal:   xDomain(tag),
				SingField: "rule_set",
				SingVal:   "geosite-" + tag,
			}, true, nil
		case "geoip":
			tag := strings.ToLower(rest)
			if tag == "private" {
				return parsedCustomRule{
					Canonical: "geoip:private",
					DedupKey:  "geoip:private",
					IsIP:      true,
					XrayVal:   xIP("private"),
					SingField: "ip_is_private",
					SingVal:   true,
				}, true, nil
			}
			if ruleAssetName("geoip-"+tag) == "" {
				return parsedCustomRule{}, false, fmt.Errorf("不支持的 geoip 标签（支持 private/cn/google/netflix/telegram/twitter/facebook/fastly）：%s", line)
			}
			return parsedCustomRule{
				Canonical: "geoip:" + tag,
				DedupKey:  "geoip:" + tag,
				IsIP:      true,
				XrayVal:   xIP(tag),
				SingField: "rule_set",
				SingVal:   "geoip-" + tag,
			}, true, nil
		default:
			return parsedCustomRule{}, false, fmt.Errorf("不支持的规则前缀或协议：%s", line)
		}
	}
	dom, err := normalizeCustomDomain(line, false)
	if err != nil {
		return parsedCustomRule{}, false, err
	}
	return parsedCustomRule{
		Canonical: dom,
		DedupKey:  "domain:" + dom,
		XrayVal:   "domain:" + dom,
		SingField: "domain_suffix",
		SingVal:   dom,
	}, true, nil
}

func normalizeCustomRuleList(items []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range items {
		rule, ok, err := parseCustomRule(raw)
		if err != nil {
			return nil, err
		}
		if !ok || seen[rule.DedupKey] {
			continue
		}
		seen[rule.DedupKey] = true
		out = append(out, rule.Canonical)
	}
	return out, nil
}

func normalizeCustomRules(input CustomRoutingRules) (CustomRoutingRules, error) {
	proxy, err := normalizeCustomRuleList(input.Proxy)
	if err != nil {
		return CustomRoutingRules{}, err
	}
	direct, err := normalizeCustomRuleList(input.Direct)
	if err != nil {
		return CustomRoutingRules{}, err
	}
	block, err := normalizeCustomRuleList(input.Block)
	if err != nil {
		return CustomRoutingRules{}, err
	}
	return CustomRoutingRules{Proxy: proxy, Direct: direct, Block: block}, nil
}

func (c CustomRoutingRules) hasIP() bool {
	for _, list := range [][]string{c.Block, c.Direct, c.Proxy} {
		for _, item := range list {
			if rule, ok, err := parseCustomRule(item); err == nil && ok && rule.IsIP {
				return true
			}
		}
	}
	return false
}

// Called with a.mu held.
func (a *App) setCustomRoutingRules(input CustomRoutingRules) error {
	if a.busyUpdate {
		return errors.New("核心更新中，请稍后")
	}
	normalized, err := normalizeCustomRules(input)
	if err != nil {
		return err
	}
	before := cloneSettings(a.cfg)
	a.cfg.CustomRules = normalized
	if err := a.reconcile(); err != nil {
		a.cfg = before
		_ = a.reconcile()
		return err
	}
	if err := a.save(); err != nil {
		a.cfg = before
		_ = a.reconcile()
		return err
	}
	a.event(fmt.Sprintf("自定义分流规则已保存（代理 %d / 直连 %d / 拦截 %d）", len(normalized.Proxy), len(normalized.Direct), len(normalized.Block)))
	return nil
}

func validRoutingMode(mode string) bool {
	return mode == "bypass-cn" || mode == "blacklist" || mode == "proxy"
}
func validNodeRoute(mode string) bool { return mode == "inherit" || validRoutingMode(mode) }
func (a *App) effectiveRoute(n Node) string {
	if validRoutingMode(n.Route) {
		return n.Route
	}
	return first(a.cfg.RoutingMode, "bypass-cn")
}

// Called with a.mu held. Explicit per-node routes remain independent.
func (a *App) setRoutingMode(mode string) error {
	if !validRoutingMode(mode) {
		return errors.New("无效路由模式")
	}
	if a.busyUpdate {
		return errors.New("核心更新中，请稍后")
	}
	before := cloneSettings(a.cfg)
	a.cfg.RoutingMode = mode
	if err := a.reconcile(); err != nil {
		a.cfg = before
		_ = a.reconcile()
		return err
	}
	if err := a.save(); err != nil {
		a.cfg = before
		_ = a.reconcile()
		return err
	}
	a.event("全局路由模式已应用：" + mode)
	a.refreshTray()
	return nil
}

var blacklistSiteSets = []string{"geosite-google", "geosite-gfw", "geosite-greatfire"}
var blacklistIPSets = []string{"geoip-facebook", "geoip-fastly", "geoip-google", "geoip-netflix", "geoip-telegram", "geoip-twitter"}
var proxyDNSIPs = []string{"1.1.1.1", "1.0.0.1", "8.8.8.8", "8.8.4.4", "9.9.9.9", "149.112.112.112", "2606:4700:4700::1111", "2606:4700:4700::1001", "2001:4860:4860::8888", "2001:4860:4860::8844"}
var proxyDNSDomains = []string{"cloudflare-dns.com", "one.one.one.one", "dns.google", "quad9.net"}

func ruleAssetName(name string) string {
	names := map[string]string{
		"geoip": "Mirroxy_route_ip.bin", "geosite": "Mirroxy_route_domain.bin",
		"geoip-cn": "Mirroxy_set_01.bin", "geosite-cn": "Mirroxy_set_02.bin", "geosite-private": "Mirroxy_set_03.bin",
		"geosite-gfw": "Mirroxy_set_04.bin", "geosite-greatfire": "Mirroxy_set_05.bin", "geosite-google": "Mirroxy_set_06.bin",
		"geoip-facebook": "Mirroxy_set_07.bin", "geoip-fastly": "Mirroxy_set_08.bin", "geoip-google": "Mirroxy_set_09.bin",
		"geoip-netflix": "Mirroxy_set_10.bin", "geoip-telegram": "Mirroxy_set_11.bin", "geoip-twitter": "Mirroxy_set_12.bin",
	}
	return names[name]
}
func xDomain(tag string) string { return "ext:" + ruleAssetName("geosite") + ":" + tag }
func xIP(tag string) string     { return "ext-ip:" + ruleAssetName("geoip") + ":" + tag }

type compiledCustomBucket struct {
	xrayDomains []string
	xrayIPs     []string
	singDomain  []string
	singSuffix  []string
	singKeyword []string
	singRegex   []string
	singSets    []string
	singIPCIDR  []string
	singIPSets  []string
	singPrivate bool
}

func compileCustomBucket(items []string) compiledCustomBucket {
	var b compiledCustomBucket
	for _, raw := range items {
		rule, ok, err := parseCustomRule(raw)
		if err != nil || !ok {
			continue
		}
		if rule.IsIP {
			b.xrayIPs = append(b.xrayIPs, rule.XrayVal)
			switch rule.SingField {
			case "ip_is_private":
				b.singPrivate = true
			case "rule_set":
				b.singIPSets = append(b.singIPSets, rule.SingVal.(string))
			case "ip_cidr":
				b.singIPCIDR = append(b.singIPCIDR, rule.SingVal.(string))
			}
			continue
		}
		b.xrayDomains = append(b.xrayDomains, rule.XrayVal)
		switch rule.SingField {
		case "domain":
			b.singDomain = append(b.singDomain, rule.SingVal.(string))
		case "domain_suffix":
			b.singSuffix = append(b.singSuffix, rule.SingVal.(string))
		case "domain_keyword":
			b.singKeyword = append(b.singKeyword, rule.SingVal.(string))
		case "domain_regex":
			b.singRegex = append(b.singRegex, rule.SingVal.(string))
		case "rule_set":
			b.singSets = append(b.singSets, rule.SingVal.(string))
		}
	}
	return b
}

func xrayNodeRules(in, tag, mode string, bypassLAN bool) []any {
	return xrayNodeRulesWithCustom(in, tag, mode, bypassLAN, CustomRoutingRules{})
}

func xrayNodeRulesWithCustom(in, tag, mode string, bypassLAN bool, custom CustomRoutingRules) []any {
	rules := []any{}
	add := func(key string, values []string, target string) {
		r := M{"type": "field", "inboundTag": []string{in}, "outboundTag": target}
		if key != "" {
			r[key] = values
		}
		rules = append(rules, r)
	}
	blockBucket := compileCustomBucket(custom.Block)
	directBucket := compileCustomBucket(custom.Direct)
	proxyBucket := compileCustomBucket(custom.Proxy)
	customEntries := []struct {
		target string
		bucket compiledCustomBucket
	}{
		{"block", blockBucket},
		{"direct", directBucket},
		{tag, proxyBucket},
	}
	for _, entry := range customEntries {
		if len(entry.bucket.xrayDomains) > 0 {
			add("domain", entry.bucket.xrayDomains, entry.target)
		}
	}
	addCustomIPs := func() {
		for _, entry := range customEntries {
			if len(entry.bucket.xrayIPs) > 0 {
				add("ip", entry.bucket.xrayIPs, entry.target)
			}
		}
	}
	switch mode {
	case "bypass-cn":
		add("domain", []string{xDomain("private"), xDomain("cn")}, "direct")
		addCustomIPs()
		add("ip", []string{xIP("private"), xIP("cn")}, "direct")
	case "blacklist":
		add("domain", []string{xDomain("private")}, "direct")
		domains := []string{xDomain("google"), xDomain("gfw"), xDomain("greatfire")}
		for _, domain := range proxyDNSDomains {
			domains = append(domains, "domain:"+domain)
		}
		add("domain", domains, tag)
		addCustomIPs()
		add("ip", []string{xIP("private")}, "direct")
		ips := append([]string{}, proxyDNSIPs...)
		for _, set := range blacklistIPSets {
			ips = append(ips, xIP(set[len("geoip-"):]))
		}
		add("ip", ips, tag)
		add("", nil, "direct")
		return rules
	default:
		// Global mode keeps LAN and loopback traffic direct when the toggle is on,
		// while explicit custom rules take precedence before built-in LAN rules.
		if bypassLAN {
			add("domain", []string{xDomain("private")}, "direct")
		}
		addCustomIPs()
		if bypassLAN {
			add("ip", []string{xIP("private")}, "direct")
		}
	}
	add("", nil, tag)
	return rules
}

func singNodeRules(in, tag, mode string, sets map[string]bool, bypassLAN bool) []any {
	return singNodeRulesWithCustom(in, tag, mode, sets, bypassLAN, CustomRoutingRules{})
}

func singNodeRulesWithCustom(in, tag, mode string, sets map[string]bool, bypassLAN bool, custom CustomRoutingRules) []any {
	rules := []any{}
	add := func(fields M, target string) {
		fields["inbound"] = []string{in}
		if target == "reject" {
			fields["action"] = "reject"
		} else {
			fields["action"] = "route"
			fields["outbound"] = target
		}
		rules = append(rules, fields)
	}
	useSets := func(names []string, target string) {
		for _, name := range names {
			sets[name] = true
		}
		add(M{"rule_set": names}, target)
	}
	blockBucket := compileCustomBucket(custom.Block)
	directBucket := compileCustomBucket(custom.Direct)
	proxyBucket := compileCustomBucket(custom.Proxy)
	customEntries := []struct {
		target string
		bucket compiledCustomBucket
	}{
		{"reject", blockBucket},
		{"direct", directBucket},
		{tag, proxyBucket},
	}
	for _, entry := range customEntries {
		if len(entry.bucket.singSets) > 0 {
			useSets(entry.bucket.singSets, entry.target)
		}
		if len(entry.bucket.singDomain) > 0 {
			add(M{"domain": entry.bucket.singDomain}, entry.target)
		}
		if len(entry.bucket.singSuffix) > 0 {
			add(M{"domain_suffix": entry.bucket.singSuffix}, entry.target)
		}
		if len(entry.bucket.singKeyword) > 0 {
			add(M{"domain_keyword": entry.bucket.singKeyword}, entry.target)
		}
		if len(entry.bucket.singRegex) > 0 {
			add(M{"domain_regex": entry.bucket.singRegex}, entry.target)
		}
	}
	if mode != "proxy" {
		useSets([]string{"geosite-private"}, "direct")
		if mode == "bypass-cn" {
			useSets([]string{"geosite-cn"}, "direct")
		}
		if mode == "blacklist" {
			useSets(blacklistSiteSets, tag)
			add(M{"domain_suffix": proxyDNSDomains}, tag)
		}
	} else if bypassLAN {
		useSets([]string{"geosite-private"}, "direct")
	}
	hasCustomIP := false
	for _, entry := range customEntries {
		if entry.bucket.singPrivate || len(entry.bucket.singIPSets) > 0 || len(entry.bucket.singIPCIDR) > 0 {
			hasCustomIP = true
			break
		}
	}
	if mode != "proxy" || bypassLAN || hasCustomIP {
		// Only unmatched domains reach DNS resolution, then IP-based routing.
		rules = append(rules, M{"inbound": []string{in}, "action": "resolve", "server": "local"})
	}
	for _, entry := range customEntries {
		if entry.bucket.singPrivate {
			add(M{"ip_is_private": true}, entry.target)
		}
		if len(entry.bucket.singIPSets) > 0 {
			useSets(entry.bucket.singIPSets, entry.target)
		}
		if len(entry.bucket.singIPCIDR) > 0 {
			add(M{"ip_cidr": entry.bucket.singIPCIDR}, entry.target)
		}
	}
	if mode != "proxy" {
		add(M{"ip_is_private": true}, "direct")
		if mode == "bypass-cn" {
			useSets([]string{"geoip-cn"}, "direct")
		}
		if mode == "blacklist" {
			useSets(blacklistIPSets, tag)
			add(M{"ip_cidr": proxyDNSIPs}, tag)
			add(M{}, "direct")
			return rules
		}
	} else if bypassLAN {
		add(M{"ip_is_private": true}, "direct")
	}
	add(M{}, tag)
	return rules
}

func (a *App) singRuleSets(sets map[string]bool) []any {
	names := []string{}
	for name := range sets {
		names = append(names, name)
	}
	sort.Strings(names)
	rules := []any{}
	for _, name := range names {
		rules = append(rules, M{"type": "local", "tag": name, "format": "binary", "path": filepath.Join(a.root, "bin", "resources", ruleAssetName(name))})
	}
	return rules
}
