//go:build windows

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestInstallerMockBackendProcess(t *testing.T) {
	port := os.Getenv("MIRROXY_INSTALLER_MOCK_PORT")
	if port == "" {
		t.Skip("subprocess-only installer backend mock")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:"+port)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(M{"version": version, "token": "fake-installer-test-token"})
	})
	mux.HandleFunc("/api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-App-Token") != "fake-installer-test-token" {
			t.Error("missing shutdown token")
		}
		_ = json.NewEncoder(w).Encode(M{"ok": true})
		go server.Close()
	})
	server.Handler = mux
	fmt.Println("installer-mock-ready")
	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}

func TestInstallerUpgradeAllowsOnlyItsOwnBackend(t *testing.T) {
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(root, "Mirroxy.exe")
	if err = os.WriteFile(installed, payload, 0700); err != nil {
		t.Fatal(err)
	}
	listener := installerTestListener(t)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	a := &App{root: root, cfg: Settings{PanelPort: port}}
	if err = a.save(); err != nil {
		t.Fatal(err)
	}
	// Run only a mock server in a child, never the production startup path.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, installed, "-test.run=^TestInstallerMockBackendProcess$")
	child.Env = append(os.Environ(), "MIRROXY_INSTALLER_MOCK_PORT="+strconv.Itoa(port))
	hideCommand(child)
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if child.ProcessState == nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "installer-mock-ready" {
		t.Fatal("mock backend did not become ready")
	}
	pid, path := installerPanelPortOwner(port)
	if pid != child.Process.Pid || path == "" {
		t.Fatalf("wrong socket owner: pid=%d, path=%q", pid, path)
	}
	if err = installerPanelPort(root, "check-upgrade", port); err != nil {
		t.Fatalf("own running backend blocked wizard: %v", err)
	}
	if err = installerPanelPort(root, "check", port); err == nil {
		t.Fatal("final check accepted backend before it exited")
	}
	if err = installerPanelPort(root, "stop", port); err != nil {
		t.Fatal(err)
	}
	if err = child.Wait(); err != nil {
		t.Fatal(err)
	}
	if err = installerPanelPort(root, "check", port); err != nil {
		t.Fatalf("port not released after graceful exit: %v", err)
	}
}
