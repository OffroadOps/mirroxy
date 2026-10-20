//go:build windows

package main

import (
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

func hideCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
func openBrowser(url string) {
	cmd := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
	hideCommand(cmd)
	_ = cmd.Start()
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
}
func createJob() uintptr {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		windows.CloseHandle(h)
		return 0
	}
	return uintptr(h)
}
func assignJob(job uintptr, pid int) {
	if job == 0 {
		return
	}
	h, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if e != nil {
		return
	}
	defer windows.CloseHandle(h)
	_ = windows.AssignProcessToJobObject(windows.Handle(job), h)
}
func closeJob(job uintptr) {
	if job != 0 {
		_ = windows.CloseHandle(windows.Handle(job))
	}
}

const internetSettings = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

var proxySettingsPath = internetSettings

type ProxyBackup struct {
	Enable     uint64 `json:"enable"`
	Server     string `json:"server"`
	Override   string `json:"override"`
	AutoConfig string `json:"autoConfig"`
	Managed    string `json:"managed"`
	NodeID     string `json:"nodeID"`
	ManagedPAC string `json:"managedPAC,omitempty"`
}

func currentPAC() string {
	k, e := proxyKey()
	if e != nil {
		return ""
	}
	defer k.Close()
	s, _, _ := k.GetStringValue("AutoConfigURL")
	return s
}

func proxyKey() (registry.Key, error) {
	return registry.OpenKey(registry.CURRENT_USER, proxySettingsPath, registry.QUERY_VALUE|registry.SET_VALUE)
}
func currentProxy() (uint64, string) {
	k, e := proxyKey()
	if e != nil {
		return 0, ""
	}
	defer k.Close()
	en, _, _ := k.GetIntegerValue("ProxyEnable")
	s, _, _ := k.GetStringValue("ProxyServer")
	return en, s
}
func notifyProxy() {
	dll := windows.NewLazySystemDLL("wininet.dll")
	p := dll.NewProc("InternetSetOptionW")
	_, _, _ = p.Call(0, 39, 0, 0)
	_, _, _ = p.Call(0, 37, 0, 0)
}
func (a *App) readProxyBackup() (*ProxyBackup, error) {
	var v ProxyBackup
	e := a.readState("system-proxy", &v)
	return &v, e
}
func (a *App) setProxy(id string) error {
	n := a.node(id)
	if n == nil || !n.Enabled || !a.cores[n.coreKey()].alive() {
		return fmt.Errorf("请先启动该节点")
	}
	k, e := proxyKey()
	if e != nil {
		return e
	}
	defer k.Close()
	b, e := a.readProxyBackup()
	if e != nil {
		b = &ProxyBackup{}
		b.Enable, _, _ = k.GetIntegerValue("ProxyEnable")
		b.Server, _, _ = k.GetStringValue("ProxyServer")
		b.Override, _, _ = k.GetStringValue("ProxyOverride")
		b.AutoConfig, _, _ = k.GetStringValue("AutoConfigURL")
	}
	b.Managed = fmt.Sprintf("127.0.0.1:%d", n.Port)
	b.ManagedPAC = ""
	b.NodeID = id
	if e = a.writeState("system-proxy", b); e != nil {
		return e
	}
	if e = k.SetStringValue("ProxyServer", b.Managed); e != nil {
		return e
	}
	if e = k.SetStringValue("ProxyOverride", "localhost;127.*;[::1];<local>"); e != nil {
		return e
	}
	if e = k.SetStringValue("AutoConfigURL", ""); e != nil {
		return e
	}
	if e = k.SetDWordValue("ProxyEnable", 1); e != nil {
		return e
	}
	notifyProxy()
	a.event("系统代理已切换到 " + n.Name)
	return nil
}
func (a *App) restoreProxy() {
	b, e := a.readProxyBackup()
	if e != nil {
		return
	}
	_, current := currentProxy()
	if (b.ManagedPAC != "" && currentPAC() == b.ManagedPAC) || (b.ManagedPAC == "" && current == b.Managed && currentPAC() == "") {
		k, err := proxyKey()
		if err != nil {
			return
		}
		defer k.Close()
		_ = k.SetStringValue("ProxyServer", b.Server)
		_ = k.SetStringValue("ProxyOverride", b.Override)
		_ = k.SetStringValue("AutoConfigURL", b.AutoConfig)
		_ = k.SetDWordValue("ProxyEnable", uint32(b.Enable))
		notifyProxy()
	}
	_ = a.deleteState("system-proxy")
}
func (a *App) syncProxy() {
	b, e := a.readProxyBackup()
	if e != nil {
		return
	}
	n := a.node(b.NodeID)
	if n == nil || !n.Enabled || !a.cores[n.coreKey()].alive() {
		a.restoreProxy()
	}
}
func (a *App) recoverProxy() { a.syncProxy() }

// retirePAC removes the registry leftovers of the removed PAC mode: an
// AutoConfigURL pointing at this panel's /pac/ endpoint is cleared once.
func (a *App) retirePAC() {
	pac := currentPAC()
	if pac == "" {
		return
	}
	ours := fmt.Sprintf("127.0.0.1:%d/pac/", a.panelPort)
	if !strings.Contains(pac, ours) && !strings.Contains(pac, "127.0.0.1:15060/pac/") && !strings.Contains(pac, "127.0.0.1:9527/pac/") {
		return
	}
	k, err := proxyKey()
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.SetStringValue("AutoConfigURL", "")
	enabled, _ := currentProxy()
	if enabled != 1 {
		_ = k.SetDWordValue("ProxyEnable", 0)
	}
	_ = a.deleteState("system-proxy")
	notifyProxy()
	a.event("已清理旧版 PAC 模式的系统设置残留")
}
func (a *App) applySystemProxy(mode, id string) error {
	before := cloneSettings(a.cfg)
	var e error
	switch mode {
	case "unchanged": // Explicitly leave v2rayN (or any other client) in control.
	case "auto":
		e = a.setProxy(id)
	case "clear":
		var k registry.Key
		k, e = proxyKey()
		if e == nil {
			defer k.Close()
			e = k.SetDWordValue("ProxyEnable", 0)
			if e == nil {
				e = k.SetStringValue("AutoConfigURL", "")
			}
			if e == nil {
				_ = a.deleteState("system-proxy")
				notifyProxy()
			}
		}
	default:
		return fmt.Errorf("无效系统代理模式")
	}
	if e != nil {
		return e
	}
	a.cfg.PausedSystemProxy = nil // A manual proxy decision cancels the accelerator pause.
	a.cfg.SystemProxyMode = mode
	a.cfg.SystemProxyNode = id
	if e = a.save(); e != nil {
		a.cfg = before
		if mode == "auto" {
			a.restoreProxy()
		}
		return e
	}
	return nil
}
func (a *App) setAutostart(enabled bool) error {
	k, _, e := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if e != nil {
		return e
	}
	defer k.Close()
	// Remove the pre-0.5 product entry during the one-time brand migration.
	_ = k.DeleteValue("scrcpyproxy")
	if !enabled {
		e = k.DeleteValue("Mirroxy")
		if e == registry.ErrNotExist {
			return nil
		}
		return e
	}
	exe := filepath.Join(a.root, "Mirroxy.exe")
	return k.SetStringValue("Mirroxy", fmt.Sprintf(`"%s" -no-open -dir "%s"`, exe, a.root))
}
func (a *App) recoverAutostart() { _ = a.setAutostart(a.cfg.Autostart) }
