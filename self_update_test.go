package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAppUpdateArchiveValidation(t *testing.T) {
	binary := executableName("Mirroxy")
	for _, tc := range []struct {
		name, extra, platform string
		badHash               bool
	}{
		{name: "valid", platform: runtime.GOOS + "-" + runtime.GOARCH},
		{name: "traversal", extra: "../outside"}, {name: "data overwrite", extra: "data/settings.json"},
		{name: "wrong platform", platform: "invalid-arm64"}, {name: "tampered", badHash: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string][]byte{binary: []byte("candidate")}
			if runtime.GOOS == "windows" {
				files[executableName("upgrade")] = []byte("upgrader")
			}
			if tc.extra != "" {
				files[tc.extra] = []byte("bad")
			}
			hashes := map[string]string{}
			for n, b := range files {
				hashes[n] = digestBytes(b)
			}
			if tc.badHash {
				hashes[binary] = "wrong"
			}
			platform := tc.platform
			if platform == "" {
				platform = runtime.GOOS + "-" + runtime.GOARCH
			}
			files["manifest.json"], _ = json.Marshal(M{"application": "Mirroxy", "kind": "app-update", "version": "0.4.3", "platform": platform, "files": hashes})
			var buf bytes.Buffer
			w := zip.NewWriter(&buf)
			for n, b := range files {
				f, _ := w.Create(n)
				f.Write(b)
			}
			w.Close()
			root := t.TempDir()
			path, err := extractAppUpdate(buf.Bytes(), filepath.Join(root, "stage"), "0.4.3")
			if tc.name == "valid" {
				if err != nil || string(mustReadFile(path)) != "candidate" {
					t.Fatal(path, err)
				}
			} else if err == nil {
				t.Fatal("unsafe package accepted")
			}
		})
	}
}

func TestAppUpdateFeedTrustAndVersion(t *testing.T) {
	var feed any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(feed) }))
	defer srv.Close()
	t.Setenv("MIRROXY_RELEASE_BASE", srv.URL+"/")
	a := &App{cores: map[string]*Core{}}
	feed = M{"latest": "0.0.1"}
	offer, err := a.findAppUpdate()
	if err != nil || offer.Version != version {
		t.Fatal("downgrade accepted", err)
	}
	feed = M{"latest": "99.0.0", "releases": []any{M{"version": "99.0.0", "files": []any{M{"name": appUpdateName(), "url": "https://elsewhere.invalid/package.zip", "size": 10, "sha256": digestBytes([]byte("test"))}}}}}
	if _, err = a.findAppUpdate(); err == nil {
		t.Fatal("foreign URL accepted")
	}
	name := appUpdateName()
	feed = M{"latest": "99.0.0", "releases": []any{M{"version": "99.0.0", "files": []any{M{"name": name, "url": srv.URL + "/v99.0.0/" + name, "size": 10, "sha256": digestBytes([]byte("test"))}}}}}
	state, err := a.checkAppUpdate()
	if err != nil || !state.Available || state.Latest != "99.0.0" || a.busyUpdate {
		t.Fatal("checking for an optional update started or hid the update", state, err)
	}
	t.Setenv("MIRROXY_RELEASE_BASE", "http://example.com/")
	if _, err = releaseBase(); err == nil {
		t.Fatal("non-local plaintext release source accepted")
	}
}

func TestUpdateRollbackRestoresData(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "data", "app-updates", "test")
	os.MkdirAll(stage, 0700)
	p := &appUpdatePlan{Root: root, Exe: filepath.Join(root, executableName("Mirroxy")), Candidate: filepath.Join(stage, executableName("Mirroxy")), Backup: filepath.Join(stage, "previous"+filepath.Ext(executableName("Mirroxy")))}
	os.WriteFile(p.Exe, []byte("old"), 0700)
	os.WriteFile(p.Candidate, []byte("new"), 0700)
	for _, n := range []string{"Mirroxy.db", "Mirroxy.key"} {
		os.WriteFile(filepath.Join(root, "data", n), []byte("before"), 0600)
	}
	if err := backupUpdateData(p); err != nil {
		t.Fatal(err)
	}
	if err := commitUpdate(p); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "data", "Mirroxy.db"), []byte("after"), 0600)
	if err := rollbackUpdate(p); err != nil {
		t.Fatal(err)
	}
	if string(mustReadFile(p.Exe)) != "old" || string(mustReadFile(filepath.Join(root, "data", "Mirroxy.db"))) != "before" {
		t.Fatal("rollback lost application or data")
	}
}
