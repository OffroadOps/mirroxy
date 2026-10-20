//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var shellExecuteW = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW")

func platformApplicationRoutingSupported() bool { return true }

func startElevatedApplicationRouter(root, config string, controlPort int, secret string, parentPID int) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	operation, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	args := []string{
		"-app-router-helper", "-dir", root, "-app-router-config", config,
		"-app-router-control", strconv.Itoa(controlPort), "-app-router-secret", secret,
		"-app-router-parent", strconv.Itoa(parentPID),
	}
	escaped := make([]string, 0, len(args))
	for _, arg := range args {
		escaped = append(escaped, windows.EscapeArg(arg))
	}
	parameters, _ := windows.UTF16PtrFromString(strings.Join(escaped, " "))
	directory, _ := windows.UTF16PtrFromString(root)
	result, _, callErr := shellExecuteW.Call(0, uintptr(unsafe.Pointer(operation)), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(parameters)), uintptr(unsafe.Pointer(directory)), uintptr(windows.SW_HIDE))
	if result <= 32 {
		if callErr != nil && !errors.Is(callErr, windows.ERROR_SUCCESS) {
			return fmt.Errorf("管理员授权被取消或启动失败：%w", callErr)
		}
		return fmt.Errorf("管理员授权被取消或启动失败（代码 %d）", result)
	}
	return nil
}

func runApplicationRouterHelper(root, config string, controlPort int, secret string, parentPID int) error {
	if controlPort < 1024 || controlPort > 65535 || len(secret) < 32 || parentPID <= 0 {
		return errors.New("应用路由助手参数无效")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	config, err = filepath.Abs(config)
	if err != nil {
		return err
	}
	expected := filepath.Join(root, "data", "runtime", "Mirroxy_app_router.json")
	if !strings.EqualFold(filepath.Clean(config), filepath.Clean(expected)) {
		return errors.New("应用路由配置不在允许的位置")
	}
	app := &App{root: root}
	core := app.corePath("sing-box")
	if err = app.validateCore("sing-box", config, core); err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", controlPort))
	if err != nil {
		return err
	}
	defer listener.Close()

	logPath := filepath.Join(root, "data", "runtime", "Mirroxy_app_router.log")
	if err = os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		return err
	}
	if st, statErr := os.Stat(logPath); statErr == nil && st.Size() > 2<<20 {
		_ = os.Rename(logPath, logPath+".previous")
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(core, "run", "-c", config)
	hideCommand(cmd)
	cmd.Env = app.coreEnv()
	cmd.Dir = root
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err = cmd.Start(); err != nil {
		return err
	}
	coreDone := make(chan error, 1)
	go func() { coreDone <- cmd.Wait() }()
	time.Sleep(1200 * time.Millisecond)
	select {
	case err = <-coreDone:
		return fmt.Errorf("应用路由核心启动失败：%w", err)
	default:
	}
	_ = os.Remove(config)

	stop := make(chan struct{})
	var stopOnce sync.Once
	authorized := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Mirroxy-Router-Token") != secret {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next(w, r)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/status", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"running":true}`))
	}))
	mux.HandleFunc("/stop", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		stopOnce.Do(func() { close(stop) })
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second}
	serverDone := make(chan struct{})
	go func() { _ = server.Serve(listener); close(serverDone) }()
	parentDone := waitForApplicationRouterParent(uint32(parentPID))
	coreExited := false
	select {
	case <-stop:
	case <-parentDone:
	case <-serverDone:
		err = errors.New("应用路由控制服务意外停止")
	case err = <-coreDone:
		coreExited = true
		if err == nil {
			err = errors.New("应用路由核心意外退出")
		}
	}
	if cmd.Process != nil && !coreExited {
		_ = cmd.Process.Kill()
		select {
		case <-coreDone:
		case <-time.After(3 * time.Second):
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_ = server.Shutdown(ctx)
	cancel()
	select {
	case <-serverDone:
	case <-time.After(time.Second):
	}
	return err
}

func waitForApplicationRouterParent(pid uint32) <-chan struct{} {
	done := make(chan struct{})
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		close(done)
		return done
	}
	go func() {
		defer windows.CloseHandle(handle)
		_, _ = windows.WaitForSingleObject(handle, windows.INFINITE)
		close(done)
	}()
	return done
}

func listApplicationProcesses() ([]applicationProcess, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err = windows.Process32First(snapshot, &entry); err != nil {
		return nil, err
	}
	processes := []applicationProcess{}
	seen := map[string]bool{}
	blocked := map[string]bool{"mirroxy.exe": true, "mirroxy_core_a.exe": true, "mirroxy_core_b.exe": true, "upgrade.exe": true}
	for {
		name := windows.UTF16ToString(entry.ExeFile[:])
		path := queryApplicationProcessPath(entry.ProcessID)
		key := strings.ToLower(first(path, name))
		if name != "" && strings.EqualFold(filepath.Ext(name), ".exe") && !blocked[strings.ToLower(name)] && !seen[key] {
			seen[key] = true
			processes = append(processes, applicationProcess{PID: int(entry.ProcessID), Name: name, Path: path})
		}
		if err = windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return nil, err
		}
	}
	sortedApplicationProcesses(processes)
	return processes, nil
}

func queryApplicationProcessPath(pid uint32) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buffer[:size])
}
