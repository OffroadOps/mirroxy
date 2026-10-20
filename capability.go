package main

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

type NodeCapabilities struct {
	CheckedAt       time.Time `json:"checkedAt"`
	TCP             bool      `json:"tcp"`
	UDP             bool      `json:"udp"`
	IPv6            bool      `json:"ipv6"`
	NAT             string    `json:"nat"`
	ExternalAddress string    `json:"externalAddress,omitempty"`
	LatencyMS       int64     `json:"latencyMs,omitempty"`
	UDPError        string    `json:"udpError,omitempty"`
	IPv6Error       string    `json:"ipv6Error,omitempty"`
}

func (a *App) probeNodeCapabilities(id string) (NodeCapabilities, error) {
	a.mu.Lock()
	n := a.node(id)
	if n == nil {
		a.mu.Unlock()
		return NodeCapabilities{}, errors.New("节点不存在")
	}
	if !n.Enabled || !a.cores[n.coreKey()].alive() || n.Port == 0 {
		a.mu.Unlock()
		return NodeCapabilities{}, errors.New("请先启动节点再检测游戏网络能力")
	}
	before := cloneSettings(a.cfg)
	if n.AppPort == 0 {
		var err error
		n.AppPort, err = a.allocateAppPort()
		if err == nil {
			err = a.reconcile()
		}
		if err == nil {
			err = a.save()
		}
		if err != nil {
			a.cfg = before
			_ = a.reconcile()
			a.mu.Unlock()
			return NodeCapabilities{}, err
		}
	}
	proxyAddress := fmt.Sprintf("127.0.0.1:%d", n.AppPort)
	nodeName := n.Name
	proxySecret := a.cfg.AppRouting.Secret
	a.mu.Unlock()

	result := NodeCapabilities{CheckedAt: time.Now().UTC(), NAT: "unknown"}
	started := time.Now()
	conn, _, err := socks5Command(proxyAddress, "mirroxy-app", proxySecret, "turn.cloudflare.com", 80, 1, 5*time.Second)
	if err != nil {
		return result, fmt.Errorf("TCP 代理检测失败：%w", err)
	}
	result.TCP = true
	result.LatencyMS = time.Since(started).Milliseconds()
	_ = conn.Close()

	if err = probeIPv6ThroughSOCKS(proxyAddress, proxySecret); err == nil {
		result.IPv6 = true
	} else {
		result.IPv6Error = shortCapabilityError(err)
	}
	firstMapping, secondMapping, udpErr := probeUDPAndNATThroughSOCKS(proxyAddress, proxySecret)
	if udpErr == nil {
		result.UDP = true
		result.ExternalAddress = firstMapping
		if secondMapping == "" {
			result.NAT = "unknown"
		} else if firstMapping == secondMapping {
			result.NAT = "endpoint-independent"
		} else {
			result.NAT = "symmetric-likely"
		}
	} else {
		result.UDPError = shortCapabilityError(udpErr)
	}
	a.mu.Lock()
	if a.capabilities == nil {
		a.capabilities = map[string]NodeCapabilities{}
	}
	a.capabilities[id] = result
	a.event("节点游戏网络能力检测完成：" + nodeName)
	a.mu.Unlock()
	return result, nil
}

func shortCapabilityError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if len(text) > 240 {
		text = text[:240]
	}
	return text
}

func probeIPv6ThroughSOCKS(proxyAddress, secret string) error {
	conn, _, err := socks5Command(proxyAddress, "mirroxy-app", secret, "2606:4700:4700::1111", 443, 1, 5*time.Second)
	if conn != nil {
		_ = conn.Close()
	}
	return err
}

func probeUDPAndNATThroughSOCKS(proxyAddress, secret string) (string, string, error) {
	control, relay, err := socks5Command(proxyAddress, "mirroxy-app", secret, "0.0.0.0", 0, 3, 5*time.Second)
	if err != nil {
		return "", "", err
	}
	defer control.Close()
	host, _, _ := net.SplitHostPort(proxyAddress)
	if relay == nil {
		return "", "", errors.New("SOCKS5 未返回 UDP 中继地址")
	}
	if relay.IP == nil || relay.IP.IsUnspecified() {
		relay.IP = net.ParseIP(host)
	}
	udp, err := net.DialUDP("udp", nil, relay)
	if err != nil {
		return "", "", err
	}
	defer udp.Close()
	firstMapping, err := stunThroughSOCKSUDP(udp, "stun.cloudflare.com", 3478)
	if err != nil {
		return "", "", err
	}
	secondMapping, secondErr := stunThroughSOCKSUDP(udp, "stun.cloudflare.com", 53)
	if secondErr != nil {
		return firstMapping, "", nil
	}
	return firstMapping, secondMapping, nil
}

func socks5Command(proxyAddress, username, password, targetHost string, targetPort int, command byte, timeout time.Duration) (net.Conn, *net.UDPAddr, error) {
	conn, err := net.DialTimeout("tcp4", proxyAddress, timeout)
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (net.Conn, *net.UDPAddr, error) {
		_ = conn.Close()
		return nil, nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	methods := []byte{5, 1, 0}
	if username != "" {
		methods = []byte{5, 1, 2}
	}
	if _, err = conn.Write(methods); err != nil {
		return fail(err)
	}
	reply := make([]byte, 2)
	if _, err = io.ReadFull(conn, reply); err != nil {
		return fail(err)
	}
	if reply[0] != 5 || (reply[1] != 0 && reply[1] != 2) {
		return fail(errors.New("SOCKS5 协商失败"))
	}
	if reply[1] == 2 {
		if len(username) > 255 || len(password) > 255 {
			return fail(errors.New("SOCKS5 凭据过长"))
		}
		auth := append([]byte{1, byte(len(username))}, []byte(username)...)
		auth = append(auth, byte(len(password)))
		auth = append(auth, []byte(password)...)
		if _, err = conn.Write(auth); err != nil {
			return fail(err)
		}
		if _, err = io.ReadFull(conn, reply); err != nil {
			return fail(err)
		}
		if reply[0] != 1 || reply[1] != 0 {
			return fail(errors.New("SOCKS5 认证失败"))
		}
	}
	request := []byte{5, command, 0}
	address, err := encodeSOCKSAddress(targetHost, targetPort)
	if err != nil {
		return fail(err)
	}
	request = append(request, address...)
	if _, err = conn.Write(request); err != nil {
		return fail(err)
	}
	header := make([]byte, 4)
	if _, err = io.ReadFull(conn, header); err != nil {
		return fail(err)
	}
	if header[0] != 5 || header[1] != 0 {
		return fail(fmt.Errorf("SOCKS5 请求被拒绝（代码 %d）", header[1]))
	}
	boundHost, boundPort, err := readSOCKSAddress(conn, header[3])
	if err != nil {
		return fail(err)
	}
	_ = conn.SetDeadline(time.Time{})
	var relay *net.UDPAddr
	if command == 3 {
		relay, err = net.ResolveUDPAddr("udp", net.JoinHostPort(boundHost, fmt.Sprint(boundPort)))
		if err != nil {
			return fail(err)
		}
	}
	return conn, relay, nil
}

func encodeSOCKSAddress(host string, port int) ([]byte, error) {
	ip := net.ParseIP(host)
	var result []byte
	if ipv4 := ip.To4(); ipv4 != nil {
		result = append([]byte{1}, ipv4...)
	} else if ipv6 := ip.To16(); ipv6 != nil {
		result = append([]byte{4}, ipv6...)
	} else {
		if len(host) == 0 || len(host) > 255 {
			return nil, errors.New("SOCKS5 目标地址无效")
		}
		result = append([]byte{3, byte(len(host))}, []byte(host)...)
	}
	return append(result, byte(port>>8), byte(port)), nil
}

func readSOCKSAddress(reader io.Reader, addressType byte) (string, int, error) {
	var size int
	switch addressType {
	case 1:
		size = 4
	case 4:
		size = 16
	case 3:
		length := []byte{0}
		if _, err := io.ReadFull(reader, length); err != nil {
			return "", 0, err
		}
		size = int(length[0])
	default:
		return "", 0, errors.New("SOCKS5 返回了未知地址类型")
	}
	data := make([]byte, size+2)
	if _, err := io.ReadFull(reader, data); err != nil {
		return "", 0, err
	}
	host := string(data[:size])
	if addressType == 1 || addressType == 4 {
		host = net.IP(data[:size]).String()
	}
	return host, int(binary.BigEndian.Uint16(data[size:])), nil
}

func stunThroughSOCKSUDP(conn *net.UDPConn, host string, port int) (string, error) {
	transaction := make([]byte, 12)
	if _, err := rand.Read(transaction); err != nil {
		return "", err
	}
	request := make([]byte, 20)
	binary.BigEndian.PutUint16(request[0:2], 0x0001)
	binary.BigEndian.PutUint32(request[4:8], 0x2112a442)
	copy(request[8:], transaction)
	target, err := encodeSOCKSAddress(host, port)
	if err != nil {
		return "", err
	}
	packet := append([]byte{0, 0, 0}, target...)
	packet = append(packet, request...)
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	if _, err = conn.Write(packet); err != nil {
		return "", err
	}
	buffer := make([]byte, 2048)
	n, err := conn.Read(buffer)
	if err != nil {
		return "", err
	}
	payload, err := socksUDPPayload(buffer[:n])
	if err != nil {
		return "", err
	}
	return parseSTUNMapping(payload, transaction)
}

func socksUDPPayload(packet []byte) ([]byte, error) {
	if len(packet) < 7 || packet[0] != 0 || packet[1] != 0 || packet[2] != 0 {
		return nil, errors.New("SOCKS5 UDP 返回格式无效")
	}
	offset := 3
	switch packet[offset] {
	case 1:
		offset += 1 + 4 + 2
	case 4:
		offset += 1 + 16 + 2
	case 3:
		if len(packet) < offset+2 {
			return nil, errors.New("SOCKS5 UDP 域名格式无效")
		}
		offset += 2 + int(packet[offset+1]) + 2
	default:
		return nil, errors.New("SOCKS5 UDP 地址类型无效")
	}
	if offset > len(packet) {
		return nil, errors.New("SOCKS5 UDP 数据不完整")
	}
	return packet[offset:], nil
}

func parseSTUNMapping(packet, transaction []byte) (string, error) {
	if len(packet) < 20 || binary.BigEndian.Uint16(packet[0:2]) != 0x0101 || binary.BigEndian.Uint32(packet[4:8]) != 0x2112a442 || !equalBytes(packet[8:20], transaction) {
		return "", errors.New("STUN 响应无效")
	}
	length := int(binary.BigEndian.Uint16(packet[2:4]))
	if 20+length > len(packet) {
		return "", errors.New("STUN 响应不完整")
	}
	for offset := 20; offset+4 <= 20+length; {
		kind := binary.BigEndian.Uint16(packet[offset : offset+2])
		size := int(binary.BigEndian.Uint16(packet[offset+2 : offset+4]))
		start := offset + 4
		end := start + size
		if end > len(packet) {
			break
		}
		if (kind == 0x0020 || kind == 0x0001) && size >= 8 {
			family := packet[start+1]
			port := binary.BigEndian.Uint16(packet[start+2 : start+4])
			address := append([]byte{}, packet[start+4:end]...)
			if kind == 0x0020 {
				port ^= 0x2112
				mask := append([]byte{0x21, 0x12, 0xa4, 0x42}, transaction...)
				for i := range address {
					address[i] ^= mask[i]
				}
			}
			if (family == 1 && len(address) == 4) || (family == 2 && len(address) == 16) {
				return net.JoinHostPort(net.IP(address).String(), fmt.Sprint(port)), nil
			}
		}
		offset = (end + 3) &^ 3
	}
	return "", errors.New("STUN 未返回外部映射地址")
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func capabilityLabel(result NodeCapabilities) string {
	values := []string{fmt.Sprintf("TCP=%t", result.TCP), fmt.Sprintf("UDP=%t", result.UDP), fmt.Sprintf("IPv6=%t", result.IPv6), "NAT=" + result.NAT}
	return strings.Join(values, " ")
}
