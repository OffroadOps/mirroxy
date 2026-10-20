package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadOpenUsesKnownCompletedFileOnly(t *testing.T) {
	d, task := trashFixture(t)
	var launched string
	var reveal bool
	d.openTarget = func(path string, folder bool) error { launched = path; reveal = folder; return nil }
	if err := d.action(task.ID, "open-file", DownloadSpec{Dir: t.TempDir()}); err != nil || launched != task.Path || reveal {
		t.Fatal("completed file target", err, launched)
	}
	task.Status = "active"
	launched = ""
	if err := d.action(task.ID, "open-file", DownloadSpec{}); err == nil || launched != "" {
		t.Fatal("launched incomplete file")
	}
	if err := d.action(task.ID, "open-folder", DownloadSpec{}); err != nil || !reveal || launched != task.Path {
		t.Fatal("folder target", err)
	}
	task.Path = filepath.Join(t.TempDir(), "foreign.exe")
	_ = os.WriteFile(task.Path, []byte("test"), 0600)
	if _, err := d.downloadOpenTarget(task, true); err == nil {
		t.Fatal("opened an external path")
	}
}

func TestDownloadStallTrackingAndRecovery(t *testing.T) {
	task := &DownloadTask{}
	now := time.Now()
	status := ariaStatus{Status: "active", Completed: "100", Speed: "0"}
	trackDownloadProgress(task, status, now)
	trackDownloadProgress(task, status, now.Add(44*time.Second))
	if task.Stalled {
		t.Fatal("warned too early")
	}
	trackDownloadProgress(task, status, now.Add(46*time.Second))
	if !task.Stalled {
		t.Fatal("stall not detected")
	}
	status.Completed = "200"
	trackDownloadProgress(task, status, now.Add(47*time.Second))
	if task.Stalled {
		t.Fatal("progress did not clear stall")
	}
	status.Status = "paused"
	trackDownloadProgress(task, status, now.Add(200*time.Second))
	if task.Stalled {
		t.Fatal("paused task reported stalled")
	}
}

func TestDownloadRecoveryAndConcurrencyOptions(t *testing.T) {
	d, task := trashFixture(t)
	options, err := d.options(task)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"connect-timeout": "15", "timeout": "30", "max-tries": "5", "retry-wait": "3", "check-certificate": "true", "continue": "true", "allow-overwrite": "false", "force-save": "true"} {
		if options[key] != value {
			t.Fatalf("%s=%q", key, options[key])
		}
	}
	snapshot, err := d.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["maxConcurrent"] != 30 || snapshot["maxConnections"] != 16 {
		t.Fatal("capacity mismatch")
	}
}
