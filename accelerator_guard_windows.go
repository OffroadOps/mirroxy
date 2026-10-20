//go:build windows

package main

// pauseSystemProxyForAcceleratorLocked withdraws only the values Mirroxy set
// (ProxyEnable); the ProxyBackup state entry is kept so the user's original
// pre-Mirroxy settings survive for later restores.
func (a *App) pauseSystemProxyForAcceleratorLocked(product string) bool {
	mode, _, ok := a.managedSystemProxyActive()
	if !ok || mode != "auto" {
		return false
	}
	k, err := proxyKey()
	if err != nil {
		return false
	}
	defer k.Close()
	if err = k.SetDWordValue("ProxyEnable", 0); err != nil {
		return false
	}
	a.cfg.PausedSystemProxy = &PausedSystemProxy{Mode: mode, Node: a.cfg.SystemProxyNode}
	notifyProxy()
	a.event("检测到加速器：" + product + "，已自动暂停系统代理；退出加速器后自动恢复")
	return true
}

// managedSystemProxyActive reports whether the Windows system proxy currently
// points at the node port Mirroxy itself configured.
func (a *App) managedSystemProxyActive() (mode, node string, ok bool) {
	b, err := a.readProxyBackup()
	if err != nil {
		return "", "", false
	}
	enabled, proxy := currentProxy()
	if b.Managed != "" && enabled == 1 && proxy == b.Managed {
		return "auto", b.NodeID, true
	}
	return "", "", false
}
