//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func platformOpenDownload(target string, folder bool) error {
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		if folder {
			command = exec.Command("open", "-R", target)
		} else {
			command = exec.Command("open", target)
		}
	} else {
		if folder {
			if info, err := os.Stat(target); err == nil && !info.IsDir() {
				target = filepath.Dir(target)
			}
		}
		command = exec.Command("xdg-open", target)
	}
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
