package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseSubscriptionUserinfo(t *testing.T) {
	header := http.Header{}
	header.Set("Subscription-Userinfo", " upload=1024 ; download=2048.5; total=1048576; expire=1893456000 ")
	upload, download, total, expire := parseSubscriptionUserinfo(header)
	if upload != 1024 || download != 2048 || total != 1048576 || expire != 1893456000 {
		t.Fatalf("parsed %d/%d/%d/%d", upload, download, total, expire)
	}
	if u, d, n, e := parseSubscriptionUserinfo(http.Header{}); u+d+n+e != 0 {
		t.Fatal("empty header should produce zero values")
	}
}

func TestSubscriptionUAFallbackClashAndUserinfo(t *testing.T) {
	var mirroxyUA, v2rayNUA, requireMirroxy int32
	const clash = `proxies:
  - name: Provider Node
    type: ss
    server: ss.example.invalid
    port: 8388
    cipher: aes-128-gcm
    password: provider-password
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent := r.Header.Get("User-Agent")
		wanted := "v2rayN/"
		if atomic.LoadInt32(&requireMirroxy) == 1 {
			wanted = "Mirroxy/"
		}
		if strings.Contains(agent, wanted) {
			if wanted == "v2rayN/" {
				atomic.AddInt32(&v2rayNUA, 1)
			} else {
				atomic.AddInt32(&mirroxyUA, 1)
			}
			w.Header().Set("Subscription-Userinfo", "upload=100; download=250; total=1000; expire=1893456000")
			_, _ = w.Write([]byte(clash))
			return
		}
		_, _ = w.Write([]byte("provider filtered this client"))
	}))
	defer srv.Close()
	a := &App{root: t.TempDir(), panelPort: 19950, token: "ui-token", cores: map[string]*Core{}}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}
	a.cfg.SystemProxyMode = "unchanged"
	a.cfg.Subscriptions = []Subscription{{ID: "sub", Name: "Airport", URL: srv.URL}}
	count, err := a.refreshSubscriptionStored("sub")
	if err != nil || count != 1 {
		t.Fatalf("refresh: count=%d err=%v", count, err)
	}
	if atomic.LoadInt32(&v2rayNUA) == 0 || atomic.LoadInt32(&mirroxyUA) != 0 {
		t.Fatalf("v2rayN primary UA not used: mirroxy=%d v2rayN=%d", mirroxyUA, v2rayNUA)
	}
	atomic.StoreInt32(&mirroxyUA, 0)
	atomic.StoreInt32(&v2rayNUA, 0)
	atomic.StoreInt32(&requireMirroxy, 1)
	if count, err = a.refreshSubscriptionStored("sub"); err != nil || count != 1 {
		t.Fatalf("fallback refresh: count=%d err=%v", count, err)
	}
	if atomic.LoadInt32(&mirroxyUA) == 0 {
		t.Fatalf("Mirroxy fallback UA not used: mirroxy=%d v2rayN=%d", mirroxyUA, v2rayNUA)
	}
	sub := a.cfg.Subscriptions[0]
	if sub.Upload != 100 || sub.Download != 250 || sub.Total != 1000 || sub.Expire != 1893456000 {
		t.Fatalf("userinfo not stored: %+v", sub)
	}
	if sub.Error != "" || sub.Updated == "" {
		t.Fatalf("subscription state not updated: %+v", sub)
	}
	reloaded := &App{root: a.root, panelPort: 19950, token: "ui-token", cores: map[string]*Core{}}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if got := reloaded.cfg.Subscriptions[0]; got.Total != 1000 || got.Expire != 1893456000 {
		t.Fatalf("userinfo not persisted: %+v", got)
	}
}
