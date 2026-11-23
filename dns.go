package main

// xrayDNSConfig splits DNS through encrypted resolvers: mainland and private
// domains use AliDNS DoH directly, everything else uses Cloudflare DoH routed
// through the active node (the dns-in rules in coreConfig send it there).
// localhost remains the last fallback so a blocked DoH endpoint does not take
// name resolution down completely. User rules are injected first: blocked
// domains fail immediately, direct rules use the domestic resolver and proxy
// rules use the node resolver.
func xrayDNSConfig(custom CustomRoutingRules) M {
	block := compileCustomBucket(custom.Block)
	direct := compileCustomBucket(custom.Direct)
	proxy := compileCustomBucket(custom.Proxy)
	servers := []any{}
	if len(direct.xrayDomains) > 0 {
		servers = append(servers, M{"address": "https://223.5.5.5/dns-query", "domains": direct.xrayDomains, "skipFallback": true})
	}
	if len(proxy.xrayDomains) > 0 {
		servers = append(servers, M{"address": "https://1.1.1.1/dns-query", "domains": proxy.xrayDomains, "skipFallback": true})
	}
	servers = append(servers,
		M{"address": "https://223.5.5.5/dns-query", "domains": []string{xDomain("cn"), xDomain("private")}, "expectedIPs": []string{xIP("cn"), xIP("private")}, "skipFallback": true},
		M{"address": "https://1.1.1.1/dns-query"},
		"localhost",
	)
	cfg := M{"tag": "dns-in", "queryStrategy": "UseIP", "servers": servers}
	if len(block.xrayDomains) > 0 {
		hosts := M{}
		for _, domain := range block.xrayDomains {
			hosts[domain] = "#3"
		}
		cfg["hosts"] = hosts
	}
	return cfg
}

// singDNSConfig builds the per-node DNS layout. Mainland and private domains
// are resolved through AliDNS over a direct connection; everything that should
// exit through the node uses Cloudflare DoH with the node as its detour. The
// system server is only used to bootstrap node server hostnames, so a DoH
// outage cannot prevent the outbound itself from connecting. Remote DoH is
// only attached to the node when the node is an actual proxy; a synthetic
// direct outbound would be rejected by sing-box's DNS transport.
func (a *App) singDNSConfig(tag, mode string, sets map[string]bool, custom CustomRoutingRules, remoteDetour bool) M {
	block := compileCustomBucket(custom.Block)
	direct := compileCustomBucket(custom.Direct)
	proxy := compileCustomBucket(custom.Proxy)
	rules := []any{}
	add := func(action, server string, fields M) {
		fields["action"] = action
		if server != "" {
			fields["server"] = server
		}
		rules = append(rules, fields)
	}
	addBucket := func(bucket compiledCustomBucket, action, server string) {
		if len(bucket.singSets) > 0 {
			for _, name := range bucket.singSets {
				sets[name] = true
			}
			add(action, server, M{"rule_set": bucket.singSets})
		}
		if len(bucket.singDomain) > 0 {
			add(action, server, M{"domain": bucket.singDomain})
		}
		if len(bucket.singSuffix) > 0 {
			add(action, server, M{"domain_suffix": bucket.singSuffix})
		}
		if len(bucket.singKeyword) > 0 {
			add(action, server, M{"domain_keyword": bucket.singKeyword})
		}
		if len(bucket.singRegex) > 0 {
			add(action, server, M{"domain_regex": bucket.singRegex})
		}
	}
	// Match the routing priority: block > direct > proxy > built-in rules.
	addBucket(block, "reject", "")
	addBucket(direct, "route", "local")
	addBucket(proxy, "route", "remote")

	sets["geosite-private"] = true
	final := "remote"
	switch mode {
	case "bypass-cn":
		sets["geosite-cn"] = true
		add("route", "local", M{"rule_set": []string{"geosite-private", "geosite-cn"}})
	case "blacklist":
		for _, name := range blacklistSiteSets {
			sets[name] = true
		}
		add("route", "local", M{"rule_set": []string{"geosite-private"}})
		add("route", "remote", M{"rule_set": blacklistSiteSets})
		add("route", "remote", M{"domain_suffix": proxyDNSDomains})
		final = "local"
	default:
		add("route", "local", M{"rule_set": []string{"geosite-private"}})
	}
	remote := M{"type": "https", "tag": "remote", "server": "1.1.1.1"}
	if remoteDetour {
		remote["detour"] = tag
	}
	return M{
		"servers": []any{
			M{"type": "https", "tag": "local", "server": "223.5.5.5"},
			remote,
			M{"type": "local", "tag": "system"},
		},
		"rules": rules,
		"final": final,
	}
}
