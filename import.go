package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Spec struct {
	Name, Protocol, Server, Password, Username, Method, Flow, Network, Security, SNI, Fingerprint, PublicKey, ShortID, Path, Host, ServiceName, ALPN, Encryption string
	Port, AlterID                                                                                                                                                int
	Insecure                                                                                                                                                     bool
	Extra                                                                                                                                                        M
}

func stringVal(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}
func intVal(v any) int { i, _ := strconv.Atoi(stringVal(v)); return i }
func first(values ...string) string {
	for _, s := range values {
		if s != "" {
			return s
		}
	}
	return ""
}
func b64(s string) (string, error) {
	for _, e := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		if b, err := e.DecodeString(strings.TrimSpace(s)); err == nil {
			return string(b), nil
		}
	}
	return "", errors.New("Base64 格式不正确")
}
func parseLinks(text string) ([]Node, []string) {
	text = strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	if !strings.Contains(text, "://") && !strings.HasPrefix(text, "{") {
		if s, e := b64(text); e == nil {
			text = s
		}
	}
	var nodes []Node
	var warnings []string
	for i, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		n, err := parseLink(line)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("第 %d 行：%s", i+1, err))
		} else {
			nodes = append(nodes, n)
		}
	}
	return nodes, warnings
}
func parseLink(link string) (Node, error) {
	var s Spec
	if strings.HasPrefix(link, "vmess://") {
		raw, err := b64(strings.TrimPrefix(link, "vmess://"))
		if err != nil {
			return Node{}, err
		}
		var m M
		if json.Unmarshal([]byte(raw), &m) != nil {
			return Node{}, errors.New("VMess JSON 无效")
		}
		s = Spec{Name: stringVal(m["ps"]), Protocol: "vmess", Server: stringVal(m["add"]), Port: intVal(m["port"]), Password: stringVal(m["id"]), AlterID: intVal(m["aid"]), Method: first(stringVal(m["scy"]), "auto"), Network: stringVal(m["net"]), Security: stringVal(m["tls"]), SNI: stringVal(m["sni"]), Path: stringVal(m["path"]), Host: stringVal(m["host"]), Fingerprint: stringVal(m["fp"]), ALPN: stringVal(m["alpn"])}
		if s.Network == "grpc" {
			s.ServiceName = s.Path
		}
		s.Insecure = strings.EqualFold(stringVal(m["allowInsecure"]), "true") || stringVal(m["allowInsecure"]) == "1"
		return makeNode(s)
	}
	if strings.HasPrefix(link, "ss://") {
		body := strings.TrimPrefix(link, "ss://")
		fragment := ""
		if p := strings.IndexByte(body, '#'); p >= 0 {
			fragment, _ = url.PathUnescape(body[p+1:])
			body = body[:p]
		}
		if !strings.Contains(body, "@") {
			decoded, err := b64(body)
			if err != nil {
				return Node{}, err
			}
			body = decoded
		}
		u, err := url.Parse("ss://" + body)
		if err != nil || u.User == nil {
			return Node{}, errors.New("Shadowsocks 链接无效")
		}
		method := u.User.Username()
		password, hasPassword := u.User.Password()
		if !hasPassword {
			decoded, e := b64(method)
			if e != nil {
				return Node{}, e
			}
			method, password, hasPassword = strings.Cut(decoded, ":")
			if !hasPassword {
				return Node{}, errors.New("Shadowsocks 密码缺失")
			}
		}
		if u.Query().Get("plugin") != "" {
			return Node{}, errors.New("第一版不支持 Shadowsocks 外部插件；请使用原生 SS 节点")
		}
		port, _ := strconv.Atoi(u.Port())
		return makeNode(Spec{Name: fragment, Protocol: "shadowsocks", Server: u.Hostname(), Port: port, Password: password, Method: method})
	}
	u, err := url.Parse(link)
	if err != nil {
		return Node{}, errors.New("分享链接无法解析")
	}
	q := u.Query()
	port, _ := strconv.Atoi(u.Port())
	s = Spec{Name: u.Fragment, Protocol: strings.ToLower(u.Scheme), Server: u.Hostname(), Port: port, Network: q.Get("type"), Security: q.Get("security"), SNI: first(q.Get("sni"), q.Get("peer")), Fingerprint: q.Get("fp"), PublicKey: q.Get("pbk"), ShortID: q.Get("sid"), Path: q.Get("path"), Host: q.Get("host"), ServiceName: q.Get("serviceName"), Flow: q.Get("flow"), ALPN: q.Get("alpn"), Encryption: q.Get("encryption"), Insecure: q.Get("allowInsecure") == "1" || q.Get("insecure") == "1", Extra: M{}}
	if u.User != nil {
		s.Password = u.User.Username()
		if p, ok := u.User.Password(); ok {
			s.Username = u.User.Username()
			s.Password = p
		}
	}
	switch s.Protocol {
	case "vless", "trojan":
		if s.Protocol == "trojan" && s.Security == "" {
			s.Security = "tls"
		}
	case "socks", "socks5":
		s.Protocol = "socks"
		if s.Username == "" {
			s.Username = s.Password
			s.Password = ""
		}
	case "http", "https":
		if s.Port == 0 {
			if s.Protocol == "https" {
				s.Port = 443
			} else {
				s.Port = 80
			}
		}
		if s.Protocol == "https" {
			s.Security = "tls"
		}
		s.Protocol = "http"
		if s.Username == "" {
			s.Username = s.Password
			s.Password = ""
		}
	case "hysteria2", "hy2":
		s.Protocol = "hysteria2"
		s.Extra["obfs"] = q.Get("obfs")
		s.Extra["obfs_password"] = q.Get("obfs-password")
	case "tuic":
		s.Extra["congestion_control"] = q.Get("congestion_control")
		s.Extra["udp_relay_mode"] = q.Get("udp_relay_mode")
	case "anytls":
	default:
		return Node{}, errors.New("暂不支持此分享协议；可导入 VMess / VLESS / SS / Trojan / SOCKS / HTTP / Hysteria2 / TUIC / AnyTLS")
	}
	return makeNode(s)
}

func makeNode(s Spec) (Node, error) {
	if s.Server == "" || s.Port < 1 || s.Port > 65535 {
		return Node{}, errors.New("服务器地址或端口无效")
	}
	if s.Protocol != "socks" && s.Protocol != "http" && s.Password == "" {
		return Node{}, errors.New("节点认证信息为空")
	}
	if s.Name == "" {
		s.Name = s.Protocol + " · " + s.Server
	}
	n := Node{Name: s.Name, Protocol: s.Protocol, Server: s.Server, Engine: "xray", Route: "proxy"}
	if s.Protocol == "hysteria2" || s.Protocol == "tuic" || s.Protocol == "anytls" {
		n.Engine = "sing-box"
		o := M{"type": s.Protocol, "server": s.Server, "server_port": s.Port, "password": s.Password}
		tls := M{"enabled": true, "server_name": first(s.SNI, s.Server), "insecure": s.Insecure}
		if s.ALPN != "" {
			tls["alpn"] = strings.Split(s.ALPN, ",")
		}
		o["tls"] = tls
		if s.Protocol == "tuic" {
			o["uuid"] = s.Username
			if s.Username == "" {
				return Node{}, errors.New("TUIC UUID 缺失")
			}
			if v := stringVal(s.Extra["congestion_control"]); v != "" {
				o["congestion_control"] = v
			}
			if v := stringVal(s.Extra["udp_relay_mode"]); v != "" {
				o["udp_relay_mode"] = v
			}
		}
		if s.Protocol == "hysteria2" && stringVal(s.Extra["obfs"]) != "" {
			o["obfs"] = M{"type": s.Extra["obfs"], "password": s.Extra["obfs_password"]}
		}
		n.Outbound = o
		return n, nil
	}
	o := M{"protocol": s.Protocol}
	server := M{"address": s.Server, "port": s.Port}
	settings := M{}
	switch s.Protocol {
	case "vless", "vmess":
		user := M{"id": s.Password}
		if s.Protocol == "vless" {
			user["encryption"] = first(s.Encryption, "none")
			if s.Flow != "" {
				user["flow"] = s.Flow
			}
		} else {
			user["security"] = first(s.Method, "auto")
			user["alterId"] = s.AlterID
		}
		server["users"] = []any{user}
		settings["vnext"] = []any{server}
	case "shadowsocks":
		server["method"] = s.Method
		server["password"] = s.Password
		settings["servers"] = []any{server}
	case "trojan":
		server["password"] = s.Password
		settings["servers"] = []any{server}
	case "socks", "http":
		if s.Username != "" {
			server["users"] = []any{M{"user": s.Username, "pass": s.Password}}
		}
		settings["servers"] = []any{server}
	default:
		return Node{}, errors.New("当前节点协议尚未支持")
	}
	o["settings"] = settings
	network := first(s.Network, "raw")
	if network == "tcp" {
		network = "raw"
	}
	stream := M{"network": network}
	switch network {
	case "raw":
	case "ws":
		v := M{"path": first(s.Path, "/")}
		if s.Host != "" {
			v["headers"] = M{"Host": s.Host}
		}
		stream["wsSettings"] = v
	case "grpc":
		stream["grpcSettings"] = M{"serviceName": first(s.ServiceName, s.Path)}
	case "httpupgrade":
		stream["httpupgradeSettings"] = M{"path": first(s.Path, "/"), "host": s.Host}
	case "xhttp":
		stream["xhttpSettings"] = M{"path": first(s.Path, "/"), "host": s.Host, "mode": "auto"}
	default:
		return Node{}, fmt.Errorf("第一版尚未支持传输方式 %s", network)
	}
	if s.Security == "reality" {
		if s.PublicKey == "" {
			return Node{}, errors.New("REALITY 公钥缺失")
		}
		stream["security"] = "reality"
		stream["realitySettings"] = M{"serverName": s.SNI, "fingerprint": first(s.Fingerprint, "chrome"), "publicKey": s.PublicKey, "shortId": s.ShortID}
	} else if s.Security == "tls" {
		stream["security"] = "tls"
		t := M{"serverName": first(s.SNI, s.Server), "allowInsecure": s.Insecure}
		if s.Fingerprint != "" {
			t["fingerprint"] = s.Fingerprint
		}
		if s.ALPN != "" {
			t["alpn"] = strings.Split(s.ALPN, ",")
		}
		stream["tlsSettings"] = t
	} else if s.Security != "" && s.Security != "none" {
		return Node{}, fmt.Errorf("不支持安全传输方式 %s", s.Security)
	}
	o["streamSettings"] = stream
	n.Outbound = o
	return n, nil
}

func importV2rayN(path string) ([]Node, []string, error) {
	if strings.EqualFold(filepath.Ext(path), ".exe") {
		path = filepath.Dir(path)
	}
	if !strings.EqualFold(filepath.Ext(path), ".db") {
		path = filepath.Join(path, "guiConfigs", "guiNDB.db")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, nil, errors.New("找不到 guiConfigs/guiNDB.db，请填写 v2rayN 的安装目录")
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	u.RawQuery = "mode=ro"
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	rows, err := db.Query("SELECT * FROM ProfileItem")
	if err != nil {
		return nil, nil, fmt.Errorf("读取 v2rayN 节点失败: %w", err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var nodes []Node
	var warnings []string
	protocols := map[int]string{1: "vmess", 3: "shadowsocks", 4: "socks", 5: "vless", 6: "trojan", 7: "hysteria2", 8: "tuic", 10: "http", 11: "anytls"}
	for rows.Next() {
		values := make([]any, len(cols))
		dest := make([]any, len(cols))
		for i := range values {
			dest[i] = &values[i]
		}
		if err = rows.Scan(dest...); err != nil {
			return nil, nil, err
		}
		m := M{}
		for i, k := range cols {
			m[k] = values[i]
		}
		name := stringVal(m["Remarks"])
		proto := protocols[intVal(m["ConfigType"])]
		if proto == "" {
			warnings = append(warnings, name+"：暂不支持这种配置类型")
			continue
		}
		extra := M{}
		_ = json.Unmarshal([]byte(stringVal(m["ProtoExtra"])), &extra)
		s := Spec{Name: name, Protocol: proto, Server: stringVal(m["Address"]), Port: intVal(m["Port"]), Password: stringVal(m["Password"]), Username: stringVal(m["Username"]), Method: first(stringVal(extra["SsMethod"]), stringVal(extra["VmessSecurity"]), stringVal(m["Security"])), Flow: first(stringVal(extra["Flow"]), stringVal(m["Flow"])), Encryption: stringVal(extra["VlessEncryption"]), AlterID: intVal(extra["AlterId"]), Network: stringVal(m["Network"]), Security: stringVal(m["StreamSecurity"]), SNI: stringVal(m["Sni"]), Fingerprint: stringVal(m["Fingerprint"]), PublicKey: stringVal(m["PublicKey"]), ShortID: stringVal(m["ShortId"]), Path: stringVal(m["Path"]), Host: stringVal(m["RequestHost"]), ALPN: stringVal(m["Alpn"]), Insecure: strings.EqualFold(stringVal(m["AllowInsecure"]), "true"), Extra: M{}}
		if proto == "tuic" {
			s.Username = first(stringVal(extra["TuicUuid"]), s.Username)
		}
		n, e := makeNode(s)
		if e != nil {
			warnings = append(warnings, name+"："+e.Error())
			continue
		}
		nodes = append(nodes, n)
	}
	return nodes, warnings, rows.Err()
}
