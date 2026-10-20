package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func checkReady(port int) error {
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "localhost"}}}
	var r *http.Response
	var err error
	for _, scheme := range []string{"https", "http"} {
		r, err = client.Get(fmt.Sprintf("%s://127.0.0.1:%d/api/v1/status", scheme, port))
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	defer r.Body.Close()
	var state struct {
		Version string `json:"version"`
		Remote  struct {
			Running bool `json:"running"`
		} `json:"remote"`
	}
	if r.StatusCode != 200 {
		return errors.New("后台尚未就绪")
	}
	if err = json.NewDecoder(r.Body).Decode(&state); err != nil {
		return err
	}
	if state.Version != version || !state.Remote.Running {
		return errors.New("管理面板尚未就绪")
	}
	fmt.Println("Mirroxy " + version + " ready")
	return nil
}

func (a *App) initializeServer(host string, remotePort int, reset bool) error {
	if strings.ContainsAny(host, "/\r\n\t @") {
		return errors.New("--public-host 请填写 IP 或域名，不带 http://、路径或端口")
	}
	if remotePort < 1024 || remotePort > 65000 || remotePort == a.panelPort {
		return errors.New("管理端口必须在 1024–65000 且不能与本机 API 端口相同")
	}
	// Refuse offline configuration changes while a server is using that config.
	l, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", a.panelPort))
	if err != nil {
		return errors.New("请先停止 Mirroxy 服务，再初始化或重置密码")
	}
	defer l.Close()
	if err = a.load(); err != nil {
		return err
	}
	if a.cfg.Remote.PasswordHash != "" && !reset {
		return a.printAccessInfo()
	}
	password := newID() + newID()[:8]
	username := "admin"
	if a.cfg.Remote.Username != "" {
		username = a.cfg.Remote.Username
	}
	existing := a.cfg.Remote.PasswordHash != ""
	if !existing {
		a.cfg.Remote = RemoteSettings{Enabled: true, Scheme: "http", Port: remotePort, Username: username}
	}
	a.cfg.Remote.Salt = newID() + newID()
	remotePort = a.cfg.Remote.Port
	a.cfg.Remote.PasswordHash = passwordHash(password, a.cfg.Remote.Salt)
	if !existing {
		a.cfg.OpenBrowser, a.cfg.ResumeNodes = false, true
		a.cfg.AutoDownload, a.cfg.AutoDeploy = true, true
		a.cfg.SystemProxyMode, a.cfg.SystemProxyNode = "unchanged", ""
		if serverPlatform() {
			a.cfg.Autostart = true
		}
	}
	if err = a.save(); err != nil {
		return err
	}
	addresses := []string{}
	if host != "" {
		if strings.ContainsAny(host, "/\r\n\t @") {
			return errors.New("--public-host 请填写 IP 或域名，不带 http://、路径或端口")
		}
		addresses = append(addresses, host)
	} else {
		ifaces, _ := net.InterfaceAddrs()
		for _, address := range ifaces {
			ip, _, e := net.ParseCIDR(address.String())
			if e == nil && ip.To4() != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				addresses = append(addresses, ip.String())
			}
		}
	}
	if len(addresses) == 0 {
		addresses = []string{"127.0.0.1"}
	}
	info := "Mirroxy " + version + "\n"
	for _, address := range addresses {
		info += "面板地址：" + first(a.cfg.Remote.Scheme, "https") + "://" + net.JoinHostPort(address, fmt.Sprint(remotePort)) + "/\n"
	}
	info += "账号：" + username + "\n密码：" + password + "\n\n节点端口从 5060 起，HTTP / SOCKS5 均可使用。\n本机控制台示例：curl -x http://127.0.0.1:5060 https://example.com\n管理面板需放行 TCP " + fmt.Sprint(remotePort) + "；节点代理端口仅监听本机。\n"
	if err = a.writeState("access", info); err != nil {
		return err
	}
	fmt.Print(info)
	return nil
}

func (a *App) printAccessInfo() error {
	var info string
	err := a.readState("access", &info)
	if errors.Is(err, errStateMissing) {
		fmt.Println("配置已存在，保持原账号密码。遗忘时运行 sudo mirroxyctl password。")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Print(info)
	return nil
}

func (a *App) checkInstallation() error {
	if err := a.load(); err != nil {
		return err
	}
	for _, engine := range []string{"xray", "sing-box"} {
		v := coreVersion(a.corePath(engine), engine)
		if v == "未检测到" || v == "未知" {
			return fmt.Errorf("%s 不可执行，请确认操作系统和 CPU 架构匹配", coreDisplayName(engine))
		}
		cfg, _ := a.coreConfig(engine)
		suffix := "A"
		if coreEngine(engine) == "sing-box" {
			suffix = "B"
		}
		path := a.dataPath(filepath.Join("runtime", "Mirroxy_install_"+suffix+".json"))
		if err := writeJSON(path, cfg); err != nil {
			return err
		}
		err := a.validateCore(engine, path, a.corePath(engine))
		_ = os.Remove(path)
		if err != nil {
			return err
		}
		fmt.Println(coreDisplayName(engine) + " " + v + " OK")
	}
	return nil
}
