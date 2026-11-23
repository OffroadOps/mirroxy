package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestActivityParseXrayAccessLines(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		wantOK  bool
		host    string
		port    string
		network string
		nodeID  string
		action  string
	}{
		{
			name:    "domain proxy arrow",
			line:    "2026/11/23 18:10:00.123456 from 127.0.0.1:54321 accepted tcp:api.github.com:443 [in-n1 -> node-n1]",
			wantOK:  true,
			host:    "api.github.com",
			port:    "443",
			network: "tcp",
			nodeID:  "n1",
			action:  "proxy",
		},
		{
			name:    "ipv6 udp app-in direct double arrow",
			line:    "2026/11/23 18:10:01 from [::1]:51234 accepted udp:[2001:4860:4860::8888]:53 [app-in-n2 >> direct]",
			wantOK:  true,
			host:    "2001:4860:4860::8888",
			port:    "53",
			network: "udp",
			nodeID:  "n2",
			action:  "direct",
		},
		{
			name:    "without from prefix block",
			line:    "2026/11/23 18:10:02.001 127.0.0.1:50000 accepted tcp:ads.example.com:80 [in-n3 -> block]",
			wantOK:  true,
			host:    "ads.example.com",
			port:    "80",
			network: "tcp",
			nodeID:  "n3",
			action:  "block",
		},
		{
			name:   "warning line ignored",
			line:   "2026/11/23 18:10:03 [Warning] core: Xray started",
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseXrayAccessLine(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (%+v)", ok, tc.wantOK, got)
			}
			if !ok {
				return
			}
			if got.Host != tc.host || got.Port != tc.port || got.Network != tc.network || got.NodeID != tc.nodeID || got.Action != tc.action || got.Engine != "A" {
				t.Fatalf("unexpected entry: %+v", got)
			}
		})
	}
}

func TestActivityCoreLogWriterPrivacyAndCap(t *testing.T) {
	a := &App{root: t.TempDir(), cores: map[string]*Core{}, traffic: map[string]*Traffic{}}
	logPath := filepath.Join(a.root, "worker.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	w := newCoreLogWriter(a, f, "n1", "HK Node")

	for i := 0; i < maxActivityEntries+35; i++ {
		_, _ = fmt.Fprintf(w, "2026/11/23 18:10:00.%06d from 127.0.0.1:10000 accepted tcp:host-%d.secret.invalid:443 [in-n1 -> node-n1]\n", i, i)
	}
	_, _ = fmt.Fprint(w, "2026/11/23 18:10:05 [Warning] runtime initialized\r\n")
	_, _ = fmt.Fprint(w, "partial trailing diagnostic without newline")
	w.Flush()
	_ = f.Close()

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(rawLog)
	if strings.Contains(text, "secret.invalid") {
		t.Fatalf("access log domain leaked to disk log: %s", text)
	}
	if !strings.Contains(text, "runtime initialized") || !strings.Contains(text, "partial trailing diagnostic without newline") {
		t.Fatalf("expected non-access warnings preserved on disk, got: %s", text)
	}

	items := a.snapshotActivity()
	if len(items) != maxActivityEntries {
		t.Fatalf("expected %d buffered entries, got %d", maxActivityEntries, len(items))
	}
	// Oldest 35 entries were evicted; newest must remain in RAM with resolved NodeName.
	foundNewest := false
	for _, item := range items {
		if item.Host == "host-234.secret.invalid" && item.NodeName == "HK Node" {
			foundNewest = true
		}
		if item.Host == "host-0.secret.invalid" {
			t.Fatal("evicted oldest entry unexpectedly retained")
		}
	}
	if !foundNewest {
		t.Fatal("newest entry missing from RAM ring buffer")
	}

	a.clearActivity()
	if len(a.snapshotActivity()) != 0 {
		t.Fatal("expected clearActivity to empty ring buffer")
	}
}

func TestActivitySnapshotMergesSingBoxAndAPIAccessControl(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connections" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"connections": []map[string]any{
				{
					"metadata": map[string]any{
						"network":         "tcp",
						"host":            "openai.com",
						"destinationIP":   "104.18.0.1",
						"destinationPort": "443",
					},
					"start":  time.Now().Format(time.RFC3339Nano),
					"chains": []string{"node-sb1"},
					"rule":   "domain_suffix=openai.com",
				},
				{
					"metadata": map[string]any{
						"network":         "udp",
						"host":            "",
						"destinationIP":   "192.168.1.5",
						"destinationPort": "53",
					},
					"start":  time.Now().Add(-time.Second).Format(time.RFC3339Nano),
					"chains": []string{"direct"},
					"rule":   "ip_is_private=true",
				},
			},
		})
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestActivityHelperProcess$")
	cmd.Env = append(os.Environ(), "MIRROXY_ACTIVITY_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start helper process: %v", err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	defer func() {
		_ = cmd.Process.Kill()
		<-done
	}()

	a := &App{
		root:          t.TempDir(),
		metricsSecret: "test-secret",
		metricsPorts:  map[string]int{"sing-box-sb1": port},
		cores:         map[string]*Core{"sing-box-sb1": {cmd: cmd, done: done}},
		traffic:       map[string]*Traffic{},
		cfg: Settings{
			Nodes: []Node{{ID: "sb1", Name: "SG Sing", Engine: "sing-box", Enabled: true}},
		},
	}
	a.recordActivity(ActivityEntry{
		Time:   "12:00:00",
		Host:   "xray.example.com",
		Port:   "443",
		Engine: "A",
		Action: "proxy",
		ts:     time.Now().Add(-5 * time.Second),
	})

	localCtx := context.WithValue(context.Background(), localAccessKey{}, true)
	rec := httptest.NewRecorder()
	a.api(rec, httptest.NewRequest(http.MethodGet, "/api/activity", nil).WithContext(localCtx))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/activity failed: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "openai.com") || !strings.Contains(body, "192.168.1.5") || !strings.Contains(body, "xray.example.com") || !strings.Contains(body, "SG Sing") {
		t.Fatalf("expected merged sing-box and xray activity, got: %s", body)
	}

	// Non-local must be rejected.
	remoteCtx := context.WithValue(context.Background(), localAccessKey{}, false)
	remoteGet := httptest.NewRecorder()
	a.api(remoteGet, httptest.NewRequest(http.MethodGet, "/api/activity", nil).WithContext(remoteCtx))
	if remoteGet.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for remote GET /api/activity, got %d %s", remoteGet.Code, remoteGet.Body.String())
	}
	remoteClear := httptest.NewRecorder()
	a.api(remoteClear, httptest.NewRequest(http.MethodPost, "/api/activity/clear", strings.NewReader("{}")).WithContext(remoteCtx))
	if remoteClear.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for remote POST /api/activity/clear, got %d %s", remoteClear.Code, remoteClear.Body.String())
	}

	// Local clear resets Xray RAM history.
	localClear := httptest.NewRecorder()
	a.api(localClear, httptest.NewRequest(http.MethodPost, "/api/activity/clear", strings.NewReader("{}")).WithContext(localCtx))
	if localClear.Code != http.StatusOK {
		t.Fatalf("local clear failed: %d %s", localClear.Code, localClear.Body.String())
	}
}

func TestActivityHelperProcess(t *testing.T) {
	if os.Getenv("MIRROXY_ACTIVITY_HELPER") != "1" {
		return
	}
	time.Sleep(15 * time.Second)
}
