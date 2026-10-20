//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"time"
)

func (a *App) launchAppUpdate(planPath string) error {
	p, err := loadUpdatePlan(planPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(p.CandidateUpgrade, "--apply", planPath)
	hideCommand(cmd)
	if err = cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	for i := 0; i < 50; i++ {
		if _, err = os.Stat(planPath + ".ready"); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("更新助手未就绪，当前版本继续运行")
}
func finishAppUpdate(string) error { return nil }
func rollbackAndExec(string) error { return errors.New("更新恢复由 Windows 助手管理") }
