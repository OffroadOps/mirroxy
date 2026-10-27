package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func connectorTestApp(t *testing.T) *App {
	t.Helper()
	a := &App{root: t.TempDir(), panelPort: 9950, token: "panel-token", panelTLS: true, cores: map[string]*Core{}, cfg: Settings{BrowserDownloadToken: "connector-token"}}
	return a
}

func connectorRequest(a *App, method, path, origin, token, body string, remote bool) *http.Request {
	r := httptest.NewRequest(method, "http://127.0.0.1:9950"+path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:43210"
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if token != "" {
		r.Header.Set("X-Mirroxy-Download-Token", token)
	}
	return r
}

func TestBrowserConnectorAcceptExtensionOriginOnly(t *testing.T) {
	a := connectorTestApp(t)
	cases := []struct {
		name, method, origin, token string
		want                        int
	}{
		{"ping", "POST", "chrome-extension://abcdefghijklmnop", "connector-token", 200},
		{"edge origin uses chrome scheme", "POST", "chrome-extension://edgeid", "connector-token", 200},
		{"firefox origin", "POST", "moz-extension://uuid", "connector-token", 200},
		{"wrong token", "POST", "chrome-extension://abcdefghijklmnop", "wrong", 400},
		{"missing token", "POST", "chrome-extension://abcdefghijklmnop", "", 400},
		{"web page origin rejected", "POST", "https://evil.invalid", "connector-token", 403},
		{"no origin rejected", "POST", "", "connector-token", 403},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		a.handler(false).ServeHTTP(rec, connectorRequest(a, tc.method, "/api/downloads/connector/ping", tc.origin, tc.token, `{}`, false))
		if rec.Code != tc.want {
			t.Fatalf("%s: got %d, want %d: %s", tc.name, rec.Code, tc.want, rec.Body.String())
		}
		if tc.want == 200 && rec.Header().Get("Access-Control-Allow-Origin") != tc.origin {
			t.Fatalf("%s: missing CORS allow origin", tc.name)
		}
	}
}

func TestBrowserConnectorPreflight(t *testing.T) {
	a := connectorTestApp(t)
	r := connectorRequest(a, "OPTIONS", "/api/downloads/connector/add", "chrome-extension://abcdefghijklmnop", "", "", false)
	r.Header.Set("Access-Control-Request-Method", "POST")
	r.Header.Set("Access-Control-Request-Headers", "content-type,x-mirroxy-download-token")
	r.Header.Set("Access-Control-Request-Private-Network", "true")
	rec := httptest.NewRecorder()
	a.handler(false).ServeHTTP(rec, r)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Private-Network") != "true" {
		t.Fatalf("preflight failed: %d %v", rec.Code, rec.Header())
	}
	if !strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), "X-Mirroxy-Download-Token") {
		t.Fatal("preflight does not allow the token header")
	}
}

func TestBrowserConnectorRejectsRemoteAndBadPayload(t *testing.T) {
	a := connectorTestApp(t)
	r := connectorRequest(a, "POST", "/api/downloads/connector/add", "chrome-extension://abcdefghijklmnop", "connector-token", `{"url":"file:///etc/passwd"}`, true)
	rec := httptest.NewRecorder()
	a.handler(true).ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatalf("remote connector request was accepted: %d", rec.Code)
	}
	// A valid token still goes through download validation.
	r = connectorRequest(a, "POST", "/api/downloads/connector/add", "chrome-extension://abcdefghijklmnop", "connector-token", `{"url":"file:///etc/passwd"}`, false)
	rec = httptest.NewRecorder()
	a.handler(false).ServeHTTP(rec, r)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "HTTP") {
		t.Fatalf("invalid URL was not rejected by download validation: %d %s", rec.Code, rec.Body.String())
	}
}

func TestBrowserConnectorInfoAndExtensionZip(t *testing.T) {
	a := connectorTestApp(t)
	rec := httptest.NewRecorder()
	a.handler(false).ServeHTTP(rec, connectorRequest(a, "GET", "/api/downloads/connector", "http://127.0.0.1:9950", "", "", false))
	if rec.Code != 200 {
		t.Fatalf("info failed: %d %s", rec.Code, rec.Body.String())
	}
	var info M
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil || info["token"] != "connector-token" || info["url"] != "https://127.0.0.1:9950" {
		t.Fatalf("connector info wrong: %v %v", info, err)
	}
	rec = httptest.NewRecorder()
	a.handler(false).ServeHTTP(rec, connectorRequest(a, "GET", "/api/downloads/connector/extension.zip", "http://127.0.0.1:9950", "", "", false))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/zip" || !bytes.HasPrefix(rec.Body.Bytes(), []byte("PK")) {
		t.Fatalf("extension zip failed: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	archive, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, file := range archive.File {
		names[file.Name] = true
	}
	for _, want := range []string{"manifest.json", "background.js", "options.html", "popup.html", "README.md", "icons/icon128.png"} {
		if !names[want] {
			t.Fatalf("extension zip missing %s: %v", want, names)
		}
	}
	for _, file := range archive.File {
		if file.Name != "manifest.json" {
			continue
		}
		reader, _ := file.Open()
		data, _ := io.ReadAll(reader)
		reader.Close()
		var manifest M
		if json.Unmarshal(data, &manifest) != nil || manifest["manifest_version"] != float64(3) {
			t.Fatalf("invalid manifest: %s", data)
		}
		if manifest["name"] != "Mirroxy 下载接力 / Download Relay" {
			t.Fatalf("unexpected manifest name: %v", manifest["name"])
		}
	}
}

func TestBrowserDownloadTokenNeverAppearsInState(t *testing.T) {
	a := connectorTestApp(t)
	b, err := json.Marshal(a.state())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "connector-token") || strings.Contains(string(b), "browserDownloadToken") {
		t.Fatal("public panel state leaked the browser connector token")
	}
}

func TestFreshInstallBrowserTokenPersists(t *testing.T) {
	a := &App{root: t.TempDir()}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}
	if len(a.cfg.BrowserDownloadToken) < 32 {
		t.Fatalf("fresh install did not create a connector token (len %d)", len(a.cfg.BrowserDownloadToken))
	}
	reloaded := &App{root: a.root}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if reloaded.cfg.BrowserDownloadToken != a.cfg.BrowserDownloadToken {
		t.Fatal("connector token was regenerated on reload")
	}
}
