package main

import (
	"fmt"
	"runtime"
	"time"
)

// PausedSystemProxy remembers the managed system-proxy intent (mode and node)
// while a game accelerator temporarily takes over, so the guard can restore it.
type PausedSystemProxy struct {
	Mode string `json:"mode"`
	Node string `json:"node,omitempty"`
}

func (a *App) acceleratorGuardEnabled() bool {
	return a.cfg.PauseSystemProxyOnAccelerator == nil || *a.cfg.PauseSystemProxyOnAccelerator
}

// bypassLANEnabled reports whether LAN and loopback addresses stay direct even
// in global proxy mode; enabled unless explicitly turned off.
func (a *App) bypassLANEnabled() bool {
	return a.cfg.BypassLAN == nil || *a.cfg.BypassLAN
}

// acceleratorRunning returns the product name of a known accelerator process,
// or an empty string. Process enumeration is Windows-only in practice.
func (a *App) acceleratorRunning() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	processes, err := listApplicationProcesses()
	if err != nil {
		return ""
	}
	if hits := detectAcceleratorConflicts(processes); len(hits) > 0 {
		if product, ok := hits[0]["product"].(string); ok {
			return product
		}
	}
	return ""
}

func (a *App) acceleratorGuardLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.shutdown:
			return
		case <-ticker.C:
			if runtime.GOOS == "windows" {
				a.acceleratorGuardTick()
			}
		}
	}
}

// acceleratorGuardTick pauses Mirroxy's managed system proxy while an
// accelerator runs and restores it afterwards. Only registrations Mirroxy
// itself set up are touched; system proxies from other software and per-app
// custom proxy targets (local node ports) are never modified.
func (a *App) acceleratorGuardTick() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.acceleratorGuardEnabled() {
		return
	}
	paused := a.cfg.PausedSystemProxy != nil
	managed := a.cfg.SystemProxyMode == "auto"
	if !paused && !managed {
		return
	}
	processes, err := listApplicationProcesses()
	if err != nil {
		return
	}
	hits := detectAcceleratorConflicts(processes)
	switch {
	case len(hits) > 0 && managed && !paused:
		product := ""
		if s, ok := hits[0]["product"].(string); ok {
			product = s
		}
		if a.pauseSystemProxyForAcceleratorLocked(product) {
			_ = a.save()
		}
	case len(hits) == 0 && paused:
		if a.resumeSystemProxyAfterAcceleratorLocked() {
			_ = a.save()
		}
	}
}

func (a *App) resumeSystemProxyAfterAcceleratorLocked() bool {
	snap := a.cfg.PausedSystemProxy
	if snap == nil {
		return false
	}
	a.cfg.PausedSystemProxy = nil
	enabled, proxy := currentProxy()
	if (enabled == 1 && proxy != "") || currentPAC() != "" {
		a.event("加速器已退出；系统代理已被其他程序占用，未自动恢复")
		return true
	}
	if err := a.applySystemProxy(snap.Mode, snap.Node); err != nil {
		a.event(fmt.Sprintf("加速器已退出，但系统代理恢复失败：%s", err.Error()))
		return true
	}
	a.event("加速器已退出，系统代理已恢复")
	return true
}
