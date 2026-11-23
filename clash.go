package main

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// looksLikeClash detects a Clash-compatible YAML or JSON feed by its
// `proxies` key. Nested matches are harmless: parseClashSubscription only
// accepts a top-level proxies list.
func looksLikeClash(text string) bool {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, `"proxies"`) {
		return true
	}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.ToLower(line)
		for _, prefix := range []string{"proxies:", `"proxies":`, `'proxies':`, "proxies :", `"proxies" :`, `'proxies' :`} {
			if strings.HasPrefix(line, prefix) {
				return true
			}
		}
	}
	return false
}

func yamlMap(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
}

func yamlBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	default:
		return false
	}
}

func yamlStrings(value any) []string {
	switch v := value.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := stringVal(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	case string:
		if v != "" {
			return []string{v}
		}
	}
	return nil
}

// parseClashSubscription converts the `proxies` list of a Clash YAML
// subscription into the same Node form used by share links.
func parseClashSubscription(text string) ([]Node, []string, bool) {
	var root map[string]any
	if yaml.Unmarshal([]byte(text), &root) != nil {
		return nil, nil, false
	}
	raw, exists := root["proxies"]
	if !exists {
		return nil, nil, false
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, []string{"Clash 订阅的 proxies 字段格式不正确"}, true
	}
	var nodes []Node
	var warnings []string
	for i, item := range items {
		proxy, ok := item.(map[string]any)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("第 %d 个 Clash 节点格式不正确", i+1))
			continue
		}
		name := first(stringVal(proxy["name"]), fmt.Sprintf("第 %d 个节点", i+1))
		node, err := clashNode(proxy)
		if err != nil {
			warnings = append(warnings, name+"："+err.Error())
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes, warnings, true
}

func clashNode(proxy map[string]any) (Node, error) {
	protocol := strings.ToLower(stringVal(proxy["type"]))
	s := Spec{
		Name:        stringVal(proxy["name"]),
		Server:      stringVal(proxy["server"]),
		Port:        intVal(proxy["port"]),
		SNI:         first(stringVal(proxy["servername"]), stringVal(proxy["sni"]), stringVal(proxy["peer"])),
		Fingerprint: stringVal(proxy["client-fingerprint"]),
		Insecure:    yamlBool(proxy["skip-cert-verify"]),
		ALPN:        strings.Join(yamlStrings(proxy["alpn"]), ","),
		Extra:       M{},
	}
	if network := strings.ToLower(stringVal(proxy["network"])); network != "" {
		s.Network = network
	}
	if ws := yamlMap(proxy["ws-opts"]); ws != nil {
		s.Path = stringVal(ws["path"])
		if headers := yamlMap(ws["headers"]); headers != nil {
			hosts := append(yamlStrings(headers["Host"]), yamlStrings(headers["host"])...)
			if len(hosts) > 0 {
				s.Host = hosts[0]
			}
		}
	}
	if grpc := yamlMap(proxy["grpc-opts"]); grpc != nil {
		s.ServiceName = first(stringVal(grpc["grpc-service-name"]), stringVal(grpc["serviceName"]))
	}
	for _, key := range []string{"http-opts", "h2-opts", "xhttp-opts"} {
		opts := yamlMap(proxy[key])
		if opts == nil {
			continue
		}
		if s.Path == "" {
			if paths := yamlStrings(opts["path"]); len(paths) > 0 {
				s.Path = paths[0]
			}
		}
		if s.Host == "" {
			hosts := append(yamlStrings(opts["host"]), yamlStrings(opts["Host"])...)
			if len(hosts) > 0 {
				s.Host = hosts[0]
			}
		}
	}
	if reality := yamlMap(proxy["reality-opts"]); reality != nil {
		s.Security = "reality"
		s.PublicKey = first(stringVal(reality["public-key"]), stringVal(reality["publicKey"]))
		s.ShortID = first(stringVal(reality["short-id"]), stringVal(reality["shortId"]))
		s.Fingerprint = first(stringVal(reality["fingerprint"]), s.Fingerprint)
	} else if yamlBool(proxy["tls"]) {
		s.Security = "tls"
	}
	switch protocol {
	case "vmess":
		s.Protocol = "vmess"
		s.Password = stringVal(proxy["uuid"])
		s.AlterID = intVal(first(stringVal(proxy["alterId"]), stringVal(proxy["alterid"])))
		s.Method = first(stringVal(proxy["cipher"]), "auto")
	case "vless":
		s.Protocol = "vless"
		s.Password = stringVal(proxy["uuid"])
		s.Flow = stringVal(proxy["flow"])
		s.Encryption = stringVal(proxy["encryption"])
	case "ss", "shadowsocks":
		s.Protocol = "shadowsocks"
		s.Method = stringVal(proxy["cipher"])
		s.Password = stringVal(proxy["password"])
		if plugin := stringVal(proxy["plugin"]); plugin != "" {
			return Node{}, fmt.Errorf("暂不支持 Shadowsocks 插件 %s", plugin)
		}
	case "trojan":
		s.Protocol = "trojan"
		s.Password = stringVal(proxy["password"])
		if s.Security == "" {
			s.Security = "tls"
		}
	case "socks", "socks5":
		s.Protocol = "socks"
		s.Username = stringVal(proxy["username"])
		s.Password = stringVal(proxy["password"])
	case "http", "https":
		s.Protocol = "http"
		s.Username = stringVal(proxy["username"])
		s.Password = stringVal(proxy["password"])
		if protocol == "https" {
			s.Security = "tls"
		}
		if s.Port == 0 {
			if s.Security == "tls" {
				s.Port = 443
			} else {
				s.Port = 80
			}
		}
	case "hysteria2", "hy2":
		s.Protocol = "hysteria2"
		s.Password = stringVal(proxy["password"])
		s.Extra["obfs"] = stringVal(proxy["obfs"])
		s.Extra["obfs_password"] = first(stringVal(proxy["obfs-password"]), stringVal(proxy["obfs_password"]))
	case "tuic":
		s.Protocol = "tuic"
		s.Username = first(stringVal(proxy["uuid"]), stringVal(proxy["username"]))
		s.Password = first(stringVal(proxy["password"]), stringVal(proxy["token"]))
		s.Extra["congestion_control"] = first(stringVal(proxy["congestion-controller"]), stringVal(proxy["congestion_control"]))
		s.Extra["udp_relay_mode"] = first(stringVal(proxy["udp-relay-mode"]), stringVal(proxy["udp_relay_mode"]))
	case "anytls":
		s.Protocol = "anytls"
		s.Password = stringVal(proxy["password"])
	default:
		return Node{}, fmt.Errorf("暂不支持 Clash 节点类型 %s", first(protocol, "未知"))
	}
	return makeNode(s)
}
