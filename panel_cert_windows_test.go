//go:build windows

package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/pem"
	"os"
	"strings"
	"testing"
)

func TestPanelCertificateThumbprintMatchesWindowsFormat(t *testing.T) {
	a := &App{root: t.TempDir()}
	if _, err := a.ensurePanelCertificate(); err != nil {
		t.Fatal(err)
	}
	path := a.dataPath("Mirroxy_ca.pem")
	got, err := panelCertificateThumbprint(path)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(contents)
	if block == nil {
		t.Fatal("missing certificate PEM block")
	}
	sum := sha1.Sum(block.Bytes)
	want := strings.ToUpper(hex.EncodeToString(sum[:]))
	if got != want {
		t.Fatalf("thumbprint = %q, want %q", got, want)
	}
}
