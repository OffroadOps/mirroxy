//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
