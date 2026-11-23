package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

const clashFixture = `port: 7890
proxies:
  - name: vmess-ws
    type: vmess
    server: vm.example.invalid
    port: 443
    uuid: 11111111-2222-3333-4444-555555555555
    alterId: 0
    cipher: auto
    tls: true
    servername: cdn.example.invalid
    network: ws
    ws-opts:
      path: /socket
      headers:
        Host: cdn.example.invalid
  - name: vless-reality
    type: vless
    server: vl.example.invalid
    port: 443
    uuid: 11111111-2222-3333-4444-555555555555
    flow: xtls-rprx-vision
    network: tcp
    client-fingerprint: chrome
    reality-opts:
      public-key: reality-public-key
      short-id: ab12
    servername: www.example.invalid
  - name: ss-aes
    type: ss
    server: ss.example.invalid
    port: 8388
    cipher: aes-128-gcm
    password: ss-password
  - name: hy2
    type: hysteria2
    server: hy.example.invalid
    port: 443
    password: hy-password
    sni: hy.example.invalid
    skip-cert-verify: true
    obfs: salamander
    obfs-password: obfs-password
  - name: tuic
    type: tuic
    server: tuic.example.invalid
    port: 443
    uuid: 11111111-2222-3333-4444-555555555555
    password: tuic-password
    congestion-controller: bbr
    udp-relay-mode: native
  - name: anytls
    type: anytls
    server: any.example.invalid
    port: 8443
    password: any-password
  - name: httpupgrade
    type: vless
    server: hu.example.invalid
    port: 443
    uuid: 11111111-2222-3333-4444-555555555555
    network: httpupgrade
    tls: true
    servername: hu.example.invalid
    http-opts:
      path:
        - /upgrade
      host:
        - hu.example.invalid
  - name: unsupported-plugin
    type: ss
    server: ss.example.invalid
    port: 8388
    cipher: aes-128-gcm
    password: ss-password
    plugin: v2ray-plugin
proxy-groups:
  - name: auto
    type: url-test
    proxies: [vmess-ws, vless-reality]
`

func TestParseClashSubscription(t *testing.T) {
	nodes, warnings := parseLinks(clashFixture)
	if len(nodes) != 7 {
		t.Fatalf("nodes = %d, warnings = %v", len(nodes), warnings)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "插件") {
		t.Fatalf("plugin warning missing: %v", warnings)
	}
	byName := map[string]Node{}
	for _, n := range nodes {
		byName[n.Name] = n
	}
	if n, ok := byName["vmess-ws"]; !ok || n.Protocol != "vmess" || n.Engine != "xray" {
		t.Fatalf("vmess node missing: %+v", n)
	} else {
		out, _ := json.Marshal(n.Outbound)
		if !strings.Contains(string(out), `"network":"ws"`) || !strings.Contains(string(out), `/socket`) || !strings.Contains(string(out), `cdn.example.invalid`) {
			t.Fatalf("vmess stream not mapped: %s", out)
		}
	}
	if n, ok := byName["vless-reality"]; !ok || n.Protocol != "vless" {
		t.Fatalf("vless node missing")
	} else {
		out, _ := json.Marshal(n.Outbound)
		if !strings.Contains(string(out), `"security":"reality"`) || !strings.Contains(string(out), `reality-public-key`) || !strings.Contains(string(out), `"flow":"xtls-rprx-vision"`) {
			t.Fatalf("reality settings not mapped: %s", out)
		}
	}
	if n, ok := byName["ss-aes"]; !ok || n.Protocol != "shadowsocks" {
		t.Fatalf("ss node missing")
	} else {
		out, _ := json.Marshal(n.Outbound)
		if !strings.Contains(string(out), `"method":"aes-128-gcm"`) || !strings.Contains(string(out), `ss-password`) {
			t.Fatalf("ss settings not mapped: %s", out)
		}
	}
	for _, name := range []string{"hy2", "tuic", "anytls"} {
		if n, ok := byName[name]; !ok || n.Engine != "sing-box" {
			t.Fatalf("%s node missing or wrong engine: %+v", name, n)
		}
	}
	if n := byName["hy2"]; stringVal(yamlMap(n.Outbound["obfs"])["password"]) != "obfs-password" {
		t.Fatalf("hysteria2 obfs not mapped: %+v", n.Outbound)
	}
	if n := byName["tuic"]; n.Outbound["congestion_control"] != "bbr" || n.Outbound["udp_relay_mode"] != "native" {
		t.Fatalf("tuic options not mapped: %+v", n.Outbound)
	}
	if n, ok := byName["httpupgrade"]; !ok {
		t.Fatal("httpupgrade node missing")
	} else {
		out, _ := json.Marshal(n.Outbound)
		if !strings.Contains(string(out), `"httpupgradeSettings"`) || !strings.Contains(string(out), `/upgrade`) || !strings.Contains(string(out), `hu.example.invalid`) {
			t.Fatalf("httpupgrade options not mapped: %s", out)
		}
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(clashFixture))
	nodes, _ = parseLinks(encoded)
	if len(nodes) != 7 {
		t.Fatalf("base64-wrapped Clash subscription parsed %d nodes", len(nodes))
	}

	quoted := "\"proxies\":\n  - name: quoted\n    type: ss\n    server: quoted.example.invalid\n    port: 8388\n    cipher: aes-128-gcm\n    password: quoted-password\n"
	if nodes, _ := parseLinks(quoted); len(nodes) != 1 {
		t.Fatalf("quoted proxies key parsed %d nodes", len(nodes))
	}
	jsonDoc := `{"proxies":[{"name":"json","type":"ss","server":"json.example.invalid","port":8388,"cipher":"aes-128-gcm","password":"json-password"}]}`
	if nodes, _ := parseLinks(jsonDoc); len(nodes) != 1 {
		t.Fatalf("JSON proxies document parsed %d nodes", len(nodes))
	}
}
