package main

import (
	"io/fs"
	"mirroxy/web"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBridgeAllowlistAndToken(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/state" {
			w.Write([]byte(`{"token":"test-only"}`))
			return
		}
		if r.URL.Path != "/api/downloads/settings" || r.Method != "POST" || r.Header.Get("X-App-Token") != "test-only" {
			t.Error("unexpected request", r.URL.Path)
			w.WriteHeader(403)
			return
		}
		writes++
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	b := newBackend(1)
	b.base = server.URL
	if _, err := b.request("downloads/settings", `{"connections":4}`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"shutdown", "../state", "downloads/../../shutdown", "http://example.com", "downloads/connector/add"} {
		if _, err := b.request(path, `{}`); err == nil {
			t.Fatal("accepted", path)
		}
	}
	if writes != 1 {
		t.Fatal(writes)
	}
}
func TestBridgeRefusesRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "http://example.invalid", 302) }))
	defer server.Close()
	b := newBackend(1)
	b.base = server.URL
	if _, err := b.request("state", ""); err == nil {
		t.Fatal("redirect accepted")
	}
}
func TestEmbeddedDesktopPage(t *testing.T) {
	data, err := fs.ReadFile(desktopAssets{web.Files}, "index.html")
	if err != nil || !strings.Contains(string(data), `id="dmTasks"`) || !strings.Contains(string(data), `data-pick-dir="dmDir"`) {
		t.Fatal("desktop page not embedded", err)
	}
}
