package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Geo rule assets shipped under bin/resources. The xray databases keep the
// ext:/ext-ip: references built by routing.go; the sing-box sets are the
// local binary rule sets listed by singRuleSets.
type geoSpec struct {
	name     string // rule name in ruleAssetName
	engine   string // "xray" or "sing-box"
	asset    string // file name under bin/resources
	kind     string // "dat" (xray database) or "srs" (sing-box binary rule set)
	source   string // download URL
	checksum string // optional sha256sum URL
	purpose  string // zh description shown in the panel
}

var geoCatalog = []geoSpec{
	{name: "geoip", engine: "xray", asset: "Mirroxy_route_ip.bin", kind: "dat", purpose: "IP 归属数据库：局域网与大陆直连、黑名单 IP 组判断的底层数据", source: "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat", checksum: "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat.sha256sum"},
	{name: "geosite", engine: "xray", asset: "Mirroxy_route_domain.bin", kind: "dat", purpose: "域名分类数据库：局域网与大陆直连、黑名单域名组判断的底层数据", source: "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat", checksum: "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat.sha256sum"},
	{name: "geoip-cn", engine: "sing-box", asset: "Mirroxy_set_01.bin", kind: "srs", purpose: "大陆 IP 段：绕过大陆模式下直连", source: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/cn.srs"},
	{name: "geosite-cn", engine: "sing-box", asset: "Mirroxy_set_02.bin", kind: "srs", purpose: "大陆域名：绕过大陆模式下直连", source: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/cn.srs"},
	{name: "geosite-private", engine: "sing-box", asset: "Mirroxy_set_03.bin", kind: "srs", purpose: "局域网与内网域名：各模式下直连", source: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/private.srs"},
	{name: "geosite-gfw", engine: "sing-box", asset: "Mirroxy_set_04.bin", kind: "srs", purpose: "GFW 列表：黑名单模式下走代理", source: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/gfw.srs"},
	{name: "geosite-greatfire", engine: "sing-box", asset: "Mirroxy_set_05.bin", kind: "srs", purpose: "GreatFire 列表：黑名单模式下走代理", source: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/greatfire.srs"},
	{name: "geosite-google", engine: "sing-box", asset: "Mirroxy_set_06.bin", kind: "srs", purpose: "Google 域名：黑名单模式下走代理", source: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/google.srs"},
	{name: "geoip-facebook", engine: "sing-box", asset: "Mirroxy_set_07.bin", kind: "srs", purpose: "Facebook IP 段：黑名单模式下走代理", source: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/facebook.srs"},
	{name: "geoip-fastly", engine: "sing-box", asset: "Mirroxy_set_08.bin", kind: "srs", purpose: "Fastly IP 段：黑名单模式下走代理", source: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/fastly.srs"},
	{name: "geoip-google", engine: "sing-box", asset: "Mirroxy_set_09.bin", kind: "srs", purpose: "Google IP 段：黑名单模式下走代理", source: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/google.srs"},
	{name: "geoip-netflix", engine: "sing-box", asset: "Mirroxy_set_10.bin", kind: "srs", purpose: "Netflix IP 段：黑名单模式下走代理", source: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/netflix.srs"},
	{name: "geoip-telegram", engine: "sing-box", asset: "Mirroxy_set_11.bin", kind: "srs", purpose: "Telegram IP 段：黑名单模式下走代理", source: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/telegram.srs"},
	{name: "geoip-twitter", engine: "sing-box", asset: "Mirroxy_set_12.bin", kind: "srs", purpose: "Twitter IP 段：黑名单模式下走代理", source: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/twitter.srs"},
}

const geoReleaseAPI = "https://api.github.com/repos/Loyalsoldier/v2ray-rules-dat/releases/latest"

// Overridden in tests to avoid live GitHub access.
var geoReleaseEndpoint = geoReleaseAPI

func geoVersionKey(name string) string { return "geo:" + name }

// geoTagVersion maps the upstream release tag to the stored version marker.
// The .srs sources carry no versions of their own, so they piggyback on the
// dat release date that upstream refreshes in lockstep.
func geoTagVersion(spec geoSpec, tag string) string {
	if tag == "" {
		return ""
	}
	if spec.kind == "srs" {
		return "srs@" + tag
	}
	return tag
}

func resolveGeoSpecs(names []string) ([]geoSpec, error) {
	if len(names) == 0 {
		return append([]geoSpec{}, geoCatalog...), nil
	}
	byName := map[string]geoSpec{}
	for _, spec := range geoCatalog {
		byName[spec.name] = spec
	}
	out := []geoSpec{}
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		spec, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("未知的规则文件：%s", name)
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, spec)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("请选择要更新的规则文件")
	}
	return out, nil
}

func (a *App) geoAssetPath(spec geoSpec) string {
	return filepath.Join(a.root, "bin", "resources", spec.asset)
}

func (a *App) lockedEvent(message string) {
	a.mu.Lock()
	a.event(message)
	a.mu.Unlock()
}

// Routing modes that currently reference geo assets: the global mode plus
// every effective per-node override.
func (a *App) geoRoutingModes() map[string]bool {
	modes := map[string]bool{first(a.cfg.RoutingMode, "bypass-cn"): true}
	for i := range a.cfg.Nodes {
		modes[a.effectiveRoute(a.cfg.Nodes[i])] = true
	}
	return modes
}

func geoFileUsed(modes map[string]bool, name string) bool {
	contains := func(list []string) bool {
		for _, item := range list {
			if item == name {
				return true
			}
		}
		return false
	}
	switch {
	case modes["bypass-cn"]:
		return name == "geoip" || name == "geosite" || contains([]string{"geoip-cn", "geosite-cn", "geosite-private"})
	case modes["blacklist"]:
		return name == "geoip" || name == "geosite" || contains(blacklistSiteSets) || contains(blacklistIPSets)
	case modes["proxy"]:
		return name == "geoip" || name == "geosite" || name == "geosite-private"
	}
	return false
}

func (a *App) geoFilesState() []M {
	modes := a.geoRoutingModes()
	out := []M{}
	for _, spec := range geoCatalog {
		info, err := os.Stat(a.geoAssetPath(spec))
		item := M{"name": spec.name, "engine": corePublicID(spec.engine), "asset": spec.asset, "kind": spec.kind, "purpose": spec.purpose, "exists": err == nil, "used": geoFileUsed(modes, spec.name), "version": a.cfg.Updated[geoVersionKey(spec.name)]}
		if err == nil {
			item["size"] = info.Size()
			item["updatedAt"] = info.ModTime().Format("2006-01-02 15:04")
		}
		out = append(out, item)
	}
	return out
}

// geoReleaseTag returns the upstream release date of the xray rule databases.
func (a *App) geoReleaseTag() (string, error) {
	b, e := a.fetch(geoReleaseEndpoint, 4<<20, 20*time.Second)
	if e != nil {
		return "", e
	}
	var r Release
	if json.Unmarshal(b, &r) != nil || r.Tag == "" {
		return "", errors.New("Geo 规则源版本信息无效")
	}
	return r.Tag, nil
}

func geoMinSize(spec geoSpec) int64 {
	if spec.kind == "dat" {
		return 1 << 20
	}
	return 512
}

func (a *App) verifyGeoChecksum(spec geoSpec, payload []byte) error {
	b, e := a.fetch(spec.checksum, 1<<20, 60*time.Second)
	if e != nil {
		return fmt.Errorf("校验值下载失败：%w", e)
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return errors.New("校验值格式无效")
	}
	sum := sha256.Sum256(payload)
	if !strings.EqualFold(fields[0], hex.EncodeToString(sum[:])) {
		return errors.New("SHA-256 校验失败，已丢弃下载内容")
	}
	return nil
}

// validateGeoPayload loads the candidate with the bundled core binary before
// it replaces the installed file. Cores that are not installed skip strict
// validation; size limits and checksums still apply.
func (a *App) validateGeoPayload(spec geoSpec, payload []byte) error {
	if spec.kind == "srs" {
		exe := a.corePath("sing-box")
		if _, e := os.Stat(exe); e != nil {
			return nil
		}
		dir, e := os.MkdirTemp(a.dataPath(""), "geo-check-")
		if e != nil {
			return e
		}
		defer os.RemoveAll(dir)
		candidate := filepath.Join(dir, "candidate.srs")
		if e = os.WriteFile(candidate, payload, 0600); e != nil {
			return e
		}
		probe := filepath.Join(dir, "probe.json")
		cfg := M{"log": M{"level": "error"}, "outbounds": []any{M{"type": "direct", "tag": "direct"}}, "route": M{"rules": []any{M{"rule_set": []string{"geo-probe"}, "outbound": "direct"}}, "rule_set": []any{M{"type": "local", "tag": "geo-probe", "format": "binary", "path": candidate}}}}
		if e = writeJSON(probe, cfg); e != nil {
			return e
		}
		return a.validateCore("sing-box", probe, exe)
	}
	exe := a.corePath("xray")
	if _, e := os.Stat(exe); e != nil {
		return nil
	}
	dir, e := os.MkdirTemp(a.dataPath(""), "geo-check-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	asset := filepath.Join(dir, ruleAssetName(spec.name))
	if e = os.WriteFile(asset, payload, 0600); e != nil {
		return e
	}
	key, ref := "domain", xDomain("private")
	if spec.name == "geoip" {
		key, ref = "ip", xIP("private")
	}
	probe := filepath.Join(dir, "probe.json")
	cfg := M{"log": M{"loglevel": "error"}, "inbounds": []any{M{"listen": "127.0.0.1", "port": 19603, "protocol": "socks", "settings": M{}}}, "outbounds": []any{M{"protocol": "freedom", "tag": "direct"}}, "routing": M{"rules": []any{M{"type": "field", key: []string{ref}, "outboundTag": "direct"}}}}
	if e = writeJSON(probe, cfg); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "run", "-test", "-config", probe)
	hideCommand(cmd)
	cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+dir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		if len(message) > 1500 {
			message = message[len(message)-1500:]
		}
		return fmt.Errorf("Xray 校验新规则文件失败：%s", message)
	}
	return nil
}

func (a *App) swapGeoFile(spec geoSpec, payload []byte) error {
	dest := a.geoAssetPath(spec)
	if e := os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
		return e
	}
	next := dest + ".next"
	if e := os.WriteFile(next, payload, 0600); e != nil {
		return e
	}
	backup := dest + ".previous"
	_ = os.Remove(backup)
	hadOld := false
	if _, err := os.Stat(dest); err == nil {
		if err = os.Rename(dest, backup); err != nil {
			_ = os.Remove(next)
			return err
		}
		hadOld = true
	}
	if e := os.Rename(next, dest); e != nil {
		if hadOld {
			_ = os.Rename(backup, dest)
		}
		return e
	}
	return nil
}

func (a *App) rollbackGeoFile(spec geoSpec) {
	dest := a.geoAssetPath(spec)
	backup := dest + ".previous"
	if _, e := os.Stat(backup); e != nil {
		return
	}
	_ = os.Remove(dest)
	_ = os.Rename(backup, dest)
}

// updateGeoFiles refreshes the named rule assets (all when names is empty).
// skipUnchanged is used by the daily auto-update: when the upstream release
// tag matches the stored version nothing is downloaded or restarted.
func (a *App) updateGeoFiles(names []string, skipUnchanged bool) error {
	specs, err := resolveGeoSpecs(names)
	if err != nil {
		return err
	}
	a.healthGate.Lock()
	defer a.healthGate.Unlock()
	a.mu.Lock()
	if a.busyUpdate {
		a.mu.Unlock()
		return errors.New("已有更新正在进行，请稍后再试")
	}
	a.busyUpdate = true
	a.event(fmt.Sprintf("开始更新 Geo 规则文件（%d 个），完成后对应内核会短暂重启", len(specs)))
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.busyUpdate = false; a.mu.Unlock() }()

	tag, tagErr := a.geoReleaseTag()
	if tagErr != nil {
		tag = ""
		if skipUnchanged {
			a.lockedEvent("Geo 规则文件自动更新跳过：暂时无法获取远程版本（" + tagErr.Error() + "）")
			return nil
		}
	}
	a.mu.Lock()
	current := map[string]string{}
	for _, spec := range specs {
		current[spec.name] = a.cfg.Updated[geoVersionKey(spec.name)]
	}
	a.mu.Unlock()

	type geoCandidate struct {
		spec      geoSpec
		payload   []byte
		version   string
		identical bool
	}
	ready := []geoCandidate{}
	var failures []string
	for _, spec := range specs {
		if skipUnchanged && tag != "" && current[spec.name] == geoTagVersion(spec, tag) {
			continue
		}
		payload, e := a.fetch(spec.source, 256<<20, 120*time.Second)
		if e != nil {
			failures = append(failures, spec.name+"："+e.Error())
			a.lockedEvent("Geo 规则文件 " + spec.name + " 下载失败：" + e.Error())
			continue
		}
		if int64(len(payload)) < geoMinSize(spec) {
			e = errors.New("下载内容过小，已丢弃")
			failures = append(failures, spec.name+"："+e.Error())
			a.lockedEvent("Geo 规则文件 " + spec.name + " " + e.Error())
			continue
		}
		if spec.checksum != "" {
			if e = a.verifyGeoChecksum(spec, payload); e != nil {
				failures = append(failures, spec.name+"："+e.Error())
				a.lockedEvent("Geo 规则文件 " + spec.name + " " + e.Error())
				continue
			}
		}
		if e = a.validateGeoPayload(spec, payload); e != nil {
			failures = append(failures, spec.name+"："+e.Error())
			a.lockedEvent("Geo 规则文件 " + spec.name + " " + e.Error())
			continue
		}
		version := geoTagVersion(spec, tag)
		if version == "" {
			version = time.Now().Format("2006-01-02 15:04")
		}
		candidate := geoCandidate{spec: spec, payload: payload, version: version}
		if old, e := os.ReadFile(a.geoAssetPath(spec)); e == nil {
			sum, newSum := sha256.Sum256(old), sha256.Sum256(payload)
			candidate.identical = hex.EncodeToString(sum[:]) == hex.EncodeToString(newSum[:])
		}
		ready = append(ready, candidate)
	}
	if len(ready) == 0 {
		if len(failures) > 0 {
			return errors.New(strings.Join(failures, "；"))
		}
		a.lockedEvent("Geo 规则文件已是最新，无需更新")
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	engines := map[string]bool{}
	for _, candidate := range ready {
		engines[candidate.spec.engine] = true
	}
	for engine := range engines {
		a.stopEngine(engine)
	}
	swapped := []geoCandidate{}
	for _, candidate := range ready {
		if candidate.identical {
			continue
		}
		if e := a.swapGeoFile(candidate.spec, candidate.payload); e != nil {
			failures = append(failures, candidate.spec.name+"："+e.Error())
			a.event("Geo 规则文件 " + candidate.spec.name + " 替换失败：" + e.Error())
			continue
		}
		swapped = append(swapped, candidate)
	}
	okEngines := map[string]bool{}
	for engine := range engines {
		if e := a.reconcileEngine(engine); e != nil {
			a.event(coreDisplayName(engine) + " 加载新规则失败：" + e.Error())
			for _, candidate := range swapped {
				if candidate.spec.engine == engine {
					a.rollbackGeoFile(candidate.spec)
				}
			}
			if err := a.reconcileEngine(engine); err != nil {
				a.event(coreDisplayName(engine) + " 恢复原规则失败：" + err.Error())
			}
			failures = append(failures, coreDisplayName(engine)+" 加载新规则失败，已恢复原文件")
			continue
		}
		okEngines[engine] = true
	}
	updated := []string{}
	for _, candidate := range ready {
		if !candidate.identical && !okEngines[candidate.spec.engine] {
			continue
		}
		a.cfg.Updated[geoVersionKey(candidate.spec.name)] = candidate.version
		updated = append(updated, candidate.spec.name)
	}
	saveErr := a.save()
	if len(updated) > 0 {
		a.event("Geo 规则文件已更新：" + strings.Join(updated, "、"))
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "；"))
	}
	return saveErr
}
