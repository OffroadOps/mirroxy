//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows"
)

var errAlreadyRunning = errors.New("已有 Mirroxy 实例正在运行")

// acquireInstanceLock enforces one running Mirroxy instance per data
// directory using an OS-managed advisory file lock so crashes leave no stale lock.
func acquireInstanceLock(root string) (func(), error) {
	lockPath := filepath.Join(root, "data", "Mirroxy.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	var ol windows.Overlapped
	err = windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &ol,
	)
	if err != nil {
		_ = f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, errAlreadyRunning
		}
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			var uol windows.Overlapped
			_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &uol)
			_ = f.Close()
		})
	}, nil
}
