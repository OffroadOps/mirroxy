package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// The release catalog is published next to the sources in the public
// repository; the binaries themselves live in the GitHub releases.
const (
	publicReleaseBase = "https://raw.githubusercontent.com/OffroadOps/mirroxy/master/"
	publicReleaseRepo = "OffroadOps/mirroxy"
)

var releaseVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

type AppUpdateState struct {
	Phase     string `json:"phase"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Message   string `json:"message"`
	Size      int64  `json:"size"`
}
type appUpdateFile struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type appUpdateOffer struct {
	Version string
	File    appUpdateFile
}
type appUpdatePlan struct {
	Root, Exe, Candidate, Backup, Version, SHA256 string
	Upgrade, CandidateUpgrade, UpgradeSHA256      string
	Port, ParentPID                               int
}

func releaseBase() (string, error) {
	base := first(os.Getenv("MIRROXY_RELEASE_BASE"), publicReleaseBase)
	u, err := url.Parse(base)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Host == "" {
		return "", errors.New("无效发布源")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())) {
		return "", errors.New("发布源必须使用 HTTPS")
	}
	return strings.TrimRight(base, "/") + "/", nil
}
func appUpdateName() string {
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	return "Mirroxy-update-" + platform + "-" + runtime.GOARCH + ".zip"
}
func digestBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// releaseFileURL accepts the two published layouts: a version directory below
// the release base, or the matching asset of the GitHub release. Both pin the
// expected version and file name, so a tampered catalog cannot redirect the
// download somewhere else.
func releaseFileURL(base, version, name, candidate string) bool {
	if candidate == base+"v"+version+"/"+name {
		return true
	}
	return candidate == "https://github.com/"+publicReleaseRepo+"/releases/download/v"+version+"/"+name
}
func (a *App) findAppUpdate() (*appUpdateOffer, error) {
	base, err := releaseBase()
	if err != nil {
		return nil, err
	}
	b, err := a.fetch(base+"releases.json?t="+strconv.FormatInt(time.Now().UnixNano(), 10), 4<<20, 20*time.Second)
	if err != nil {
		return nil, err
	}
	var feed struct {
		Latest   string `json:"latest"`
		Releases []struct {
			Version string          `json:"version"`
			Files   []appUpdateFile `json:"files"`
		} `json:"releases"`
	}
	if json.Unmarshal(b, &feed) != nil || !releaseVersionPattern.MatchString(feed.Latest) {
		return nil, errors.New("发布版本信息无效")
	}
	if !newerVersion(feed.Latest, version) {
		return &appUpdateOffer{Version: version}, nil
	}
	for _, release := range feed.Releases {
		if release.Version != feed.Latest {
			continue
		}
		for _, file := range release.Files {
			if file.Name != appUpdateName() {
				continue
			}
			hash, err := hex.DecodeString(file.SHA256)
			if err != nil || len(hash) != 32 || file.Size <= 0 || file.Size > 64<<20 || !releaseFileURL(base, release.Version, file.Name, file.URL) {
				return nil, errors.New("更新包来源或校验信息无效")
			}
			return &appUpdateOffer{Version: release.Version, File: file}, nil
		}
	}
	return nil, errors.New("最新版本尚未提供本平台在线更新包，请使用完整安装包")
}
func (a *App) checkAppUpdate() (AppUpdateState, error) {
	a.mu.Lock()
	busy := a.busyUpdate
	a.mu.Unlock()
	if busy {
		return AppUpdateState{}, errors.New("更新正在进行，请稍后")
	}
	offer, err := a.findAppUpdate()
	if err != nil {
		return AppUpdateState{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.busyUpdate {
		return a.appUpdate, nil
	}
	a.appUpdate = AppUpdateState{Phase: "current", Latest: offer.Version, Message: "已是最新版本"}
	if newerVersion(offer.Version, version) {
		a.appUpdate = AppUpdateState{Phase: "available", Latest: offer.Version, Available: true, Size: offer.File.Size, Message: "发现新版本，可以在线更新"}
	}
	return a.appUpdate, nil
}
func (a *App) updatePhase(phase, message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.appUpdate.Phase, a.appUpdate.Message = phase, message
	_ = a.writeState("updater", a.appUpdate)
}
func (a *App) startAppUpdate() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.busyUpdate {
		return errors.New("更新正在进行，请稍后")
	}
	a.busyUpdate = true
	a.appUpdate = AppUpdateState{Phase: "checking", Message: "正在检查新版本"}
	go func() {
		if err := a.prepareAppUpdate(); err != nil {
			a.updatePhase("error", err.Error())
			a.mu.Lock()
			a.busyUpdate = false
			a.mu.Unlock()
		}
	}()
	return nil
}
func allowedUpdateFile(name string) bool {
	if name == executableName("Mirroxy") || name == executableName("upgrade") || name == "manifest.json" {
		return true
	}
	if strings.HasPrefix(name, "bin/resources/Mirroxy_set_") && strings.HasSuffix(name, ".bin") && strings.Count(name, "/") == 2 {
		return true
	}
	return strings.HasPrefix(name, "licenses/") && strings.Count(name, "/") == 1
}
func extractAppUpdate(payload []byte, stage, expectedVersion string) (string, error) {
	z, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return "", err
	}
	files := map[string][]byte{}
	var total uint64
	for _, f := range z.File {
		if !f.Mode().IsRegular() || strings.Contains(f.Name, "\\") || !filepath.IsLocal(f.Name) || filepath.ToSlash(filepath.Clean(f.Name)) != f.Name || !allowedUpdateFile(f.Name) || files[f.Name] != nil {
			return "", errors.New("更新包包含不允许的路径")
		}
		total += f.UncompressedSize64
		if f.UncompressedSize64 > 64<<20 || total > 160<<20 {
			return "", errors.New("更新包解压大小异常")
		}
		r, err := f.Open()
		if err != nil {
			return "", err
		}
		b, err := io.ReadAll(io.LimitReader(r, 64<<20+1))
		r.Close()
		if err != nil || len(b) > 64<<20 {
			return "", errors.New("更新包损坏")
		}
		files[f.Name] = b
	}
	var manifest struct {
		Application, Version, Platform, Kind string
		Files                                map[string]string
	}
	if json.Unmarshal(files["manifest.json"], &manifest) != nil || manifest.Application != "Mirroxy" || manifest.Kind != "app-update" || manifest.Version != expectedVersion || manifest.Platform != runtime.GOOS+"-"+runtime.GOARCH || len(manifest.Files) != len(files)-1 {
		return "", errors.New("更新包版本或平台不匹配")
	}
	for name, digest := range manifest.Files {
		data, ok := files[name]
		if !ok || digestBytes(data) != digest {
			return "", errors.New("更新文件校验失败")
		}
	}
	binary := executableName("Mirroxy")
	if len(files[binary]) == 0 {
		return "", errors.New("更新包缺少主程序")
	}
	if runtime.GOOS == "windows" && len(files[executableName("upgrade")]) == 0 {
		return "", errors.New("更新包缺少独立 upgrade.exe")
	}
	for name, data := range files {
		path := filepath.Join(stage, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return "", err
		}
		mode := os.FileMode(0600)
		if name == binary || name == executableName("upgrade") {
			mode = 0700
		}
		if err := os.WriteFile(path, data, mode); err != nil {
			return "", err
		}
	}
	return filepath.Join(stage, binary), nil
}
func (a *App) prepareAppUpdate() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	if filepath.Clean(filepath.Dir(exe)) != filepath.Clean(a.root) {
		return errors.New("请从安装目录中的主程序运行在线更新")
	}
	probe, err := os.CreateTemp(a.root, ".update-permission-*")
	if err != nil {
		return errors.New("安装目录不可写，请先使用新版安装脚本升级一次，以启用在线更新")
	}
	probe.Close()
	os.Remove(probe.Name())
	offer, err := a.findAppUpdate()
	if err != nil {
		return err
	}
	if !newerVersion(offer.Version, version) {
		return errors.New("已是最新版本")
	}
	a.mu.Lock()
	a.appUpdate.Latest = offer.Version
	a.appUpdate.Size = offer.File.Size
	a.mu.Unlock()
	a.updatePhase("downloading", "正在下载并校验更新包，当前代理继续运行")
	var payload []byte
	for attempt := 0; attempt < 3; attempt++ {
		payload, err = a.fetch(offer.File.URL, 64<<20, 3*time.Minute)
		if err == nil && int64(len(payload)) == offer.File.Size && digestBytes(payload) == offer.File.SHA256 {
			break
		}
		err = errors.New("更新包下载不完整或校验失败，请稍后重试")
	}
	if err != nil {
		return err
	}
	stage := a.dataPath(filepath.Join("app-updates", newID()))
	if err = os.MkdirAll(stage, 0700); err != nil {
		return err
	}
	candidate, err := extractAppUpdate(payload, stage, offer.Version)
	if err != nil {
		return err
	}
	a.updatePhase("validating", "正在检查新版本兼容性")
	// Add new routing assets only. Independently updated cores/rules stay intact.
	if err = copyMissingRules(stage, a.root); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctxVersion := exec.CommandContext(ctx, candidate, "-version")
	hideCommand(ctxVersion)
	b, err := ctxVersion.Output()
	if err != nil || strings.TrimSpace(string(b)) != offer.Version {
		return errors.New("新版本程序检查失败，当前版本继续运行")
	}
	cmd := exec.CommandContext(ctx, candidate, "-dir", a.root, "-check")
	hideCommand(cmd)
	if err = cmd.Run(); err != nil {
		return errors.New("新版本核心兼容性检查未通过，当前版本继续运行")
	}
	a.mu.Lock()
	err = a.save()
	a.mu.Unlock()
	if err != nil {
		return err
	}
	plan := appUpdatePlan{Root: a.root, Exe: exe, Candidate: candidate, Backup: filepath.Join(stage, "previous"+filepath.Ext(exe)), Version: offer.Version, SHA256: digestBytes(mustReadFile(candidate)), Port: a.panelPort, ParentPID: os.Getpid()}
	if runtime.GOOS == "windows" {
		plan.Upgrade = filepath.Join(a.root, executableName("upgrade"))
		plan.CandidateUpgrade = filepath.Join(stage, executableName("upgrade"))
		plan.UpgradeSHA256 = digestBytes(mustReadFile(plan.CandidateUpgrade))
	}
	planPath := filepath.Join(stage, "plan.json")
	if err = writeJSON(planPath, plan); err != nil {
		return err
	}
	a.updatePhase("restarting", "下载校验完成，后台正在自动重启并恢复节点")
	if err = a.launchAppUpdate(planPath); err != nil {
		return err
	}
	a.requestQuit()
	return nil
}
func mustReadFile(path string) []byte { b, _ := os.ReadFile(path); return b }
func copyMissingRules(stage, root string) error {
	entries, err := os.ReadDir(filepath.Join(stage, "bin", "resources"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Join(root, "bin", "resources"), 0700); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return errors.New("无效规则目录")
		}
		if !strings.HasPrefix(entry.Name(), "Mirroxy_set_") || !strings.HasSuffix(entry.Name(), ".bin") {
			continue
		}
		dest := filepath.Join(root, "bin", "resources", entry.Name())
		if st, err := os.Lstat(dest); err == nil {
			if !st.Mode().IsRegular() {
				return errors.New("规则目标不是普通文件")
			}
			continue
		}
		b, err := os.ReadFile(filepath.Join(stage, "bin", "resources", entry.Name()))
		if err != nil {
			return err
		}
		f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = f.Write(b)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
func loadUpdatePlan(path string) (*appUpdatePlan, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p appUpdatePlan
	if json.Unmarshal(b, &p) != nil || !releaseVersionPattern.MatchString(p.Version) || p.Port < 1024 || p.Port > 65535 {
		return nil, errors.New("无效更新计划")
	}
	if err := validateUpdatePaths(path, &p); err != nil {
		return nil, err
	}
	if digestBytes(mustReadFile(p.Candidate)) != p.SHA256 {
		return nil, errors.New("待更新程序校验失败")
	}
	if runtime.GOOS == "windows" && digestBytes(mustReadFile(p.CandidateUpgrade)) != p.UpgradeSHA256 {
		return nil, errors.New("upgrade.exe 校验失败")
	}
	return &p, nil
}
func validateUpdatePaths(path string, p *appUpdatePlan) error {
	stage := filepath.Dir(path)
	bad := filepath.Clean(filepath.Join(p.Root, "data", "app-updates", filepath.Base(stage))) != stage || filepath.Dir(p.Exe) != p.Root || p.Candidate != filepath.Join(stage, executableName("Mirroxy")) || p.Backup != filepath.Join(stage, "previous"+filepath.Ext(p.Exe))
	if runtime.GOOS == "windows" {
		bad = bad || p.Upgrade != filepath.Join(p.Root, executableName("upgrade")) || p.CandidateUpgrade != filepath.Join(stage, executableName("upgrade"))
	}
	if bad {
		return errors.New("更新计划路径不匹配")
	}
	return nil
}
func backupUpdateData(p *appUpdatePlan) error {
	for _, name := range []string{"Mirroxy.db", "Mirroxy.key", "settings.json", "traffic.json"} {
		b, err := os.ReadFile(filepath.Join(p.Root, "data", name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(filepath.Dir(p.Backup), "rollback-"+name), b, 0600); err != nil {
			return err
		}
	}
	return nil
}
func commitUpdate(p *appUpdatePlan) error {
	if err := os.Rename(p.Exe, p.Backup); err != nil {
		return err
	}
	if err := os.Rename(p.Candidate, p.Exe); err != nil {
		_ = os.Rename(p.Backup, p.Exe)
		return err
	}
	return nil
}
func rollbackUpdate(p *appUpdatePlan) error {
	if _, err := os.Stat(p.Backup); err != nil {
		return err
	}
	if err := os.Rename(p.Exe, p.Candidate); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(p.Backup, p.Exe); err != nil {
		return err
	}
	for _, name := range []string{"Mirroxy.db", "Mirroxy.key", "settings.json", "traffic.json"} {
		b, err := os.ReadFile(filepath.Join(filepath.Dir(p.Backup), "rollback-"+name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(p.Root, "data", name), b, 0600); err != nil {
			return err
		}
	}
	return nil
}
func updateArgs(p *appUpdatePlan) []string {
	return []string{"-dir", p.Root, "-panel", fmt.Sprint(p.Port), "-no-open", "-resume-update"}
}
func (a *App) loadAppUpdate() {
	_ = a.readState("updater", &a.appUpdate)
	if a.appUpdate.Phase == "" {
		a.appUpdate.Phase = "idle"
	}
	if a.appUpdate.Phase == "restarting" && a.appUpdate.Latest == version {
		a.appUpdate = AppUpdateState{Phase: "current", Latest: version, Message: "在线更新完成，节点已恢复"}
		_ = a.writeState("updater", a.appUpdate)
	}
	if a.appUpdate.Phase == "downloading" || a.appUpdate.Phase == "validating" || a.appUpdate.Phase == "checking" {
		a.appUpdate.Phase = "error"
		a.appUpdate.Message = "上次更新已中断，可以重新检查并更新"
	}
}

// Used only by the Windows helper, after the old application has exited.
func waitUpdatedReady(p *appUpdatePlan, timeout time.Duration) bool {
	client := http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		res, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/status", p.Port))
		if err == nil {
			var state struct {
				Version string
				Nodes   []struct {
					Enabled bool
					Status  string
				}
			}
			err = json.NewDecoder(res.Body).Decode(&state)
			res.Body.Close()
			if err == nil && state.Version == p.Version {
				ready := true
				for _, n := range state.Nodes {
					if n.Enabled && n.Status != "running" {
						ready = false
					}
				}
				if ready {
					return true
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}
