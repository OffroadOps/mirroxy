package main

import (
	"context"
	"crypto/rand"
	"net"
	"runtime"
	"time"
)

type ServerCapabilities struct {
	CheckedAt       time.Time `json:"checkedAt"`
	OS              string    `json:"os"`
	Arch            string    `json:"arch"`
	TCPv4           bool      `json:"tcpV4"`
	UDPv4           bool      `json:"udpV4"`
	IPv6            bool      `json:"ipv6"`
	NAT             string    `json:"nat"`
	ExternalAddress string    `json:"externalAddress,omitempty"`
	CoreA           string    `json:"coreA,omitempty"`
	CoreB           string    `json:"coreB,omitempty"`
	TCPError        string    `json:"tcpError,omitempty"`
	UDPError        string    `json:"udpError,omitempty"`
	IPv6Error       string    `json:"ipv6Error,omitempty"`
	Notice          string    `json:"notice"`
}

func probeServerCapabilities(root string) ServerCapabilities {
	result := ServerCapabilities{
		CheckedAt: time.Now().UTC(), OS: runtime.GOOS, Arch: runtime.GOARCH, NAT: "unknown",
		Notice: "主机探测只说明服务器出口；具体节点是否支持 UDP/IPv6 仍需从客户端穿过该节点复测。",
	}
	a := &App{root: root}
	result.CoreA = coreVersion(a.corePath("xray"), "xray")
	result.CoreB = coreVersion(a.corePath("sing-box"), "sing-box")
	if conn, err := net.DialTimeout("tcp4", "turn.cloudflare.com:80", 5*time.Second); err == nil {
		result.TCPv4 = true
		_ = conn.Close()
	} else {
		result.TCPError = shortCapabilityError(err)
	}
	if conn, err := net.DialTimeout("tcp6", "[2606:4700:4700::1111]:443", 5*time.Second); err == nil {
		result.IPv6 = true
		_ = conn.Close()
	} else {
		result.IPv6Error = shortCapabilityError(err)
	}
	first, second, err := directSTUNMappings("stun.cloudflare.com", 3478, 53)
	if err == nil {
		result.UDPv4 = true
		result.ExternalAddress = first
		if second != "" {
			if first == second {
				result.NAT = "endpoint-independent"
			} else {
				result.NAT = "symmetric-likely"
			}
		}
	} else {
		result.UDPError = shortCapabilityError(err)
	}
	return result
}

func directSTUNMappings(host string, firstPort, secondPort int) (string, string, error) {
	addresses, err := net.DefaultResolver.LookupIPAddr(context.Background(), host)
	if err != nil {
		return "", "", err
	}
	var target net.IP
	for _, address := range addresses {
		if ipv4 := address.IP.To4(); ipv4 != nil {
			target = ipv4
			break
		}
	}
	if target == nil {
		return "", "", &net.AddrError{Err: "没有 IPv4 地址", Addr: host}
	}
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return "", "", err
	}
	defer conn.Close()
	first, err := directSTUNMapping(conn, &net.UDPAddr{IP: target, Port: firstPort})
	if err != nil {
		return "", "", err
	}
	second, _ := directSTUNMapping(conn, &net.UDPAddr{IP: target, Port: secondPort})
	return first, second, nil
}

func directSTUNMapping(conn *net.UDPConn, target *net.UDPAddr) (string, error) {
	transaction := make([]byte, 12)
	if _, err := rand.Read(transaction); err != nil {
		return "", err
	}
	request := make([]byte, 20)
	request[0], request[1] = 0, 1
	request[4], request[5], request[6], request[7] = 0x21, 0x12, 0xa4, 0x42
	copy(request[8:], transaction)
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	if _, err := conn.WriteToUDP(request, target); err != nil {
		return "", err
	}
	response := make([]byte, 2048)
	n, _, err := conn.ReadFromUDP(response)
	if err != nil {
		return "", err
	}
	return parseSTUNMapping(response[:n], transaction)
}
