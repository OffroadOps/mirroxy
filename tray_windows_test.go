//go:build windows

package main

import (
	"strings"
	"testing"
)

func TestTrayVersionAndOptionalUpdateLabels(t *testing.T) {
	if _, ok := trayUpdateLabel("0.5.1", AppUpdateState{Available: true, Latest: "0.5.1"}); ok {
		t.Fatal("same version must not show an update action")
	}
	if _, ok := trayUpdateLabel("0.5.1", AppUpdateState{Available: true, Latest: "0.5.0"}); ok {
		t.Fatal("older version must not show an update action")
	}
	label, ok := trayUpdateLabel("0.5.1", AppUpdateState{Available: true, Latest: "0.5.2"})
	if !ok || label != "更新到 v0.5.2（可选）" {
		t.Fatalf("new version action missing: %q, %v", label, ok)
	}
}

func TestTrayTruncateKeepsShortNamesAndMarksLongOnes(t *testing.T) {
	if got := trayTruncate("DMIT.LAX.4809", 14); got != "DMIT.LAX.4809" {
		t.Fatalf("short name must stay unchanged: %q", got)
	}
	long := "超级长节点名称超过十四个字符限制"
	got := trayTruncate(long, 14)
	if len([]rune(got)) != 15 || !strings.HasSuffix(got, "…") {
		t.Fatalf("long name must be truncated with an ellipsis: %q", got)
	}
}

func TestTrayByteAndRateFormatting(t *testing.T) {
	cases := map[uint64]string{0: "0B", 512: "512B", 2048: "2.0K", 367 * 1024 * 1024: "367.0M", 10737418240: "10.0G"}
	for value, want := range cases {
		if got := trayBytes(value); got != want {
			t.Fatalf("trayBytes(%d) = %q, want %q", value, got, want)
		}
	}
	if got := trayRate(367); got != "367B" {
		t.Fatalf("trayRate(367) = %q", got)
	}
	if got := trayRate(2.5 * 1024 * 1024); got != "2.5M" {
		t.Fatalf("trayRate(2.5MiB) = %q", got)
	}
	if trayNumber(uint64(7)) != 7 || trayNumber(int64(8)) != 8 || trayNumber(9) != 9 || trayNumber(10.0) != 10 || trayNumber(nil) != 0 {
		t.Fatal("trayNumber must accept the numeric shapes stored in traffic state")
	}
}
