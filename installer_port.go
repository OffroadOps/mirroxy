package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// The installer probes before copying files. Reading an existing installation
// must not create a database, migrate settings, or change a running backend.
func installerConfiguration(root string) (Settings, bool, error) {
	var cfg Settings
	dbPath := filepath.Join(root, "data", "Mirroxy.db")
	if _, err := os.Stat(dbPath); err == nil {
		key, err := readExistingDataKey(filepath.Join(root, "data", "Mirroxy.key"))
		if err != nil {
			return cfg, false, err
		}
		uri := "file:" + strings.ReplaceAll(url.PathEscape(filepath.ToSlash(dbPath)), "%2F", "/") + "?mode=ro"
		db, err := sql.Open("sqlite", uri)
		if err != nil {
			return cfg, false, err
		}
		defer db.Close()
		var nonce, payload []byte
		err = db.QueryRow(`SELECT nonce,payload FROM state WHERE name='configuration'`).Scan(&nonce, &payload)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return cfg, false, err
		}
		if err == nil {
			store := &stateStore{key: key}
			plain, err := store.open(nonce, payload)
			if err != nil {
				return cfg, false, err
			}
			err = json.Unmarshal(plain, &cfg)
			return cfg, true, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return cfg, false, err
	}
	legacy, err := os.ReadFile(filepath.Join(root, "data", "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, false, nil
	}
	if err != nil {
		return cfg, false, err
	}
	err = json.Unmarshal(legacy, &cfg)
	return cfg, true, err
}

func validateInstallerPanelPort(cfg Settings, port int) error {
	if port < 1024 || port > 65000 {
		return errors.New("面板端口必须在 1024–65000")
	}
	if cfg.Remote.Enabled && cfg.Remote.Port == port {
		return fmt.Errorf("面板端口 %d 与远程面板端口冲突，请更换端口", port)
	}
	for _, node := range cfg.Nodes {
		if node.Port == port || node.AppPort == port {
			return fmt.Errorf("面板端口 %d 与已有节点端口冲突，请更换端口", port)
		}
	}
	return nil
}

func installerPanelPort(root, operation string, port int) error {
	if operation != "read" && operation != "check" && operation != "check-upgrade" && operation != "stop" && operation != "write" {
		return errors.New("无效安装器端口操作")
	}
	cfg, exists, err := installerConfiguration(root)
	if err != nil {
		return fmt.Errorf("无法读取已有面板设置，已保留原数据：%w", err)
	}
	a := &App{root: root, cfg: cfg}
	if operation == "read" {
		fmt.Println(a.configuredPanelPort())
		return nil
	}
	if operation == "stop" {
		for _, currentPort := range []int{a.configuredPanelPort(), legacyPanelPort} {
			if installerOwnsPanelPort(root, currentPort) {
				if err := shutdownExisting(currentPort); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err = validateInstallerPanelPort(cfg, port); err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		if operation == "check-upgrade" && port == a.configuredPanelPort() && installerOwnsPanelPort(root, port) {
			return nil // This exact installed executable will exit before copying.
		}
		owner := ""
		if pid, path := installerPanelPortOwner(port); pid != 0 {
			owner = fmt.Sprintf("当前占用程序：%s（PID %d）。", filepath.Base(path), pid)
		}
		return fmt.Errorf("无法使用面板端口 %d。%s请关闭占用该端口的程序，或更换面板端口后重试：%w", port, owner, err)
	}
	defer listener.Close()
	if operation == "check" || operation == "check-upgrade" {
		return nil
	}
	// Only write after the installed backend has exited. Preserve every other
	// configuration field and leave traffic, credentials and proxy state alone.
	if !exists {
		if err = a.load(); err != nil {
			return err
		}
	} else if a.configuredPanelPort() == port {
		return nil
	}
	a.cfg.PanelPort = port
	return a.save()
}

func installerOwnsPanelPort(root string, port int) bool {
	_, path := installerPanelPortOwner(port)
	if path == "" {
		return false
	}
	owner, err := os.Stat(path)
	if err != nil {
		return false
	}
	installed, err := os.Stat(filepath.Join(root, executableName("Mirroxy")))
	return err == nil && os.SameFile(owner, installed)
}
