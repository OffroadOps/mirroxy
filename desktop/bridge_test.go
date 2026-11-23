package main

import (
	"io"
	"io/fs"
	"mirroxy/web"
	"net"
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
	for _, view := range []string{"downloads", "panel"} {
		data, err := fs.ReadFile(desktopAssets{FS: web.Files, view: view}, "index.html")
		if err != nil {
			t.Fatalf("%s index.html failed: %v", view, err)
		}
		if view == "downloads" && (!strings.Contains(string(data), `id="dmTasks"`) || !strings.Contains(string(data), `data-pick-dir="dmDir"`)) {
			t.Fatal("desktop downloads page not embedded")
		}
		if view == "panel" && !strings.Contains(string(data), `id="view-nodes"`) {
			t.Fatal("desktop panel page not embedded")
		}
	}
	if _, err := fs.ReadFile(desktopAssets{FS: web.Files, view: "panel"}, "changelog.json"); err != nil {
		t.Fatalf("changelog.json not embedded: %v", err)
	}
}

func TestBridgeSupportsTLSAndAPIProxyStripsCrossOriginHeaders(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" || r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
			t.Errorf("cross-origin/forwarded headers leaked: %v", r.Header)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/state":
			_, _ = w.Write([]byte(`{"token":"tls-token","version":"0.9.0"}`))
		case "/api/downloads/settings":
			if r.Header.Get("X-App-Token") != "tls-token" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			body, _ := io.ReadAll(r.Body)
			_, _ = w.Write(body)
		case "/api/node/qr":
			if r.URL.RawQuery != "id=n1&theme=dark" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("PNG"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port
	b := newBackend(port)
	if out, err := b.request("downloads/settings", `{"connections":8}`); err != nil || !strings.Contains(out, `"connections":8`) {
		t.Fatalf("TLS request failed: %q %v", out, err)
	}

	handler := b.apiHandler()
	req := httptest.NewRequest(http.MethodGet, "http://wails.localhost/api/node/qr?id=n1&theme=dark", nil)
	req.Header.Set("Origin", "http://wails.localhost")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("X-Forwarded-For", "192.0.2.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "PNG" || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("apiHandler GET proxy failed: %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://wails.localhost/api/downloads/settings", strings.NewReader(`{"connections":4}`))
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("X-App-Token", "tls-token")
	postReq.Header.Set("Origin", "http://wails.localhost")
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK || postRec.Body.String() != `{"connections":4}` {
		t.Fatalf("apiHandler POST proxy failed: %d %q", postRec.Code, postRec.Body.String())
	}

	badReq := httptest.NewRequest(http.MethodGet, "http://wails.localhost/etc/passwd", nil)
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusNotFound {
		t.Fatalf("non-api path was proxied: %d", badRec.Code)
	}
}
