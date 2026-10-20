//go:build !windows

package main

import "errors"

// Automatic root-store installation is Windows-only; other platforms import
// the exported CA file manually.
func installPanelTrust(caPath string) error {
	return errors.New("本平台请手动导入 ca.pem（仅 Windows 支持一键安装）")
}
