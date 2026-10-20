package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestShareRoundTripPersistedNodes(t *testing.T) {
	var cases []Spec
	for _, proto := range []string{"vless", "vmess", "trojan", "http", "socks", "shadowsocks", "hysteria2", "tuic", "anytls"} {
		s := Spec{Name: "分享 / 中文 # + %", Protocol: proto, Server: "2001:db8::12", Port: 24443, Password: "test:@/+?#%password", Username: "test-user", Method: "aes-128-gcm", Extra: M{}}
		if proto == "shadowsocks" {
			cases = append(cases, s)
			continue
		}
		if proto == "hysteria2" {
			s.Extra = M{"obfs": "salamander", "obfs_password": "mixed?#obfs"}
		}
		if proto == "tuic" {
			s.Extra = M{"congestion_control": "bbr", "udp_relay_mode": "native"}
		}
		s.Security = "tls"
		s.SNI = "tls.example.invalid"
		s.ALPN = "h2,http/1.1"
		cases = append(cases, s)
		insecure := s
		insecure.Insecure = true
		cases = append(cases, insecure)
		if proto == "hysteria2" || proto == "tuic" || proto == "anytls" {
			s.Insecure = true
			cases = append(cases, s)
			continue
		}
		for _, network := range []string{"ws", "grpc", "httpupgrade", "xhttp"} {
			v := s
			v.Network = network
			v.Path = "/a?ed=2048"
			v.Host = "cdn.example.invalid"
			v.ServiceName = "grpc/test"
			v.Fingerprint = "chrome"
			cases = append(cases, v)
		}
		if proto == "vless" {
			s.Network = "raw"
			s.Security = "reality"
			s.PublicKey = "test-public-key"
			s.ShortID = "abc123"
			s.Flow = "xtls-rprx-vision"
			cases = append(cases, s)
		}
	}
	for _, s := range cases {
		t.Run(s.Protocol+"/"+s.Network+"/"+s.Security, func(t *testing.T) {
			n, err := makeNode(s)
			if err != nil {
				t.Fatal(err)
			}
			// Model a saved, reloaded old node, whose port is the local listener.
			n.ID = "share-test"
			n.Port = 5066
			b, _ := json.Marshal(n)
			json.Unmarshal(b, &n)
			link, err := shareLink(n)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := parseLink(link)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(n.Outbound)
			after, _ := json.Marshal(restored.Outbound)
			if string(before) != string(after) || n.Name != restored.Name {
				t.Fatal("share changed connection options")
			}
		})
	}
}

func TestShareRejectsLossyExport(t *testing.T) {
	n, _ := parseLink("vless://test-id@example.invalid:443?security=tls#test")
	n.Outbound["futureOption"] = "must-not-drop"
	if uri, err := shareLink(n); err == nil || uri != "" {
		t.Fatal("silently discarded unsupported option")
	}
}

func TestShareAPIRequiresExplicitAuthenticatedRequest(t *testing.T) {
	a := &App{root: t.TempDir(), panelPort: 15060, token: "ui-token", cores: map[string]*Core{}}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}
	n, _ := parseLink("http://alice:PRIVATE_SHARE_PASSWORD@example.invalid:24443#Node")
	n.ID = "export"
	n.Port = 5066
	a.cfg.Nodes = []Node{n}
	a.cfg.Remote = RemoteSettings{Username: "admin", Salt: "test", PasswordHash: passwordHash("strong-test-password", "test")}
	for _, tc := range []struct {
		path, method, origin string
		remote, auth         bool
		want                 int
	}{
		{"/api/v1/nodes/export/share", "POST", "", false, false, 200},
		{"/api/node/share", "POST", "", false, false, 200},
		{"/api/v1/nodes/export/share", "POST", "", true, false, 401},
		{"/api/v1/nodes/export/share", "POST", "", true, true, 200},
		{"/api/v1/nodes/export/share", "POST", "https://evil.invalid", false, false, 403},
		{"/api/v1/nodes/export/share", "GET", "", false, false, 404},
		{"/api/v1/nodes/missing/share", "POST", "", false, false, 400},
	} {
		r := httptest.NewRequest(tc.method, "http://127.0.0.1:15060"+tc.path, strings.NewReader(`{"id":"export"}`))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-App-Token", "ui-token")
		r.Header.Set("Origin", tc.origin)
		if tc.auth {
			r.SetBasicAuth("admin", "strong-test-password")
		}
		w := httptest.NewRecorder()
		a.handler(tc.remote).ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s remote=%v auth=%v: got %d want %d", tc.path, tc.remote, tc.auth, w.Code, tc.want)
		}
		if tc.want == 200 {
			if !strings.Contains(w.Body.String(), "PRIVATE_SHARE_PASSWORD") || !strings.Contains(w.Body.String(), ":24443") {
				t.Fatal("wrong exported endpoint")
			}
			if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
				t.Fatal("export is cacheable")
			}
		}
	}
	for _, path := range []string{"/api/state", "/api/v1/nodes", "/api/v1/status", "/api/traffic"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:15060"+path, nil)
		r.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		a.handler(false).ServeHTTP(w, r)
		if w.Code != 200 || strings.Contains(w.Body.String(), "PRIVATE_SHARE_PASSWORD") {
			t.Fatal("ordinary status exposed share credential: " + path)
		}
	}
}
