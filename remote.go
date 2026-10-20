package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type RemoteSettings struct {
	Scheme       string `json:"scheme"`
	Enabled      bool   `json:"enabled"`
	Port         int    `json:"port"`
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
	Salt         string `json:"salt"`
	CertFile     string `json:"certFile"`
	KeyFile      string `json:"keyFile"`
}
type loginFailures struct {
	Count int
	Since time.Time
}

func passwordHash(password, salt string) string {
	b, _ := pbkdf2.Key(sha256.New, password, []byte(salt), 600000, 32)
	return hex.EncodeToString(b)
}
func (a *App) authenticated(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok || len(pass) > 1024 {
		return false
	}
	a.mu.Lock()
	cfg := a.cfg.Remote
	a.mu.Unlock()
	if cfg.PasswordHash == "" || cfg.Username == "" {
		return false
	}
	sum := sha256.Sum256([]byte(cfg.PasswordHash + "\x00" + cfg.Username + "\x00" + r.Header.Get("Authorization")))
	cacheKey := hex.EncodeToString(sum[:])
	peer, _, _ := net.SplitHostPort(r.RemoteAddr)
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if a.authCache == nil {
		a.authCache = map[string]time.Time{}
	}
	if a.authFailures == nil {
		a.authFailures = map[string]*loginFailures{}
	}
	if len(a.authFailures) > 4096 {
		for peer, v := range a.authFailures {
			if time.Since(v.Since) > time.Minute {
				delete(a.authFailures, peer)
			}
		}
		if a.authFailures[peer] == nil && len(a.authFailures) > 4096 {
			return false
		}
	}
	now := time.Now()
	if expires := a.authCache[cacheKey]; expires.After(now) {
		return true
	}
	f := a.authFailures[peer]
	if f == nil || now.Sub(f.Since) > time.Minute {
		f = &loginFailures{Since: now}
		a.authFailures[peer] = f
	}
	if f.Count >= 10 {
		return false
	}
	validUser := subtle.ConstantTimeCompare([]byte(user), []byte(cfg.Username)) == 1
	validPass := subtle.ConstantTimeCompare([]byte(passwordHash(pass, cfg.Salt)), []byte(cfg.PasswordHash)) == 1
	if !validUser || !validPass {
		f.Count++
		return false
	}
	if len(a.authCache) > 128 {
		clear(a.authCache)
	}
	if len(a.authFailures) > 1024 {
		clear(a.authFailures)
	}
	a.authCache[cacheKey] = now.Add(5 * time.Minute)
	return true
}
func loopbackRequest(r *http.Request) bool {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func forwardedRequest(r *http.Request) bool {
	for key := range r.Header {
		k := strings.ToLower(key)
		if k == "forwarded" || strings.HasPrefix(k, "x-forwarded-") || k == "x-real-ip" {
			return true
		}
	}
	return false
}

type localAccessKey struct{}

func localAccess(r *http.Request) bool { v, _ := r.Context().Value(localAccessKey{}).(bool); return v }
func (a *App) accessGuard(next http.Handler, remote bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("Cache-Control", "no-store")
		local := !remote && loopbackRequest(r) && !forwardedRequest(r)
		if local {
			expected := fmt.Sprintf("127.0.0.1:%d", a.panelPort)
			localhost := fmt.Sprintf("localhost:%d", a.panelPort)
			if r.Host != expected && r.Host != localhost {
				http.Error(w, "不支持的本机 Host", 403)
				return
			}
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			if e != nil || u.Host != r.Host || u.Scheme != scheme {
				http.Error(w, "跨站请求被拒绝", 403)
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" && strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, "跨站请求被拒绝", 403)
			return
		}
		if !local && !a.authenticated(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Mirroxy", charset="UTF-8"`)
			http.Error(w, "需要远程账号密码，或尝试次数过多", 401)
			return
		}
		if !local && !serverPlatform() && (r.URL.Path == "/api/remote" || r.URL.Path == "/api/settings") {
			http.Error(w, "全局与远程访问设置请在本机面板修改", 403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/api/v1/") && r.Method != "GET" {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-App-Token")), []byte(a.token)) != 1 {
				http.Error(w, "请刷新面板后再试", 403)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), localAccessKey{}, local)))
	})
}
func (a *App) remoteState() M {
	p := a.cfg.Remote.Port
	if p == 0 {
		p = 15061
	}
	return M{"enabled": a.cfg.Remote.Enabled, "running": a.remoteServer != nil, "port": p, "username": a.cfg.Remote.Username, "passwordSet": a.cfg.Remote.PasswordHash != "", "certFile": a.cfg.Remote.CertFile, "keyFile": a.cfg.Remote.KeyFile, "error": a.remoteError, "scheme": first(a.cfg.Remote.Scheme, "https")}
}
func (a *App) remoteCertificate(cfg RemoteSettings) (tls.Certificate, error) {
	if cfg.CertFile != "" || cfg.KeyFile != "" {
		return tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	}
	certPath, keyPath := a.dataPath("Mirroxy_cert.pem"), a.dataPath("Mirroxy_tls.key")
	if _, e := os.Stat(certPath); e == nil {
		return tls.LoadX509KeyPair(certPath, keyPath)
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return tls.Certificate{}, e
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	name, _ := os.Hostname()
	cert := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Mirroxy"}, DNSNames: []string{"localhost", name}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(1, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	addrs, _ := net.InterfaceAddrs()
	for _, v := range addrs {
		if ip, _, e := net.ParseCIDR(v.String()); e == nil {
			cert.IPAddresses = append(cert.IPAddresses, ip)
		}
	}
	der, e := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if e != nil {
		return tls.Certificate{}, e
	}
	pk, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		return tls.Certificate{}, e
	}
	if e = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk}), 0600); e != nil {
		return tls.Certificate{}, e
	}
	if e = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); e != nil {
		return tls.Certificate{}, e
	}
	return tls.LoadX509KeyPair(certPath, keyPath)
}

// The remote listener always requires credentials, including connections forwarded
// from a local reverse proxy. It never inherits the local API's auth exemption.
func (a *App) prepareRemote(cfg RemoteSettings) (*http.Server, net.Listener, error) {
	if !cfg.Enabled {
		return nil, nil, nil
	}
	if cfg.Port < 1024 || cfg.Port > 65535 || cfg.Port == a.panelPort {
		return nil, nil, errors.New("远程端口必须在 1024–65535，且不能与面板或节点重复")
	}
	for _, n := range a.cfg.Nodes {
		if n.Port == cfg.Port {
			return nil, nil, errors.New("远程端口与节点端口重复")
		}
	}
	if cfg.Username == "" || cfg.PasswordHash == "" {
		return nil, nil, errors.New("请先设置远程账号和密码")
	}
	scheme := first(cfg.Scheme, "https")
	if scheme != "http" && scheme != "https" {
		return nil, nil, errors.New("远程协议必须为 HTTP 或 HTTPS")
	}
	var cert tls.Certificate
	var e error
	if scheme == "https" {
		cert, e = a.remoteCertificate(cfg)
		if e != nil {
			return nil, nil, fmt.Errorf("HTTPS 证书加载失败：%w", e)
		}
	}
	l, e := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", cfg.Port))
	if e != nil {
		return nil, nil, e
	}
	server := &http.Server{Handler: a.handler(true), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 << 10, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}}
	if scheme == "http" {
		return server, l, nil
	}
	return server, tls.NewListener(l, server.TLSConfig), nil
}
func (a *App) startRemote(server *http.Server, l net.Listener) {
	a.remoteServer = server
	a.remoteError = ""
	if server == nil {
		return
	}
	go func() {
		if e := server.Serve(l); e != nil && e != http.ErrServerClosed {
			a.mu.Lock()
			a.remoteError = "远程服务已停止：" + e.Error()
			if a.remoteServer == server {
				a.remoteServer = nil
			}
			a.mu.Unlock()
		}
	}()
}
func (a *App) saveRemote(cfg RemoteSettings, password string) error {
	before := a.cfg.Remote
	cfg.PasswordHash = before.PasswordHash
	cfg.Salt = before.Salt
	cfg.Username = strings.TrimSpace(cfg.Username)
	if strings.ContainsAny(cfg.Username, ":\r\n") || len(cfg.Username) > 128 {
		return errors.New("账号不能含冒号或换行，长度最多 128")
	}
	if password != "" {
		if len(password) < 10 || len(password) > 256 {
			return errors.New("远程密码至少 10 个字符，最多 256")
		}
		cfg.Salt = newID() + newID()
		cfg.PasswordHash = passwordHash(password, cfg.Salt)
	}
	if cfg.Port == 0 {
		cfg.Port = 15061
	}
	if cfg.Scheme == "" {
		cfg.Scheme = first(before.Scheme, "https")
	}
	reuse := a.remoteServer != nil && cfg.Enabled && before.Port == cfg.Port && before.CertFile == cfg.CertFile && before.KeyFile == cfg.KeyFile && first(before.Scheme, "https") == cfg.Scheme
	if reuse && (cfg.Username == "" || cfg.PasswordHash == "") {
		return errors.New("远程账号密码不能为空")
	}
	if a.remoteServer != nil && cfg.Enabled && before.Port == cfg.Port && !reuse {
		return errors.New("更换协议或证书时请同时使用新管理端口，保存后访问新地址")
	}
	var server *http.Server
	var l net.Listener
	var e error
	if !reuse {
		server, l, e = a.prepareRemote(cfg)
		if e != nil {
			return e
		}
	}
	a.cfg.Remote = cfg
	if e = a.save(); e != nil {
		a.cfg.Remote = before
		if l != nil {
			l.Close()
		}
		return e
	}
	if !reuse {
		if a.remoteServer != nil {
			a.remoteServer.Close()
		}
		a.startRemote(server, l)
	}
	if password != "" || cfg.Username != before.Username || cfg.Port != before.Port || cfg.Scheme != before.Scheme {
		_ = os.Remove(a.dataPath("access-info.txt"))
	}
	a.event("远程访问设置已保存")
	return nil
}
