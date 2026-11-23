package main

// xrayDNSConfig splits DNS through encrypted resolvers: mainland and private
// domains use AliDNS DoH directly, everything else uses Cloudflare DoH routed
// through the active node (the dns-in rules in coreConfig send it there).
// localhost remains the last fallback so a blocked DoH endpoint does not take
// name resolution down completely.
func xrayDNSConfig() M {
	return M{
		"tag":           "dns-in",
		"queryStrategy": "UseIP",
		"servers": []any{
			M{"address": "https://223.5.5.5/dns-query", "domains": []string{xDomain("cn"), xDomain("private")}, "expectedIPs": []string{xIP("cn"), xIP("private")}, "skipFallback": true},
			M{"address": "https://1.1.1.1/dns-query"},
			"localhost",
		},
	}
}

// singDNSConfig builds the per-node DNS layout. Mainland and private domains
// are resolved through AliDNS over a direct connection; everything that should
// exit through the node uses Cloudflare DoH with the node as its detour. The
// system server is only used to bootstrap node server hostnames, so a DoH
// outage cannot prevent the outbound itself from connecting.
func (a *App) singDNSConfig(tag, mode string, sets map[string]bool) M {
	sets["geosite-private"] = true
	rules := []any{}
	final := "remote"
	switch mode {
	case "bypass-cn":
		sets["geosite-cn"] = true
		rules = append(rules, M{"action": "route", "server": "local", "rule_set": []string{"geosite-private", "geosite-cn"}})
	case "blacklist":
		for _, name := range blacklistSiteSets {
			sets[name] = true
		}
		rules = append(rules, M{"action": "route", "server": "local", "rule_set": []string{"geosite-private"}})
		rules = append(rules, M{"action": "route", "server": "remote", "rule_set": blacklistSiteSets})
		rules = append(rules, M{"action": "route", "server": "remote", "domain_suffix": proxyDNSDomains})
		final = "local"
	default:
		rules = append(rules, M{"action": "route", "server": "local", "rule_set": []string{"geosite-private"}})
	}
	return M{
		"servers": []any{
			M{"type": "https", "tag": "local", "server": "223.5.5.5", "detour": "direct"},
			M{"type": "https", "tag": "remote", "server": "1.1.1.1", "detour": tag},
			M{"type": "local", "tag": "system"},
		},
		"rules": rules,
		"final": final,
	}
}
