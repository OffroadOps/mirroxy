//go:build linux

package main

import (
	"errors"
	"os/exec"
	"syscall"
)

// systemd also owns the complete process group. Pdeathsig covers console runs.
func hideCommand(cmd *exec.Cmd)      { cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL} }
func openBrowser(string)             {}
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
	return errors.New("Linux 开机自启由系统服务管理，请使用 systemctl enable/disable mirroxy")
}
func (a *App) recoverAutostart() {}
func (a *App) applySystemProxy(mode, id string) error {
	if mode != "unchanged" {
		return errors.New("Linux 控制台使用节点端口或代理环境变量，不支持 Windows 系统代理模式")
	}
	a.cfg.SystemProxyMode, a.cfg.SystemProxyNode = "unchanged", ""
	return a.save()
}
