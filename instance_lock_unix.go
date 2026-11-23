//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

var errAlreadyRunning = errors.New("已有 Mirroxy 实例正在运行")

// acquireInstanceLock enforces one running Mirroxy instance per data
// directory using an OS-managed advisory flock so crashes leave no stale lock.
func acquireInstanceLock(root string) (func(), error) {
	lockPath := filepath.Join(root, "data", "Mirroxy.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, errAlreadyRunning
		}
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			_ = f.Close()
		})
	}, nil
}
