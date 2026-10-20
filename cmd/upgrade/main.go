//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

type plan struct {
	Root, Exe, Candidate, Backup, Version, SHA256 string
	Upgrade, CandidateUpgrade, UpgradeSHA256      string
	Port, ParentPID                               int
}

var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func digest(path string) string {
	f, e := os.Open(path)
	if e != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}
func readPlan(path string) (*plan, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var p plan
	if json.Unmarshal(b, &p) != nil || !versionPattern.MatchString(p.Version) || p.Port < 1024 || p.Port > 65535 || p.ParentPID <= 0 {
		return nil, errors.New("无效更新计划")
	}
	stage := filepath.Dir(path)
	expectedStage := filepath.Join(p.Root, "data", "app-updates", filepath.Base(stage))
	if filepath.Clean(expectedStage) != filepath.Clean(stage) || p.Exe != filepath.Join(p.Root, "Mirroxy.exe") || p.Candidate != filepath.Join(stage, "Mirroxy.exe") || p.Backup != filepath.Join(stage, "previous.exe") || p.Upgrade != filepath.Join(p.Root, "upgrade.exe") || p.CandidateUpgrade != filepath.Join(stage, "upgrade.exe") {
		return nil, errors.New("更新计划路径不匹配")
	}
	for _, file := range []string{p.Candidate, p.CandidateUpgrade} {
		if st, e := os.Lstat(file); e != nil || !st.Mode().IsRegular() {
			return nil, errors.New("更新文件不存在或类型无效")
		}
	}
	if digest(p.Candidate) != p.SHA256 || digest(p.CandidateUpgrade) != p.UpgradeSHA256 {
		return nil, errors.New("更新文件校验失败")
	}
	return &p, nil
}
func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
func copyFile(src, dst string, mode os.FileMode) error {
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	closeErr := out.Close()
	if e != nil {
		return e
	}
	return closeErr
}
func backupData(p *plan) error {
	for _, name := range []string{"Mirroxy.db", "Mirroxy.key", "settings.json", "traffic.json"} {
		src := filepath.Join(p.Root, "data", name)
		if _, e := os.Stat(src); os.IsNotExist(e) {
			continue
		}
		if e := copyFile(src, filepath.Join(filepath.Dir(p.Backup), "rollback-"+name), 0600); e != nil {
			return e
		}
	}
	return nil
}
func restoreData(p *plan) {
	for _, name := range []string{"Mirroxy.db", "Mirroxy.key", "settings.json", "traffic.json"} {
		src := filepath.Join(filepath.Dir(p.Backup), "rollback-"+name)
		if _, e := os.Stat(src); e == nil {
			_ = copyFile(src, filepath.Join(p.Root, "data", name), 0600)
		}
	}
}
func start(p *plan) (*exec.Cmd, error) {
	cmd := exec.Command(p.Exe, "-dir", p.Root, "-panel", fmt.Sprint(p.Port), "-no-open", "-resume-update")
	hide(cmd)
	e := cmd.Start()
	return cmd, e
}
func ready(p *plan, pid int, timeout time.Duration) bool {
	client := http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(process)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if status, _ := windows.WaitForSingleObject(process, 0); status == windows.WAIT_OBJECT_0 {
			return false
		}
		res, e := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/status", p.Port))
		if e == nil {
			var v struct {
				Version string
				Nodes   []struct {
					Enabled bool
					Status  string
				}
			}
			e = json.NewDecoder(res.Body).Decode(&v)
			res.Body.Close()
			if e == nil && v.Version == p.Version {
				ok := true
				for _, n := range v.Nodes {
					if n.Enabled && n.Status != "running" {
						ok = false
					}
				}
				if ok {
					return true
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}
func apply(path string) error {
	p, e := readPlan(path)
	if e != nil {
		return e
	}
	parent, e := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(p.ParentPID))
	if e != nil {
		return e
	}
	defer windows.CloseHandle(parent)
	if e = os.WriteFile(path+".ready", []byte("ready"), 0600); e != nil {
		return e
	}
	if status, w := windows.WaitForSingleObject(parent, 45000); w != nil || status != windows.WAIT_OBJECT_0 {
		return errors.New("旧程序未退出，未执行更新")
	}
	if e = backupData(p); e != nil {
		return e
	}
	deadline := time.Now().Add(45 * time.Second)
	for {
		e = os.Rename(p.Exe, p.Backup)
		if e == nil {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("旧程序文件仍被占用")
		}
		time.Sleep(100 * time.Millisecond)
	}
	rollback := func() { _ = os.Remove(p.Exe); _ = os.Rename(p.Backup, p.Exe); restoreData(p) }
	if e = os.Rename(p.Candidate, p.Exe); e != nil {
		rollback()
		return e
	}
	nextUpgrade := p.Upgrade + ".next"
	_ = os.Remove(nextUpgrade)
	if e = copyFile(p.CandidateUpgrade, nextUpgrade, 0700); e != nil {
		rollback()
		return e
	}
	upgradeBackup := filepath.Join(filepath.Dir(p.Backup), "previous-upgrade.exe")
	if _, statErr := os.Stat(p.Upgrade); statErr == nil {
		if e = os.Rename(p.Upgrade, upgradeBackup); e != nil {
			_ = os.Remove(nextUpgrade)
			rollback()
			return e
		}
	}
	if e = os.Rename(nextUpgrade, p.Upgrade); e != nil {
		_ = os.Rename(upgradeBackup, p.Upgrade)
		rollback()
		return e
	}
	cmd, e := start(p)
	if e == nil && ready(p, cmd.Process.Pid, 40*time.Second) {
		_ = cmd.Process.Release()
		return nil
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	_ = os.Remove(p.Upgrade)
	_ = os.Rename(upgradeBackup, p.Upgrade)
	rollback()
	old, _ := start(p)
	if old != nil && old.Process != nil {
		_ = old.Process.Release()
	}
	if e == nil {
		e = errors.New("新版本未能就绪，已恢复旧版本")
	}
	return e
}
func main() {
	if len(os.Args) != 3 || os.Args[1] != "--apply" {
		fmt.Fprintln(os.Stderr, "upgrade.exe 只接受 Mirroxy 生成的更新计划")
		os.Exit(2)
	}
	if p, e := readPlan(os.Args[2]); e == nil {
		_ = os.MkdirAll(filepath.Join(p.Root, "data"), 0700)
		if f, e := os.OpenFile(filepath.Join(p.Root, "data", "Mirroxy_upgrade.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); e == nil {
			log.SetOutput(f)
			defer f.Close()
		}
	}
	if e := apply(os.Args[2]); e != nil {
		log.Print(e)
		os.Exit(1)
	}
}
