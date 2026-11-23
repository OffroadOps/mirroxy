//go:build windows

package main

import (
	"strings"
	"testing"
	"unicode/utf16"
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

func TestTrayTooltipFormattingAndUTF16Limit(t *testing.T) {
	idleStopped := trayTooltip(trayStateIdle, "", "bypass-cn", 0)
	if !strings.Contains(idleStopped, "待机") {
		t.Fatalf("unexpected stopped tooltip: %q", idleStopped)
	}
	idleRunning := trayTooltip(trayStateIdle, "", "bypass-cn", 2)
	if !strings.Contains(idleRunning, "2 个节点") {
		t.Fatalf("unexpected idle running tooltip: %q", idleRunning)
	}
	proxying := trayTooltip(trayStateProxy, "HK-Relay", "blacklist", 1)
	if !strings.Contains(proxying, "代理中：HK-Relay") || !strings.Contains(proxying, "黑名单") {
		t.Fatalf("unexpected proxy tooltip: %q", proxying)
	}
	paused := trayTooltip(trayStatePaused, "HK-Relay", "proxy", 1)
	if !strings.Contains(paused, "加速器接管中") {
		t.Fatalf("unexpected paused tooltip: %q", paused)
	}

	veryLong := strings.Repeat("超长emoji节点😀", 25)
	if utf16Len := len(utf16.Encode([]rune(fitUTF16Tip("Mirroxy · "+veryLong, 127)))); utf16Len > 127 {
		t.Fatalf("UTF-16 tooltip exceeded 127 units: %d", utf16Len)
	}
}

func TestTrayCreateBadgedIcons(t *testing.T) {
	for _, st := range []trayVisualState{trayStateIdle, trayStateProxy, trayStatePaused} {
		h := createBadgedTrayIcon(32, st)
		if h == 0 {
			t.Fatalf("createBadgedTrayIcon returned 0 for state %d", st)
		}
		_, _, _ = trayUser.NewProc("DestroyIcon").Call(h)
	}
}
