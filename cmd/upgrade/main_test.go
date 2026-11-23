//go:build windows

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadPlanRequiresExactFilesAndHashes(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "data", "app-updates", "candidate")
	if err := os.MkdirAll(stage, 0700); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(stage, "Mirroxy.exe")
	upgradePath := filepath.Join(stage, "upgrade.exe")
	if err := os.WriteFile(mainPath, []byte("candidate"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(upgradePath, []byte("upgrader"), 0700); err != nil {
		t.Fatal(err)
	}
	p := plan{Root: root, Exe: filepath.Join(root, "Mirroxy.exe"), Candidate: mainPath,
		Backup: filepath.Join(stage, "previous.exe"), Version: "9.9.9", SHA256: digest(mainPath),
		Upgrade: filepath.Join(root, "upgrade.exe"), CandidateUpgrade: upgradePath,
		UpgradeSHA256: digest(upgradePath), Port: 15060, ParentPID: os.Getpid()}
	planPath := filepath.Join(stage, "plan.json")
	b, _ := json.Marshal(p)
	if err := os.WriteFile(planPath, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPlan(planPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte("tampered"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := readPlan(planPath); err == nil {
		t.Fatal("tampered candidate was accepted")
	}
}

func TestReadySupportsHTTPSAndHTTP(t *testing.T) {
	for _, useTLS := range []bool{true, false} {
		name := "http"
		if useTLS {
			name = "https"
		}
		t.Run(name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/status" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"version": "0.9.0",
					"nodes": []map[string]any{
						{"enabled": true, "status": "running"},
						{"enabled": false, "status": "stopped"},
					},
				})
			})
			var srv *httptest.Server
			if useTLS {
				srv = httptest.NewTLSServer(handler)
			} else {
				srv = httptest.NewServer(handler)
			}
			defer srv.Close()
			port := srv.Listener.Addr().(*net.TCPAddr).Port
			p := &plan{Version: "0.9.0", Port: port}
			if !ready(p, os.Getpid(), 5*time.Second) {
				t.Fatalf("ready() failed on %s panel (url=%s)", name, srv.URL)
			}
		})
	}
}
