//go:build windows

package main

import (
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// installPanelTrust imports the local CA into the current-user root store.
// Windows may show its trusted-root security confirmation. certutil has
// reliable process exit codes, unlike a PowerShell cmdlet whose non-terminating
// errors can otherwise be mistaken for success.
func installPanelTrust(caPath string) error {
	thumbprint, err := panelCertificateThumbprint(caPath)
	if err != nil {
		return fmt.Errorf("读取根证书失败：%w", err)
	}
	cmd := exec.Command("certutil.exe", "-user", "-addstore", "-f", "Root", caPath)
	hideCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("安装根证书失败：%s", message)
	}
	// Verify the exact certificate instead of trusting the import command's
	// success message. This also prevents an older same-subject CA from being
	// reported as the currently active Mirroxy root.
	check := exec.Command("certutil.exe", "-user", "-store", "Root", thumbprint)
	hideCommand(check)
	if verifyOutput, verifyErr := check.CombinedOutput(); verifyErr != nil {
		message := strings.TrimSpace(string(verifyOutput))
		if message == "" {
			message = verifyErr.Error()
		}
		return fmt.Errorf("根证书导入后校验失败：%s", message)
	}
	return nil
}

func panelCertificateThumbprint(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(contents)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("无效的 PEM 证书")
	}
	if _, err = x509.ParseCertificate(block.Bytes); err != nil {
		return "", err
	}
	sum := sha1.Sum(block.Bytes) // Windows certificate stores identify entries by SHA-1 thumbprint.
	return strings.ToUpper(hex.EncodeToString(sum[:])), nil
}
