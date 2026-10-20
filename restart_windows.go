//go:build windows

package main

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

const processSynchronize = 0x00100000

func waitForProcessExit(pid int, timeout time.Duration) error {
	handle, err := windows.OpenProcess(processSynchronize, false, uint32(pid))
	if err != nil {
		return nil // The process is already gone.
	}
	defer windows.CloseHandle(handle)
	event, err := windows.WaitForSingleObject(handle, uint32(timeout.Milliseconds()))
	if err != nil {
		return err
	}
	if event != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("等待旧进程退出超时（%dms）", timeout.Milliseconds())
	}
	return nil
}
