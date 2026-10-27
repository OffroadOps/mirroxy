package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Set MIRROXY_TEST_ARIA2 to an official aria2 executable to exercise the full
// extension -> connector -> aria2 path in an isolated application root.
func TestBrowserConnectorRelayIntegration(t *testing.T) {
	binary := os.Getenv("MIRROXY_TEST_ARIA2")
	if binary == "" {
		t.Skip("set MIRROXY_TEST_ARIA2 for native browser relay integration")
	}
	a := &App{root: t.TempDir(), panelPort: 9950, panelTLS: true, token: "panel-token", cores: map[string]*Core{}, shutdown: make(chan struct{}), cfg: Settings{BrowserDownloadToken: "connector-token", Updated: map[string]string{}}}
	if err := os.MkdirAll(filepath.Join(a.root, "bin", "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(a.root, "bin", "runtime", executableName("Mirroxy_downloader")), payload, 0755); err != nil {
		t.Fatal(err)
	}
	if err = a.save(); err != nil {
		t.Fatal(err)
	}
	d, err := a.downloads()
	if err != nil {
		t.Fatal(err)
	}
	d.journal.DefaultDir = filepath.Join(a.root, "downloads")
	if err = d.save(); err != nil {
		t.Fatal(err)
	}
	defer a.stopDownloads()
	content := bytes.Repeat([]byte("browser relay integration\n"), 200000)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://browser.example/page" || !strings.Contains(r.Header.Get("Cookie"), "session=relay-cookie") {
			http.Error(w, "missing browser context", http.StatusForbidden)
			return
		}
		http.ServeContent(w, r, "sample.bin", time.Unix(1700000000, 0), bytes.NewReader(content))
	}))
	defer origin.Close()
	body, _ := json.Marshal(M{"url": origin.URL + "/sample.bin", "name": "sample.bin", "headers": M{"referer": "https://browser.example/page", "userAgent": "Relay Test UA", "cookie": "session=relay-cookie"}})
	r := httptest.NewRequest("POST", "http://127.0.0.1:9950/api/downloads/connector/add", bytes.NewReader(body))
	r.RemoteAddr = "127.0.0.1:40000"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "chrome-extension://abcdefghijklmnop")
	r.Header.Set("X-Mirroxy-Download-Token", "connector-token")
	rec := httptest.NewRecorder()
	a.handler(false).ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Fatalf("connector add failed: %d %s", rec.Code, rec.Body.String())
	}
	var added struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &added); err != nil || added.ID == "" {
		t.Fatalf("connector add response invalid: %s", rec.Body.String())
	}
	var task *DownloadTask
	for i := 0; i < 100; i++ {
		snapshot, _ := d.snapshot()
		for _, candidate := range snapshot["tasks"].([]*DownloadTask) {
			if candidate.ID == added.ID {
				task = candidate
			}
		}
		if task != nil && task.Status == "complete" {
			break
		}
		if task != nil && task.Status == "error" {
			t.Fatalf("relayed download failed: %s", task.Error)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if task == nil || task.Status != "complete" {
		t.Fatal("relayed download did not complete")
	}
	got, err := os.ReadFile(task.Path)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("relayed file mismatch: %v", err)
	}
	var stored *DownloadTask
	for _, candidate := range d.journal.Tasks {
		if candidate.ID == added.ID {
			stored = candidate
		}
	}
	if stored == nil || stored.Cookie != "session=relay-cookie" || stored.UserAgent != "Relay Test UA" {
		t.Fatal("browser headers were not stored for the engine")
	}
}
