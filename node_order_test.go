package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNodePlacementPersistsWithoutRestartingCore(t *testing.T) {
	a := renameTestApp(t)
	a.cfg.Nodes[0].Enabled = true
	core := &Core{}
	a.cores["xray"] = core
	counters := a.trafficFor("rename-test")
	counters.Uploaded, counters.Downloaded = 123, 456
	before := cloneSettings(a.cfg)
	if err := a.changeNode("rename-test", "move-bottom", ""); err != nil {
		t.Fatal(err)
	}
	before.Nodes[0].ListBottom = true
	actualJSON, _ := json.Marshal(a.cfg)
	expectedJSON, _ := json.Marshal(before)
	if !bytes.Equal(actualJSON, expectedJSON) || a.cores["xray"] != core || !core.alive() || counters.Uploaded != 123 || counters.Downloaded != 456 {
		t.Fatal("placement changed proxy state, identity, port or traffic")
	}
	reloaded := &App{root: a.root}
	if err := reloaded.load(); err != nil || !reloaded.cfg.Nodes[0].ListBottom {
		t.Fatal("placement did not survive reload", err)
	}
	if err := a.changeNode("rename-test", "auto-order", ""); err != nil || a.cfg.Nodes[0].ListBottom || a.cores["xray"] != core {
		t.Fatal("restoring automatic order disrupted proxy", err)
	}
}

func TestNodePlacementFailedSaveRollsBack(t *testing.T) {
	a := renameTestApp(t)
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	a.root, a.store = blocker, nil
	if err := a.setNodeListBottom("rename-test", true); err == nil || a.cfg.Nodes[0].ListBottom {
		t.Fatal("failed placement save changed in-memory state")
	}
	if err := a.setNodeListBottom("missing", true); err == nil {
		t.Fatal("missing node accepted")
	}
}
