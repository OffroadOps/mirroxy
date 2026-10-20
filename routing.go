package main

import (
	"errors"
	"path/filepath"
	"sort"
)

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

func xrayNodeRules(in, tag, mode string, bypassLAN bool) []any {
	rules := []any{}
	add := func(key string, values []string, target string) {
		r := M{"type": "field", "inboundTag": []string{in}, "outboundTag": target}
		if key != "" {
			r[key] = values
		}
		rules = append(rules, r)
	}
	switch mode {
	case "bypass-cn":
		add("domain", []string{xDomain("private"), xDomain("cn")}, "direct")
		add("ip", []string{xIP("private"), xIP("cn")}, "direct")
	case "blacklist":
		add("domain", []string{xDomain("private")}, "direct")
		domains := []string{xDomain("google"), xDomain("gfw"), xDomain("greatfire")}
		for _, domain := range proxyDNSDomains {
			domains = append(domains, "domain:"+domain)
		}
		add("domain", domains, tag)
		add("ip", []string{xIP("private")}, "direct")
		ips := append([]string{}, proxyDNSIPs...)
		for _, set := range blacklistIPSets {
			ips = append(ips, xIP(set[len("geoip-"):]))
		}
		add("ip", ips, tag)
		add("", nil, "direct")
		return rules
	}
	// Global mode keeps LAN and loopback traffic direct when the toggle is on.
	if bypassLAN {
		add("domain", []string{xDomain("private")}, "direct")
		add("ip", []string{xIP("private")}, "direct")
	}
	add("", nil, tag)
	return rules
}

func singNodeRules(in, tag, mode string, sets map[string]bool, bypassLAN bool) []any {
	rules := []any{}
	add := func(fields M, target string) {
		fields["inbound"] = []string{in}
		fields["action"] = "route"
		fields["outbound"] = target
		rules = append(rules, fields)
	}
	useSets := func(names []string, target string) {
		for _, name := range names {
			sets[name] = true
		}
		add(M{"rule_set": names}, target)
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
		// Only unmatched domains reach DNS resolution, then IP-based routing.
		rules = append(rules, M{"inbound": []string{in}, "action": "resolve", "server": "local"})
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
	}
	// Global mode keeps LAN and loopback traffic direct when the toggle is on.
	if mode == "proxy" && bypassLAN {
		useSets([]string{"geosite-private"}, "direct")
		rules = append(rules, M{"inbound": []string{in}, "action": "resolve", "server": "local"})
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
