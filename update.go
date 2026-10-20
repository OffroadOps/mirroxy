package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Release struct {
	Tag    string `json:"tag_name"`
	Draft  bool   `json:"draft"`
	Assets []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"assets"`
}

func newerVersion(candidate, installed string) bool {
	parse := func(s string) []int {
		parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
		if len(parts) < 2 {
			return nil
		}
		out := []int{}
		for _, part := range parts {
			digits := strings.TrimRightFunc(part, func(r rune) bool { return r < '0' || r > '9' })
			n, e := strconv.Atoi(digits)
			if e != nil {
				return nil
			}
			out = append(out, n)
		}
		return out
	}
	next, old := parse(candidate), parse(installed)
	if next == nil {
		return false
	}
	if old == nil {
		return true
	}
	length := len(next)
	if len(old) > length {
		length = len(old)
	}
	for i := 0; i < length; i++ {
		x, y := 0, 0
		if i < len(next) {
			x = next[i]
		}
		if i < len(old) {
			y = old[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func (a *App) fetch(address string, limit int64, timeout time.Duration) ([]byte, error) {
	a.mu.Lock()
	proxies := []string{}
	for _, n := range a.cfg.Nodes {
		if n.Enabled && a.cores[n.coreKey()].alive() {
			proxies = append(proxies, fmt.Sprintf("http://127.0.0.1:%d", n.Port))
			break
		}
	}
	a.mu.Unlock()
	en, system := currentProxy()
	if en == 1 && strings.HasPrefix(system, "127.0.0.1:") {
		p := "http://" + system
		if len(proxies) == 0 || proxies[0] != p {
			proxies = append(proxies, p)
		}
	}
	proxies = append(proxies, "")
	for _, proxy := range proxies {
		transport := &http.Transport{DisableKeepAlives: true}
		if proxy != "" {
			u, _ := url.Parse(proxy)
			transport.Proxy = http.ProxyURL(u)
		}
		client := &http.Client{Transport: transport, Timeout: timeout}
		req, e := http.NewRequest("GET", address, nil)
		if e != nil {
			return nil, errors.New("无效下载地址")
		}
		req.Header.Set("User-Agent", "Mirroxy/"+version)
		req.Header.Set("Accept", "application/vnd.github+json, */*")
		res, e := client.Do(req)
		if e != nil {
			transport.CloseIdleConnections()
			continue
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			transport.CloseIdleConnections()
			continue
		}
		b, e := io.ReadAll(io.LimitReader(res.Body, limit+1))
		res.Body.Close()
		transport.CloseIdleConnections()
		if e != nil {
			continue
		}
		if int64(len(b)) > limit {
			return nil, errors.New("下载内容超过大小限制")
		}
		return b, nil
	}
	return nil, errors.New("下载失败，请检查代理连通性或稍后重试")
}
func (a *App) latestRelease(engine string) (*Release, error) {
	repo := ""
	switch engine {
	case "xray":
		repo = "XTLS/Xray-core"
	case "sing-box":
		repo = "SagerNet/sing-box"
	default:
		return nil, errors.New("不支持的核心")
	}
	endpoint := "https://api.github.com/repos/" + repo + "/releases/latest"
	if engine == "xray" {
		endpoint = "https://api.github.com/repos/" + repo + "/releases?per_page=20"
	}
	b, e := a.fetch(endpoint, 4<<20, 20*time.Second)
	if e != nil {
		return nil, e
	}
	var r Release
	if engine == "xray" {
		var releases []Release
		if json.Unmarshal(b, &releases) != nil {
			return nil, errors.New("官方版本信息无效")
		}
		for _, candidate := range releases {
			if !candidate.Draft && candidate.Tag != "" && (r.Tag == "" || newerVersion(candidate.Tag, r.Tag)) {
				r = candidate
			}
		}
	} else if json.Unmarshal(b, &r) != nil {
		return nil, errors.New("官方版本信息无效")
	}
	if r.Tag == "" {
		return nil, errors.New("官方版本信息无效")
	}
	return &r, nil
}
func coreVersion(path, engine string) string {
	args := []string{"version"}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	hideCommand(cmd)
	b, e := cmd.Output()
	if e != nil {
		return "未检测到"
	}
	fields := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
	if engine == "sing-box" && len(fields) > 2 {
		return fields[2]
	}
	if len(fields) > 1 {
		return fields[1]
	}
	return "未知"
}
func (a *App) updateCore(engine string) error {
	// Temporary health cores must release the executable before an update replaces it.
	a.healthGate.Lock()
	defer a.healthGate.Unlock()
	if engine != "xray" && engine != "sing-box" {
		return errors.New("不支持的核心")
	}
	a.mu.Lock()
	if engine == "sing-box" && (a.appRouter != nil || a.appRouterPhase == "starting") {
		a.mu.Unlock()
		return errors.New("请先停止应用代理再更新网络核心 B")
	}
	if a.busyUpdate {
		a.mu.Unlock()
		return errors.New("已有核心正在更新")
	}
	a.busyUpdate = true
	a.event("正在检查 " + coreDisplayName(engine) + " 官方更新")
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.busyUpdate = false; a.mu.Unlock() }()
	release, e := a.latestRelease(engine)
	if e != nil {
		return e
	}
	current := coreVersion(a.corePath(engine), engine)
	if !newerVersion(release.Tag, current) {
		a.mu.Lock()
		a.cfg.Updated[engine] = current
		_ = a.save()
		a.event(coreDisplayName(engine) + " 本机版本已是最新或高于官方返回版本，保留现有核心")
		a.mu.Unlock()
		return nil
	}
	assetName, e := coreAssetName(engine, release.Tag, runtime.GOOS, runtime.GOARCH)
	if e != nil {
		return e
	}
	var address, digest string
	for _, asset := range release.Assets {
		match := asset.Name == assetName
		if match {
			address = asset.URL
			digest = asset.Digest
			break
		}
	}
	if address == "" {
		return fmt.Errorf("此版本没有匹配的 %s/%s 核心包", runtime.GOOS, runtime.GOARCH)
	}
	if !strings.HasPrefix(digest, "sha256:") {
		return errors.New("官方接口未提供 SHA-256 校验值，已保留现有核心")
	}
	a.mu.Lock()
	a.event("正在下载 " + coreDisplayName(engine) + " " + release.Tag)
	a.mu.Unlock()
	archive, e := a.fetch(address, 160<<20, 90*time.Second)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != strings.TrimPrefix(digest, "sha256:") {
		return errors.New("下载校验失败，已保留现有核心")
	}
	exe := a.corePath(engine)
	candidate := exe + ".next"
	if runtime.GOOS == "windows" {
		candidate += ".exe"
	}
	upstreamBase := "xray"
	if engine == "sing-box" {
		upstreamBase = "sing-box"
	}
	if runtime.GOOS == "windows" {
		upstreamBase += ".exe"
	}
	payload, e := extractCorePayload(archive, upstreamBase)
	if e != nil {
		return e
	}
	if len(payload) == 0 {
		return errors.New("更新包中未找到核心程序")
	}
	if e = os.MkdirAll(filepath.Dir(exe), 0700); e != nil {
		return e
	}
	if e = os.WriteFile(candidate, payload, 0700); e != nil {
		return e
	}
	defer os.Remove(candidate)
	a.mu.Lock()
	defer a.mu.Unlock()
	checkPath := a.dataPath(filepath.Join("runtime", "Mirroxy_update_check.cfg"))
	defer os.Remove(checkPath)
	keys := a.coreKeys()
	if engine == "sing-box" {
		keys = append(keys, "sing-box")
	}
	for _, key := range keys {
		if coreEngine(key) != engine {
			continue
		}
		cfg, _ := a.coreConfig(key)
		if e = writeJSON(checkPath, cfg); e != nil {
			return e
		}
		if e = a.validateCore(engine, checkPath, candidate); e != nil {
			return fmt.Errorf("新版不兼容现有配置，未替换：%w", e)
		}
	}
	a.stopEngine(engine)
	backup := exe + ".previous"
	_ = os.Remove(backup)
	hadOld := false
	if _, err := os.Stat(exe); err == nil {
		if err = os.Rename(exe, backup); err != nil {
			_ = a.reconcileEngine(engine)
			return err
		}
		hadOld = true
	}
	if e = os.Rename(candidate, exe); e == nil {
		e = a.reconcileEngine(engine)
	}
	if e != nil {
		a.stopEngine(engine)
		_ = os.Remove(exe)
		if hadOld {
			_ = os.Rename(backup, exe)
		}
		restoreErr := a.reconcileEngine(engine)
		if restoreErr != nil {
			a.event("恢复核心失败：" + restoreErr.Error())
		}
		return fmt.Errorf("新版启动失败，已尝试恢复旧核心：%w", e)
	}
	a.cfg.Updated[engine] = coreVersion(exe, engine)
	e = a.save()
	a.event(coreDisplayName(engine) + " 已更新至 " + release.Tag)
	return e
}
