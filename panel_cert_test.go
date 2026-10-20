package main

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
	"time"
)

func TestPanelCertificateIssuedByLocalCA(t *testing.T) {
	a := &App{root: t.TempDir()}
	if a.configuredPanelPort() != defaultPanelPort {
		t.Fatalf("default panel port must be %d", defaultPanelPort)
	}
	custom := 19527
	a.cfg.PanelPort = custom
	if a.configuredPanelPort() != custom {
		t.Fatal("configured panel port must win over the default")
	}

	leaf, err := a.ensurePanelCertificate()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(leaf.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if !hasIPSAN(parsed, "127.0.0.1") || !hasIPSAN(parsed, "::1") {
		t.Fatalf("leaf must cover loopback IPs: %+v", parsed.IPAddresses)
	}
	if !hasDNSSAN(parsed, "localhost") {
		t.Fatalf("leaf must cover localhost: %+v", parsed.DNSNames)
	}
	caPEM, err := os.ReadFile(a.dataPath("Mirroxy_ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(caPEM)
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	if _, err = parsed.Verify(x509.VerifyOptions{
		DNSName: "127.0.0.1", Roots: pool,
		CurrentTime: time.Now(),
	}); err != nil {
		t.Fatalf("leaf must verify against the local CA: %v", err)
	}
	// A second call reuses the cached leaf (same bytes, no regeneration).
	again, err := a.ensurePanelCertificate()
	if err != nil {
		t.Fatal(err)
	}
	if string(leaf.Certificate[0]) != string(again.Certificate[0]) {
		t.Fatal("valid leaf certificates must be reused, not reissued")
	}
	status := a.panelCAStatus()
	if status["hasCA"] != true {
		t.Fatalf("CA status must report the generated CA: %+v", status)
	}
}

func hasIPSAN(cert *x509.Certificate, want string) bool {
	for _, ip := range cert.IPAddresses {
		if ip.String() == want {
			return true
		}
	}
	return false
}

func hasDNSSAN(cert *x509.Certificate, want string) bool {
	for _, name := range cert.DNSNames {
		if name == want {
			return true
		}
	}
	return false
}
