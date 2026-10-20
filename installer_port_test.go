package main

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func installerTestListener(t *testing.T) net.Listener {
	t.Helper()
	for i := 0; i < 50; i++ {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		if listener.Addr().(*net.TCPAddr).Port <= 65000 {
			t.Cleanup(func() { listener.Close() })
			return listener
		}
		listener.Close()
	}
	t.Fatal("could not allocate test port below 65001")
	return nil
}

func TestInstallerProbeNeverCreatesData(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-installed")
	if err := installerPanelPort(root, "read", 0); err != nil {
		t.Fatal(err)
	}
	listener := installerTestListener(t)
	port := listener.Addr().(*net.TCPAddr).Port
	if err := installerPanelPort(root, "check", port); err == nil || !strings.Contains(err.Error(), "更换面板端口") {
		t.Fatalf("occupied port was not rejected: %v", err)
	}
	if err := installerPanelPort(root, "write", port); err == nil {
		t.Fatal("write accepted occupied port")
	}
	if err := installerPanelPort(root, "check-upgrade", port); err == nil {
		t.Fatal("upgrade probe accepted another process's occupied port")
	}
	listener.Close()
	if err := installerPanelPort(root, "check", port); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("probe wrote to installation directory: %v", err)
	}
}

func TestInstallerPortPreservesConfigurationAndTraffic(t *testing.T) {
	root := filepath.Join(t.TempDir(), "existing #安装")
	a := &App{root: root}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}
	a.cfg.PanelPort = 9528
	a.cfg.Nodes = []Node{{ID: "test", Name: "custom-name", Port: 5100, AppPort: 25100, Outbound: M{"password": "fake-password"}}}
	a.cfg.Subscriptions = []Subscription{{ID: "subscription", URL: "https://example.invalid/test"}}
	if err := a.save(); err != nil {
		t.Fatal(err)
	}
	if err := a.writeState("traffic", M{"test": M{"upload": 12345, "download": 98765}}); err != nil {
		t.Fatal(err)
	}
	before := cloneSettings(a.cfg)
	keyBefore, _ := os.ReadFile(a.dataPath("Mirroxy.key"))
	dbBefore, _ := os.ReadFile(a.dataPath("Mirroxy.db"))
	cfg, exists, err := installerConfiguration(root)
	if err != nil || !exists || !reflect.DeepEqual(cfg, before) {
		t.Fatalf("read-only configuration probe failed: exists=%v, err=%v", exists, err)
	}
	dbAfterRead, _ := os.ReadFile(a.dataPath("Mirroxy.db"))
	if !bytes.Equal(dbBefore, dbAfterRead) {
		t.Fatal("configuration probe changed database")
	}
	listener := installerTestListener(t)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	if err := installerPanelPort(root, "write", port); err != nil {
		t.Fatal(err)
	}
	cfg, _, err = installerConfiguration(root)
	before.PanelPort = port
	if err != nil || !reflect.DeepEqual(cfg, before) {
		t.Fatalf("port change modified other settings: %v", err)
	}
	var traffic M
	if err := a.readState("traffic", &traffic); err != nil || traffic["test"].(map[string]any)["upload"] != float64(12345) {
		t.Fatalf("traffic changed: %v, %v", traffic, err)
	}
	keyAfter, _ := os.ReadFile(a.dataPath("Mirroxy.key"))
	if !bytes.Equal(keyBefore, keyAfter) {
		t.Fatal("port change replaced encryption key")
	}
}

func TestInstallerPortValidationAndNewInstall(t *testing.T) {
	cfg := Settings{Remote: RemoteSettings{Enabled: true, Port: 15061}, Nodes: []Node{{Port: 5060, AppPort: 25060}}}
	for _, port := range []int{0, 1023, 65001, 5060, 25060, 15061} {
		if err := validateInstallerPanelPort(cfg, port); err == nil {
			t.Fatalf("invalid or conflicting port accepted: %d", port)
		}
	}
	root := t.TempDir()
	listener := installerTestListener(t)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	if err := installerPanelPort(root, "write", port); err != nil {
		t.Fatal(err)
	}
	a := &App{root: root}
	if err := a.load(); err != nil || a.configuredPanelPort() != port {
		t.Fatalf("new installation forgot selected port: %v", err)
	}
	if a.cfg.SystemProxyMode != "unchanged" || len(a.cfg.Nodes) != 0 {
		t.Fatalf("unexpected initial configuration: %+v", a.cfg)
	}
}

func TestInstallerBrokenExistingDataFailsWithoutWrites(t *testing.T) {
	root := t.TempDir()
	a := &App{root: root}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}
	keyPath := a.dataPath("Mirroxy.key")
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(a.dataPath("Mirroxy.db"))
	if _, _, err := installerConfiguration(root); err == nil {
		t.Fatal("missing encryption key was ignored")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatal("probe created replacement encryption key")
	}
	after, _ := os.ReadFile(a.dataPath("Mirroxy.db"))
	if !bytes.Equal(before, after) {
		t.Fatal("failed probe changed database")
	}
}
