package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The tray opens /downloads in its own window, so the page must be served with
// the running version in its asset links, like the main panel.
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
