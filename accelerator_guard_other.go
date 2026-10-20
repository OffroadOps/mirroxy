//go:build !windows

package main

// System-proxy pausing only applies to the Windows registry; other platforms
// never report a managed system proxy, so the guard is a no-op.

func (a *App) pauseSystemProxyForAcceleratorLocked(product string) bool { return false }

func (a *App) managedSystemProxyActive() (mode, node string, ok bool) { return "", "", false }
