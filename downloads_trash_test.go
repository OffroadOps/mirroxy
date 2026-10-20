package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadDeleteAPIFlagDefaultsOff(t *testing.T) {
	for _, optIn := range []bool{false, true} {
		d, task := trashFixture(t)
		d.a.panelPort = 15060
		d.a.token = "isolated-panel-token"
		body := `{"id":"trash-test","action":"remove"}`
		if optIn {
			body = `{"id":"trash-test","action":"remove","deleteFiles":true}`
		}
		req := httptest.NewRequest("POST", "http://127.0.0.1:15060/api/downloads/action", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:1000"
		req.Header.Set("X-App-Token", d.a.token)
		response := httptest.NewRecorder()
		d.a.handler(false).ServeHTTP(response, req)
		if response.Code != 200 {
			t.Fatalf("API response %d: %s", response.Code, response.Body.String())
		}
		_, err := os.Stat(task.Path)
		if optIn && !os.IsNotExist(err) {
			t.Fatal("explicit opt-in did not move the test file")
		}
		if !optIn && err != nil {
			t.Fatal("omitting deleteFiles changed the test file")
		}
	}
}

func trashFixture(t *testing.T) (*DownloadManager, *DownloadTask) {
	t.Helper()
	a := downloadTestApp(t)
	d, err := a.downloads()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(a.root, "downloads")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fixture.iso")
	if err = os.WriteFile(path, []byte("download-content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path+".aria2", []byte("resume-data"), 0600); err != nil {
		t.Fatal(err)
	}
	task := &DownloadTask{ID: "trash-test", DownloadSpec: DownloadSpec{URL: "https://example.invalid/PRIVATE-TOKEN", Dir: dir, Connections: 4, ProxyMode: "direct"}, Status: "complete", Completed: "16", Path: path}
	d.journal.Tasks = append(d.journal.Tasks, task)
	return d, task
}
func TestDownloadDeleteDefaultRetainsFiles(t *testing.T) {
	d, task := trashFixture(t)
	if err := d.action(task.ID, "remove", DownloadSpec{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(task.Path); err != nil {
		t.Fatal("default removed file")
	}
	if _, err := os.Stat(task.Path + ".aria2"); err != nil {
		t.Fatal("default removed control file")
	}
}
func TestDownloadDeleteOptInIsRecoverable(t *testing.T) {
	d, task := trashFixture(t)
	if err := d.action(task.ID, "remove", DownloadSpec{}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(task.Path); !os.IsNotExist(err) {
		t.Fatal("file not moved")
	}
	matches, err := filepath.Glob(filepath.Join(task.Dir, ".Mirroxy-trash", "*", "download.bin"))
	if err != nil || len(matches) != 1 {
		t.Fatal(matches, err)
	}
	b, _ := os.ReadFile(matches[0])
	if string(b) != "download-content" {
		t.Fatal("file changed")
	}
	meta, _ := os.ReadFile(filepath.Join(filepath.Dir(matches[0]), "restore.json"))
	if strings.Contains(string(meta), "PRIVATE-TOKEN") {
		t.Fatal("trash metadata leaked URL")
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(matches[0]), "download.aria2")); err != nil {
		t.Fatal("resume data missing")
	}
}
func TestDownloadDeleteRefusesForeignOrSharedFiles(t *testing.T) {
	d, task := trashFixture(t)
	outside := filepath.Join(t.TempDir(), "foreign.iso")
	_ = os.WriteFile(outside, []byte("keep"), 0600)
	original := task.Path
	task.Path = outside
	if err := d.trashTaskFiles(task); err == nil {
		t.Fatal("accepted external file")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("foreign file touched")
	}
	task.Path = original
	other := *task
	other.ID = "other"
	d.journal.Tasks = append(d.journal.Tasks, &other)
	if err := d.trashTaskFiles(task); err == nil {
		t.Fatal("accepted shared file")
	}
	if _, err := os.Stat(original); err != nil {
		t.Fatal("shared file touched")
	}
}
func TestDownloadDeleteDoesNotGuessZeroByteTaskFile(t *testing.T) {
	d, task := trashFixture(t)
	task.Status = "paused"
	task.Completed = "0"
	if err := d.trashTaskFiles(task); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(task.Path); err != nil {
		t.Fatal("planned but untouched file moved")
	}
}
