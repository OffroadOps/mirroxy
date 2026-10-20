package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Reconstruct from the persisted outbound so nodes imported before sharing was
// added work too. Node.Port is the local listener, never the remote endpoint.
func shareLink(n Node) (string, error) {
	fail := errors.New("此节点含有无法完整导出的配置，暂不能生成分享链接")
	b, err := json.Marshal(n.Outbound)
	if err != nil {
		return "", fail
	}
	var o M
	if json.Unmarshal(b, &o) != nil {
		return "", fail
	}
	obj := func(v any) M { m, _ := v.(map[string]any); return m }
	firstObject := func(v any) M {
		items, _ := v.([]any)
		if len(items) != 1 {
			return nil
		}
		return obj(items[0])
	}
	list := func(v any) string {
		items, _ := v.([]any)
		var result []string
		for _, item := range items {
			result = append(result, stringVal(item))
		}
		return strings.Join(result, ",")
	}
	s := Spec{Name: n.Name, Protocol: n.Protocol, Extra: M{}}
	if n.Engine == "sing-box" {
		s.Server, s.Port = stringVal(o["server"]), intVal(o["server_port"])
		s.Password, s.Username = stringVal(o["password"]), stringVal(o["uuid"])
		tls := obj(o["tls"])
		s.SNI, s.ALPN = stringVal(tls["server_name"]), list(tls["alpn"])
		s.Insecure, _ = tls["insecure"].(bool)
		obfs := obj(o["obfs"])
		s.Extra = M{"obfs": obfs["type"], "obfs_password": obfs["password"], "congestion_control": o["congestion_control"], "udp_relay_mode": o["udp_relay_mode"]}
	} else {
		settings := obj(o["settings"])
		server := firstObject(settings["servers"])
		if n.Protocol == "vless" || n.Protocol == "vmess" {
			server = firstObject(settings["vnext"])
		}
		s.Server, s.Port = stringVal(server["address"]), intVal(server["port"])
		s.Password, s.Method = stringVal(server["password"]), stringVal(server["method"])
		user := firstObject(server["users"])
		switch n.Protocol {
		case "vless", "vmess":
			s.Password = stringVal(user["id"])
			s.Flow, s.Encryption = stringVal(user["flow"]), stringVal(user["encryption"])
			s.AlterID = intVal(user["alterId"])
			s.Method = stringVal(user["security"])
		case "http", "socks":
			s.Username, s.Password = stringVal(user["user"]), stringVal(user["pass"])
		}
		stream := obj(o["streamSettings"])
		s.Network, s.Security = stringVal(stream["network"]), stringVal(stream["security"])
		transport := obj(stream[s.Network+"Settings"])
		s.Path, s.Host = stringVal(transport["path"]), stringVal(transport["host"])
		if s.Network == "ws" {
			s.Host = stringVal(obj(transport["headers"])["Host"])
		}
		s.ServiceName = stringVal(transport["serviceName"])
		security := obj(stream[s.Security+"Settings"])
		s.SNI, s.Fingerprint = stringVal(security["serverName"]), stringVal(security["fingerprint"])
		s.PublicKey, s.ShortID = stringVal(security["publicKey"]), stringVal(security["shortId"])
		s.ALPN = list(security["alpn"])
		s.Insecure, _ = security["allowInsecure"].(bool)
	}
	// Reject lossy exports (including manually added future core options).
	check, err := makeNode(s)
	if err != nil {
		return "", fail
	}
	rebuilt, err := json.Marshal(check.Outbound)
	if err != nil || !bytes.Equal(b, rebuilt) {
		return "", fail
	}
	var link string
	if s.Protocol == "vmess" {
		network := s.Network
		if network == "raw" {
			network = "tcp"
		}
		path := s.Path
		if network == "grpc" {
			path = s.ServiceName
		}
		v := M{"v": "2", "ps": s.Name, "add": s.Server, "port": strconv.Itoa(s.Port), "id": s.Password, "aid": strconv.Itoa(s.AlterID), "scy": s.Method, "net": network, "type": "none", "host": s.Host, "path": path, "tls": s.Security, "sni": s.SNI, "fp": s.Fingerprint, "alpn": s.ALPN}
		if s.Insecure {
			v["allowInsecure"] = true
		}
		data, _ := json.Marshal(v)
		link = "vmess://" + base64.StdEncoding.EncodeToString(data)
	} else if s.Protocol == "shadowsocks" {
		link = "ss://" + base64.RawURLEncoding.EncodeToString([]byte(s.Method+":"+s.Password)) + "@" + net.JoinHostPort(s.Server, strconv.Itoa(s.Port))
		if s.Name != "" {
			link += "#" + url.PathEscape(s.Name)
		}
	} else {
		u := url.URL{Scheme: s.Protocol, Host: net.JoinHostPort(s.Server, strconv.Itoa(s.Port)), Fragment: s.Name}
		if s.Protocol == "socks" || s.Protocol == "http" || s.Protocol == "tuic" {
			if s.Username != "" {
				u.User = url.UserPassword(s.Username, s.Password)
			}
		} else {
			u.User = url.User(s.Password)
		}
		if s.Protocol == "http" && s.Security == "tls" {
			u.Scheme = "https"
		}
		q := url.Values{}
		set := func(key, value string) {
			if value != "" {
				q.Set(key, value)
			}
		}
		network := s.Network
		if network == "raw" {
			network = "tcp"
		}
		set("type", network)
		set("security", s.Security)
		set("sni", s.SNI)
		set("fp", s.Fingerprint)
		set("pbk", s.PublicKey)
		set("sid", s.ShortID)
		set("flow", s.Flow)
		set("encryption", s.Encryption)
		set("alpn", s.ALPN)
		set("path", s.Path)
		set("host", s.Host)
		set("serviceName", s.ServiceName)
		if s.Insecure {
			q.Set("allowInsecure", "1")
			q.Set("insecure", "1")
		}
		set("obfs-password", stringVal(s.Extra["obfs_password"]))
		for _, key := range []string{"obfs", "congestion_control", "udp_relay_mode"} {
			set(key, stringVal(s.Extra[key]))
		}
		u.RawQuery = q.Encode()
		link = u.String()
	}
	// Verify the URI itself preserves every connection option too.
	roundTrip, err := parseLink(link)
	if err != nil {
		return "", fail
	}
	encoded, _ := json.Marshal(roundTrip.Outbound)
	if !bytes.Equal(b, encoded) || roundTrip.Name != n.Name {
		return "", fail
	}
	return link, nil
}
