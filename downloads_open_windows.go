//go:build windows

package main

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"unsafe"
)

func platformOpenDownload(target string, folder bool) error {
	file := target
	parameters := ""
	if folder {
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			directory, err := windows.GetWindowsDirectory()
			if err != nil {
				return err
			}
			file = filepath.Join(directory, "explorer.exe")
			parameters = `/select,"` + target + `"`
		}
	}
	operation, _ := windows.UTF16PtrFromString("open")
	path, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	args, err := windows.UTF16PtrFromString(parameters)
	if err != nil {
		return err
	}
	working, _ := windows.UTF16PtrFromString(filepath.Dir(target))
	result, _, _ := shellExecuteW.Call(0, uintptr(unsafe.Pointer(operation)), uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(args)), uintptr(unsafe.Pointer(working)), 1)
	if result <= 32 {
		return fmt.Errorf("系统无法打开文件或文件夹（代码 %d）", result)
	}
	return nil
}
