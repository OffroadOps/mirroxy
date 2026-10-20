//go:build !windows

package main

import "time"

// Relaunch-wait is only reachable from the Windows tray menu; other platforms
// have no detached-relaunch caller, so the wait is a no-op.
func waitForProcessExit(pid int, timeout time.Duration) error { return nil }
