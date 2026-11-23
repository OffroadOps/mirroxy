package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The downloads page is embedded in the local dashboard and can also be opened
// in its own window, so it must be served with the running version in its asset
// links, like the main panel.
func TestDownloadWindowPageIsServed(t *testing.T) {
	a := &App{root: t.TempDir(), panelPort: 9950, token: "ui-token", cores: map[string]*Core{}}
	for _, path := range []string{"/downloads", "/downloads.html"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:9950"+path, nil)
		r.RemoteAddr = "127.0.0.1:43210"
		rec := httptest.NewRecorder()
		a.handler(false).ServeHTTP(rec, r)
		body := rec.Body.String()
		if rec.Code != 200 || !strings.Contains(body, `id="dmTasks"`) {
			t.Fatalf("%s: download window not served: %d", path, rec.Code)
		}
		if strings.Contains(body, "__VERSION__") || !strings.Contains(body, "dl-app.js?v="+version) {
			t.Fatalf("%s: version placeholder not substituted", path)
		}
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:9950/dl-app.js", nil)
	r.RemoteAddr = "127.0.0.1:43210"
	rec := httptest.NewRecorder()
	a.handler(false).ServeHTTP(rec, r)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "MirroxyDownloadModel") {
		t.Fatalf("download window script not served: %d", rec.Code)
	}
}

func TestPanelFramingPolicy(t *testing.T) {
	a := &App{root: t.TempDir(), panelPort: 9950, token: "ui-token", cores: map[string]*Core{}}
	local := httptest.NewRequest("GET", "http://127.0.0.1:9950/downloads", nil)
	local.RemoteAddr = "127.0.0.1:43210"
	rec := httptest.NewRecorder()
	a.handler(false).ServeHTTP(rec, local)
	if got := rec.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Fatalf("local panel X-Frame-Options = %q, want SAMEORIGIN", got)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Fatalf("local panel CSP = %q, want frame-ancestors 'self'", csp)
	}
	remote := httptest.NewRequest("GET", "http://192.0.2.10:15061/", nil)
	remote.RemoteAddr = "192.0.2.10:43210"
	rec = httptest.NewRecorder()
	a.handler(true).ServeHTTP(rec, remote)
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("remote panel X-Frame-Options = %q, want DENY", got)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("remote panel CSP = %q, want frame-ancestors 'none'", csp)
	}
}
