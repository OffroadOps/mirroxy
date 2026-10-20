//go:build darwin

package main

import (
	"errors"
	"os/exec"
)

func hideCommand(*exec.Cmd)          {}
func openBrowser(address string)     { _ = exec.Command("/usr/bin/open", address).Run() }
func createJob() uintptr             { return 0 }
func assignJob(uintptr, int)         {}
func closeJob(uintptr)               {}
func currentProxy() (uint64, string) { return 0, "" }
func currentPAC() string             { return "" }
func (a *App) restoreProxy()         {}
func (a *App) retirePAC()            {}
func (a *App) syncProxy()            {}
func (a *App) recoverProxy()         {}
func (a *App) runTray()              { close(a.trayDone) }
func (a *App) closeTray()            {}
func (a *App) setAutostart(bool) error {
	return errors.New("macOS 自动启动由安装器的 LaunchAgent 管理")
}
func (a *App) recoverAutostart() {}
func (a *App) applySystemProxy(mode, id string) error {
	if mode != "unchanged" {
		return errors.New("macOS 请在应用或控制台中使用节点的 HTTP / SOCKS5 端口")
	}
	a.cfg.SystemProxyMode, a.cfg.SystemProxyNode = "unchanged", ""
	return a.save()
}
