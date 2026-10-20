//go:build linux || darwin

package main

import (
	"encoding/json"
	"errors"
	"os"
	"syscall"
)

func (a *App) launchAppUpdate(path string) error {
	if _, err := loadUpdatePlan(path); err != nil {
		return err
	}
	a.mu.Lock()
	a.pendingAppUpdate = path
	a.mu.Unlock()
	return nil
}
func finishAppUpdate(path string) error {
	p, err := loadUpdatePlan(path)
	if err == nil {
		err = backupUpdateData(p)
	}
	if err == nil {
		err = commitUpdate(p)
	}
	if err != nil {
		exe, _ := os.Executable()
		// The previous executable resumes with its already preserved encrypted state.
		return syscall.Exec(exe, append(os.Args, "-resume-update"), os.Environ())
	}
	args := append(updateArgs(p), "-after-update", path)
	if err = syscall.Exec(p.Exe, append([]string{p.Exe}, args...), os.Environ()); err != nil {
		_ = rollbackUpdate(p)
		return syscall.Exec(p.Exe, append([]string{p.Exe}, updateArgs(p)...), os.Environ())
	}
	return nil
}
func rollbackAndExec(path string) error {
	// Candidate has already become the running executable; validate plan paths
	// again using its original location before restoring the trusted backup.
	var p appUpdatePlan
	if json.Unmarshal(mustReadFile(path), &p) != nil {
		return errors.New("无效恢复计划")
	}
	if err := validateUpdatePaths(path, &p); err != nil {
		return err
	}
	if digestBytes(mustReadFile(p.Exe)) != p.SHA256 {
		return errors.New("更新程序状态不匹配")
	}
	if err := rollbackUpdate(&p); err != nil {
		return err
	}
	return syscall.Exec(p.Exe, append([]string{p.Exe}, updateArgs(&p)...), os.Environ())
}
