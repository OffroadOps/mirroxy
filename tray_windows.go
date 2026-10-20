//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var trayUser = windows.NewLazySystemDLL("user32.dll")
var trayShell = windows.NewLazySystemDLL("shell32.dll")

type trayClass struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}
type trayNotify struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                windows.GUID
	BalloonIcon         uintptr
}
type trayPoint struct{ X, Y int32 }
type trayMessage struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   trayPoint
	Private uint32
}

const (
	trayMenuOpen      = 1
	trayMenuUpdate    = 2
	trayMenuExit      = 3
	trayMenuReload    = 4
	trayMenuElevate   = 5
	trayMenuToggleAll = 6

	trayProxyNodeBase  = 20 // + index into the running-node snapshot
	proxyNodeSlotLimit = 40

	trayRouteBypassCN  = 100
	trayRouteBlacklist = 101
	trayRouteProxy     = 102
)

// setAllNodesEnabled is the tray master switch: run or stop every node in one
// move. Stopping lets syncProxy hand the system proxy back to its backup.
func (a *App) setAllNodesEnabled(enable bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.busyUpdate {
		a.event("核心更新中，暂不执行托盘批量切换")
		return
	}
	before := cloneSettings(a.cfg)
	for i := range a.cfg.Nodes {
		if enable && a.cfg.Nodes[i].Port == 0 {
			port, err := a.allocatePort()
			if err != nil {
				a.event("托盘：分配端口失败，仅启用已就绪节点")
				break
			}
			a.cfg.Nodes[i].Port = port
		}
		a.cfg.Nodes[i].Enabled = enable
	}
	if err := a.reconcile(); err != nil {
		a.cfg = before
		_ = a.reconcile()
		a.event("托盘批量切换失败，已恢复原状：" + err.Error())
		return
	}
	if err := a.save(); err != nil {
		a.cfg = before
		_ = a.reconcile()
		a.event("托盘批量切换保存失败：" + err.Error())
		return
	}
	a.syncProxy()
	if enable {
		a.event("托盘：已运行全部节点")
	} else {
		a.event("托盘：已停止全部节点")
	}
}

func trayUpdateLabel(current string, update AppUpdateState) (string, bool) {
	if !update.Available || !newerVersion(update.Latest, current) {
		return "", false
	}
	return "更新到 v" + update.Latest + "（可选）", true
}
func (a *App) trayUpdateLoop() {
	check := func() { _, _ = a.checkAppUpdate() }
	check()
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-a.shutdown:
			return
		case <-ticker.C:
			check()
		}
	}
}
func confirmTrayUpdate(latest string) bool {
	selected, _, _ := trayUser.NewProc("MessageBoxW").Call(0,
		uintptr(unsafe.Pointer(wide("发现 Mirroxy v"+latest+"，是否现在更新？\n\n更新由你选择，不会强制安装。"))),
		uintptr(unsafe.Pointer(wide("Mirroxy 更新"))), 0x44) // MB_YESNO | MB_ICONINFORMATION
	return selected == 6 // IDYES
}
func showTrayUpdateError(err error) {
	trayUser.NewProc("MessageBoxW").Call(0,
		uintptr(unsafe.Pointer(wide("无法开始更新："+err.Error()))),
		uintptr(unsafe.Pointer(wide("Mirroxy 更新"))), 0x10) // MB_ICONERROR
}
func showTrayReloadError(err error) {
	trayUser.NewProc("MessageBoxW").Call(0,
		uintptr(unsafe.Pointer(wide("重新加载失败："+err.Error()))),
		uintptr(unsafe.Pointer(wide("Mirroxy"))), 0x10) // MB_ICONERROR
}
func showTrayActionError(action string, err error) {
	trayUser.NewProc("MessageBoxW").Call(0,
		uintptr(unsafe.Pointer(wide(action+"失败："+err.Error()))),
		uintptr(unsafe.Pointer(wide("Mirroxy"))), 0x10) // MB_ICONERROR
}

// trayApplyProxy and traySetRoute run tray actions off the UI thread and
// surface failures in a message box instead of the silent event log.
func (a *App) trayApplyProxy(mode, id string) {
	if err := a.applySystemProxy(mode, id); err != nil {
		showTrayActionError("系统代理设置", err)
	}
}
func (a *App) traySetRoute(mode string) {
	if err := a.setRoutingMode(mode); err != nil {
		showTrayActionError("路由模式切换", err)
	}
}

// trayReleaseProxy hands the system proxy back to the pre-Mirroxy settings.
func (a *App) trayReleaseProxy() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.restoreProxy()
	a.cfg.SystemProxyMode = "unchanged"
	a.cfg.SystemProxyNode = ""
	a.cfg.PausedSystemProxy = nil
	if err := a.save(); err != nil {
		a.event("托盘撤销系统代理保存失败：" + err.Error())
		return
	}
	a.event("托盘：已撤销系统代理")
}

// restartElevated launches an elevated copy that waits for this process to
// exit and then takes over, so later elevation-dependent features (the TUN
// application router) inherit the already-approved administrator context.
func (a *App) restartElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := fmt.Sprintf(`-panel %d -relaunch-wait %d -no-open -dir "%s"`, a.panelPort, os.Getpid(), a.root)
	result, _, _ := trayShell.NewProc("ShellExecuteW").Call(0,
		uintptr(unsafe.Pointer(wide("runas"))),
		uintptr(unsafe.Pointer(wide(exe))),
		uintptr(unsafe.Pointer(wide(args))),
		uintptr(unsafe.Pointer(wide(filepath.Dir(exe)))), 1) // SW_SHOWNORMAL
	if result <= 32 {
		return fmt.Errorf("管理员授权被取消或启动失败（代码 %d）", result)
	}
	go func() {
		time.Sleep(600 * time.Millisecond)
		a.requestQuit()
	}()
	return nil
}

// restartApplication launches a detached copy that waits for this process to
// exit, then asks this instance to shut down gracefully. Nodes, traffic totals
// and the tray come back in the new process.
func (a *App) restartApplication() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "-panel", strconv.Itoa(a.panelPort), "-relaunch-wait", strconv.Itoa(os.Getpid()))
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008} // DETACHED_PROCESS
	if err = cmd.Start(); err != nil {
		return err
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		a.requestQuit()
	}()
	return nil
}

// trayTruncate shortens long node names for one-line menu items.
func trayTruncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func trayNumber(value any) uint64 {
	switch n := value.(type) {
	case uint64:
		return n
	case int64:
		return uint64(n)
	case int:
		return uint64(n)
	case float64:
		return uint64(n)
	}
	return 0
}

func trayBytes(n uint64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := uint64(1024), 0
	for m := n / 1024; m >= 1024; m /= 1024 {
		div *= 1024
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(n)/float64(div), "KMGTPE"[exp])
}

func trayRate(n float64) string {
	if n < 1024 {
		return fmt.Sprintf("%.0fB", n)
	}
	value, unit := n, ""
	for _, c := range "KMGT" {
		if value < 1024 {
			break
		}
		value /= 1024
		unit = string(c)
	}
	return fmt.Sprintf("%.1f%s", value, unit)
}

func wide(s string) *uint16 { p, _ := windows.UTF16PtrFromString(s); return p }
func (a *App) closeTray() {
	if a.trayWindow != 0 {
		trayUser.NewProc("PostMessageW").Call(a.trayWindow, 0x0010, 0, 0)
	}
}
func (a *App) runTray() {
	defer func() {
		if a.trayDone != nil {
			close(a.trayDone)
		}
	}()
	select {
	case <-a.shutdown:
		return
	default:
	}
	go a.trayUpdateLoop()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	// The EXE and notification area use the same multi-resolution brand icon.
	size, _, _ := trayUser.NewProc("GetSystemMetrics").Call(49) // SM_CXSMICON
	icon, _, _ := trayUser.NewProc("LoadImageW").Call(instance, 1, 1, size, size, 0)
	if icon != 0 {
		defer trayUser.NewProc("DestroyIcon").Call(icon)
	} else {
		icon, _, _ = trayUser.NewProc("LoadIconW").Call(0, 32512)
	}
	taskbar, _, _ := trayUser.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(wide("TaskbarCreated"))))
	data := trayNotify{ID: 1, Flags: 7, Callback: 0x8001, Icon: icon}
	data.Size = uint32(unsafe.Sizeof(data))
	tip, _ := windows.UTF16FromString("Mirroxy · 双击打开面板")
	copy(data.Tip[:], tip)
	notify := func(action uintptr) uintptr {
		v, _, _ := trayShell.NewProc("Shell_NotifyIconW").Call(action, uintptr(unsafe.Pointer(&data)))
		return v
	}
	open := func() { openBrowser(a.panelURL()) }
	callback := windows.NewCallback(func(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
		switch msg {
		case 0x8001:
			if lParam == 0x0203 {
				go open()
			}
			if lParam == 0x0205 {
				menu, _, _ := trayUser.NewProc("CreatePopupMenu").Call()
				defer trayUser.NewProc("DestroyMenu").Call(menu)
				a.mu.Lock()
				route := first(a.cfg.RoutingMode, "bypass-cn")
				acceleratorPaused := a.cfg.PausedSystemProxy != nil
				running := []Node{}
				nodesSnapshot := append([]Node(nil), a.cfg.Nodes...)
				for _, n := range a.cfg.Nodes {
					if n.Enabled && a.cores[n.coreKey()].alive() {
						running = append(running, n)
					}
				}
				traffic := a.trafficState()
				update := a.appUpdate
				a.mu.Unlock()
				enabled, proxyServer := currentProxy()
				activeProxyID := ""
				nameOf := map[string]string{}
				for _, n := range nodesSnapshot {
					nameOf[n.ID] = n.Name
					if n.Port > 0 && enabled == 1 && proxyServer == fmt.Sprintf("127.0.0.1:%d", n.Port) {
						activeProxyID = n.ID
					}
				}
				proxyInfo := "未启用"
				switch {
				case acceleratorPaused:
					proxyInfo = "已暂停"
				case activeProxyID != "":
					proxyInfo = trayTruncate(nameOf[activeProxyID], 14)
				}
				routeInfo := map[string]string{"bypass-cn": "绕过大陆", "blacklist": "黑名单", "proxy": "全局"}[route]
				checked := func(on bool) uintptr {
					if on {
						return 0x8 // MF_CHECKED
					}
					return 0
				}
				AppendMenu := func(m uintptr, flags, id uintptr, text string) {
					trayUser.NewProc("AppendMenuW").Call(m, flags, id, uintptr(unsafe.Pointer(wide(text))))
				}
				proxyMenu, _, _ := trayUser.NewProc("CreatePopupMenu").Call()
				defer trayUser.NewProc("DestroyMenu").Call(proxyMenu)
				if len(running) == 0 {
					AppendMenu(proxyMenu, 0x3, 0, "（无运行中的节点）")
				} else {
					for i, n := range running {
						AppendMenu(proxyMenu, checked(n.ID == activeProxyID), trayProxyNodeBase+uintptr(i), trayTruncate(n.Name, 12))
					}
				}
				AppendMenu(menu, 0x10, proxyMenu, "系统代理")
				AppendMenu(menu, 0x3, 0, proxyInfo)
				routeMenu, _, _ := trayUser.NewProc("CreatePopupMenu").Call()
				defer trayUser.NewProc("DestroyMenu").Call(routeMenu)
				AppendMenu(routeMenu, checked(route == "bypass-cn"), trayRouteBypassCN, "绕过大陆")
				AppendMenu(routeMenu, checked(route == "blacklist"), trayRouteBlacklist, "黑名单")
				AppendMenu(routeMenu, checked(route == "proxy"), trayRouteProxy, "全局")
				AppendMenu(menu, 0x10, routeMenu, "路由模式")
				AppendMenu(menu, 0x3, 0, routeInfo)
				AppendMenu(menu, 0x800, 0, "")
				AppendMenu(menu, 0, trayMenuOpen, "打开面板")
				AppendMenu(menu, 0, trayMenuReload, "重启面板")
				AppendMenu(menu, 0, trayMenuElevate, "UAC运行")
				anyRunning := false
				for _, n := range running {
					if n.Enabled {
						anyRunning = true
						break
					}
				}
				if anyRunning {
					AppendMenu(menu, 0, trayMenuToggleAll, "停止代理")
				} else {
					AppendMenu(menu, 0, trayMenuToggleAll, "运行代理")
				}
				AppendMenu(menu, 0x800, 0, "")
				AppendMenu(menu, 0x3, 0, fmt.Sprintf("↑%s/s · %s", trayRate(float64(trayNumber(traffic["uploadBytesPerSecond"]))), trayBytes(trayNumber(traffic["uploadedBytes"]))))
				AppendMenu(menu, 0x3, 0, fmt.Sprintf("↓%s/s · %s", trayRate(float64(trayNumber(traffic["downloadBytesPerSecond"]))), trayBytes(trayNumber(traffic["downloadedBytes"]))))
				AppendMenu(menu, 0x3, 0, "v"+version)
				updateLabel, updateAvailable := trayUpdateLabel(version, update)
				if updateAvailable {
					AppendMenu(menu, 0, trayMenuUpdate, updateLabel)
				}
				AppendMenu(menu, 0x800, 0, "")
				AppendMenu(menu, 0, trayMenuExit, "退出")
				var pt trayPoint
				trayUser.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pt)))
				trayUser.NewProc("SetForegroundWindow").Call(hwnd)
				selected, _, _ := trayUser.NewProc("TrackPopupMenu").Call(menu, 0x102, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
				switch {
				case selected == trayMenuOpen:
					go open()
				case selected == trayMenuReload:
					if err := a.restartApplication(); err != nil {
						showTrayReloadError(err)
					} else {
						a.mu.Lock()
						a.event("托盘触发重新加载：保存状态并重启后台")
						a.mu.Unlock()
					}
				case selected == trayMenuElevate:
					if err := a.restartElevated(); err != nil {
						showTrayReloadError(err)
					} else {
						a.mu.Lock()
						a.event("托盘触发管理员运行：等待 UAC 授权后重启后台")
						a.mu.Unlock()
					}
				case selected == trayMenuToggleAll:
					go a.setAllNodesEnabled(!anyRunning)
				case selected >= trayProxyNodeBase && selected < trayProxyNodeBase+proxyNodeSlotLimit:
					if index := int(selected - trayProxyNodeBase); index < len(running) {
						if running[index].ID == activeProxyID {
							go a.trayReleaseProxy()
						} else {
							go a.trayApplyProxy("auto", running[index].ID)
						}
					}
				case selected == trayRouteBypassCN:
					go a.traySetRoute("bypass-cn")
				case selected == trayRouteBlacklist:
					go a.traySetRoute("blacklist")
				case selected == trayRouteProxy:
					go a.traySetRoute("proxy")
				case selected == trayMenuUpdate && updateAvailable:
					if confirmTrayUpdate(update.Latest) {
						if err := a.startAppUpdate(); err != nil {
							showTrayUpdateError(err)
						}
					}
				case selected == trayMenuExit:
					a.requestQuit()
				}
				trayUser.NewProc("PostMessageW").Call(hwnd, 0, 0, 0)
			}
			return 0
		case 0x0010:
			trayUser.NewProc("DestroyWindow").Call(hwnd)
			return 0
		case 0x0002:
			notify(2)
			trayUser.NewProc("PostQuitMessage").Call(0)
			return 0
		case 0x0011:
			a.requestQuit()
			return 1
		}
		if uintptr(msg) == taskbar {
			notify(0)
			return 0
		}
		result, _, _ := trayUser.NewProc("DefWindowProcW").Call(hwnd, uintptr(msg), wParam, lParam)
		return result
	})
	class := trayClass{WndProc: callback, Instance: instance, Icon: icon, SmallIcon: icon, ClassName: wide("Mirroxy.Tray")}
	class.Size = uint32(unsafe.Sizeof(class))
	atom, _, _ := trayUser.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		a.mu.Lock()
		a.event("托盘窗口注册失败")
		a.mu.Unlock()
		return
	}
	defer trayUser.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(class.ClassName)), instance)
	hwnd, _, _ := trayUser.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class.ClassName)), uintptr(unsafe.Pointer(wide("Mirroxy"))), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		a.mu.Lock()
		a.event("托盘窗口创建失败")
		a.mu.Unlock()
		return
	}
	data.Window = hwnd
	if notify(0) == 0 {
		trayUser.NewProc("DestroyWindow").Call(hwnd)
		a.mu.Lock()
		a.event("托盘图标添加失败")
		a.mu.Unlock()
		return
	}
	a.mu.Lock()
	a.trayWindow = hwnd
	select {
	case <-a.shutdown:
		a.closeTray()
	default:
	}
	a.mu.Unlock()
	var msg trayMessage
	for {
		result, _, _ := trayUser.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			break
		}
		trayUser.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		trayUser.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
	a.mu.Lock()
	a.trayWindow = 0
	a.mu.Unlock()
}
