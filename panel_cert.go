package main

// Built-in mkcert-style panel certificate: a private local CA issues a leaf
// certificate for 127.0.0.1 / localhost, and the CA can be installed into the
// OS trust store so browsers show a fully trusted padlock without prompts.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultPanelPort = 9527
	legacyPanelPort  = 15060
)

func (a *App) configuredPanelPort() int {
	if a.cfg.PanelPort >= 1024 && a.cfg.PanelPort <= 65000 {
		return a.cfg.PanelPort
	}
	return defaultPanelPort
}

func (a *App) panelScheme() string {
	if a.panelTLS {
		return "https"
	}
	return "http"
}

func (a *App) panelURL() string {
	return fmt.Sprintf("%s://127.0.0.1:%d", a.panelScheme(), a.panelPort)
}

// wrapPanelTLS swaps an already-listening plain listener for TLS in place and
// records the result; on failure the panel keeps serving plain HTTP.
func (a *App) wrapPanelTLS(listener net.Listener) net.Listener {
	cert, err := a.ensurePanelCertificate()
	if err != nil {
		a.mu.Lock()
		a.event("HTTPS 证书未就绪，面板回退 HTTP：" + err.Error())
		a.mu.Unlock()
		return listener
	}
	a.mu.Lock()
	a.panelTLS = true
	a.mu.Unlock()
	return tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
}

func randomSerial() *big.Int {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	n, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return big.NewInt(1)
	}
	return n
}

func writePEM(path string, blockType string, der []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), mode)
}

func loadOrCreateCA(keyPath, certPath string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	if certPEM, err := os.ReadFile(certPath); err == nil {
		if block, _ := pem.Decode(certPEM); block != nil {
			if parsed, e := x509.ParseCertificate(block.Bytes); e == nil && time.Now().Before(parsed.NotAfter) {
				if keyPEM, e2 := os.ReadFile(keyPath); e2 == nil {
					if keyBlock, _ := pem.Decode(keyPEM); keyBlock != nil {
						if key, e3 := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); e3 == nil {
							if caKey, ok := key.(*ecdsa.PrivateKey); ok {
								return parsed, caKey, nil
							}
						}
					}
				}
			}
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber:          randomSerial(),
		Subject:               pkix.Name{CommonName: "Mirroxy Local CA", Organization: []string{"Mirroxy"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	if err = writePEM(certPath, "CERTIFICATE", der, 0644); err != nil {
		return nil, nil, err
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	if err = writePEM(keyPath, "PRIVATE KEY", pkcs8, 0600); err != nil {
		return nil, nil, err
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return parsed, key, nil
}

// ensurePanelCertificate returns a TLS certificate for the panel, issued by
// the persistent local CA. The leaf is reused while valid and CA-matched.
func (a *App) ensurePanelCertificate() (tls.Certificate, error) {
	caCert, caKey, err := loadOrCreateCA(a.dataPath("Mirroxy_ca_key.pem"), a.dataPath("Mirroxy_ca.pem"))
	if err != nil {
		return tls.Certificate{}, err
	}
	certPath, keyPath := a.dataPath("Mirroxy_panel_cert.pem"), a.dataPath("Mirroxy_panel_key.pem")
	if leaf, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		if parsed, e := x509.ParseCertificate(leaf.Certificate[0]); e == nil &&
			time.Now().Before(parsed.NotAfter.Add(-30*24*time.Hour)) && parsed.CheckSignatureFrom(caCert) == nil {
			return leaf, nil
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	template := &x509.Certificate{
		SerialNumber: randomSerial(),
		Subject:      pkix.Name{CommonName: "Mirroxy Local Panel", Organization: []string{"Mirroxy"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(3, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	if err = writePEM(certPath, "CERTIFICATE", der, 0644); err != nil {
		return tls.Certificate{}, err
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	if err = writePEM(keyPath, "PRIVATE KEY", pkcs8, 0600); err != nil {
		return tls.Certificate{}, err
	}
	return tls.LoadX509KeyPair(certPath, keyPath)
}

// panelCAStatus describes the local CA for the settings page.
func (a *App) panelCAStatus() M {
	status := M{"hasCA": false, "tls": a.panelTLS, "url": a.panelURL()}
	caPEM, err := os.ReadFile(a.dataPath("Mirroxy_ca.pem"))
	if err != nil {
		return status
	}
	block, _ := pem.Decode(caPEM)
	if block == nil {
		return status
	}
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return status
	}
	status["hasCA"] = true
	status["subject"] = parsed.Subject.CommonName
	status["notAfter"] = parsed.NotAfter.Format("2006-01-02")
	sum := sha256.Sum256(parsed.Raw)
	status["fingerprint"] = "SHA256 " + hex.EncodeToString(sum[:])[:32]
	return status
}

// verifyPanelTrust performs a real TLS handshake against the panel using the
// OS trust store; success means browsers will show the trusted padlock.
func (a *App) verifyPanelTrust() bool {
	if !a.panelTLS {
		return false
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp",
		fmt.Sprintf("127.0.0.1:%d", a.panelPort), &tls.Config{ServerName: "127.0.0.1"})
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// waitPanelTrust gives the Windows certificate service a brief moment to make
// a newly imported root visible, then validates the exact HTTPS endpoint users
// open. Import success alone is not enough to report a trusted certificate.
func (a *App) waitPanelTrust(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if a.verifyPanelTrust() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// rebindPanelPort moves the panel listener to a new port at runtime. The old
// server keeps answering until the new one is serving, then drains.
func (a *App) rebindPanelPort(newPort int) error {
	a.mu.Lock()
	if a.panelServer == nil {
		a.mu.Unlock()
		return errors.New("面板服务尚未就绪")
	}
	if a.panelMoving {
		a.mu.Unlock()
		return errors.New("面板端口正在切换，请稍候")
	}
	if newPort == a.panelPort {
		a.mu.Unlock()
		return nil
	}
	oldPort := a.panelPort
	handler := a.panelServer.Handler
	oldServer := a.panelServer
	a.panelMoving = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.panelMoving = false
		a.mu.Unlock()
	}()

	raw, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", newPort))
	if err != nil {
		return fmt.Errorf("端口 %d 不可用：%v", newPort, err)
	}
	listener, tlsOK := raw, false
	if cert, cerr := a.ensurePanelCertificate(); cerr == nil {
		listener = tls.NewListener(raw, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
		tlsOK = true
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	a.mu.Lock()
	a.panelPort = newPort
	a.panelTLS = tlsOK
	a.panelServer = server
	a.mu.Unlock()
	go func() {
		if e := server.Serve(listener); e != nil && e != http.ErrServerClosed {
			log.Print(e)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = oldServer.Shutdown(ctx)
	a.event(fmt.Sprintf("面板端口已切换：%d → %d，新地址 %s", oldPort, newPort, a.panelURL()))
	// The PAC URL carries the panel port, so an active PAC must be re-armed.
	a.mu.Lock()
	mode, node := a.cfg.SystemProxyMode, a.cfg.SystemProxyNode
	a.mu.Unlock()
	if mode == "pac" && node != "" {
		a.mu.Lock()
		if err := a.applySystemProxy("pac", node); err != nil {
			a.event("面板迁移后 PAC 恢复失败：" + err.Error())
		}
		a.mu.Unlock()
	}
	return nil
}

// serveLegacyRedirect keeps old bookmarks alive by redirecting the previous
// default port to the current panel address.
func (a *App) serveLegacyRedirect(port int) {
	ln, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		target := a.panelURL() + r.URL.Path
		a.mu.Unlock()
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	_ = server.Serve(ln)
}
