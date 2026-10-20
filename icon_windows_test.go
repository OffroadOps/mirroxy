//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"testing"
)

func TestEmbeddedWindowsIconSizes(t *testing.T) {
	instance, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	for _, size := range []uintptr{16, 20, 32, 48, 256} {
		icon, _, err := trayUser.NewProc("LoadImageW").Call(instance, 1, 1, size, size, 0)
		if icon == 0 {
			t.Fatalf("embedded icon unavailable at %d px: %v", size, err)
		}
		trayUser.NewProc("DestroyIcon").Call(icon)
	}
}
