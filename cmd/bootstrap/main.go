// Stable online installer helper. Contains no application payload or user data.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var feedBase = "https://ny-1s.enzonix.com/bucket-1776-2431/scrcpyproxy/"
var testBuild = "false" // Only isolated test helpers are compiled with this true.
const payloadName = "Mirroxy-windows-amd64.zip"

var versionPattern = regexp.MustCompile(`^\d{1,5}\.\d{1,5}\.\d{1,5}$`)

type fileOffer struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type offer struct {
	Version string
	File    fileOffer
}
type fetcher struct {
	base    string
	clients []*http.Client
}

func newFetcher(base, proxy string, testing bool) (*fetcher, error) {
	u, err := url.Parse(base)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Host == "" {
		return nil, errors.New("无效发布源")
	}
	loopback := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(testing && u.Scheme == "http" && loopback) {
		return nil, errors.New("发布源必须使用 HTTPS")
	}
	f := &fetcher{base: strings.TrimRight(base, "/") + "/"}
	proxies := []*url.URL{nil}
	if proxy != "" {
		p, e := url.Parse(proxy)
		if e != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Hostname() == "" || p.RawQuery != "" || p.Fragment != "" || (p.Path != "" && p.Path != "/") {
			return nil, errors.New("代理地址必须为 HTTP/HTTPS 地址")
		}
		proxies = []*url.URL{p}
	}
	for _, p := range proxies {
		transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 12 * time.Second}).DialContext, TLSHandshakeTimeout: 12 * time.Second}
		if p != nil {
			transport.Proxy = http.ProxyURL(p)
		}
		f.clients = append(f.clients, &http.Client{Transport: transport, Timeout: 4 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 5 || req.URL.Scheme != u.Scheme || req.URL.Host != u.Host || !strings.HasPrefix(req.URL.Path, u.Path) {
				return errors.New("拒绝跳转到发布源以外")
			}
			return nil
		}})
	}
	return f, nil
}
func (f *fetcher) get(address string) (*http.Response, error) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		for _, client := range f.clients {
			req, _ := http.NewRequest("GET", address, nil)
			req.Header.Set("User-Agent", "MirroxyOnlineInstaller/1.0.0")
			req.Header.Set("Cache-Control", "no-cache")
			res, err := client.Do(req)
			if err != nil {
				if testBuild == "true" {
					fmt.Fprintln(os.Stderr, "Isolated test request:", err)
				}
				continue
			}
			if res.StatusCode != 200 {
				if testBuild == "true" {
					fmt.Fprintln(os.Stderr, "Isolated test HTTP status:", res.StatusCode)
				}
				res.Body.Close()
				continue
			}
			return res, nil
		}
	}
	return nil, errors.New("无法连接发布服务，请检查网络或代理")
}
func (f *fetcher) latest() (offer, error) {
	var selected offer
	res, err := f.get(f.base + "releases.json?t=" + fmt.Sprint(time.Now().UnixNano()))
	if err != nil {
		return selected, err
	}
	defer res.Body.Close()
	var catalog struct {
		Latest   string `json:"latest"`
		Releases []struct {
			Version string      `json:"version"`
			Files   []fileOffer `json:"files"`
		} `json:"releases"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&catalog); err != nil || !versionPattern.MatchString(catalog.Latest) {
		return selected, errors.New("发布清单格式或版本号无效")
	}
	for _, release := range catalog.Releases {
		if release.Version != catalog.Latest {
			continue
		}
		for _, file := range release.Files {
			if file.Name != payloadName {
				continue
			}
			hash, e := hex.DecodeString(file.SHA256)
			if e != nil || len(hash) != 32 || file.Size <= 0 || file.Size > 256<<20 || file.URL != f.base+"v"+release.Version+"/"+payloadName {
				return selected, errors.New("Windows 包来源、大小或 SHA256 无效")
			}
			return offer{release.Version, file}, nil
		}
	}
	return selected, errors.New("服务端最新版本尚未提供 Windows 在线安装 ZIP，请先发布新版；本机文件未修改")
}
func (f *fetcher) download(item offer, work string) (string, error) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		res, err := f.get(item.File.URL)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		file, err := os.CreateTemp(work, "payload-*.zip")
		if err != nil {
			return "", err
		}
		path := file.Name()
		hash := sha256.New()
		count, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(res.Body, item.File.Size+1))
		ce := file.Close()
		if err != nil || ce != nil || count != item.File.Size || hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(item.File.SHA256) {
			if testBuild == "true" {
				fmt.Fprintf(os.Stderr, "Isolated test transport: copy=%v close=%v\n", err, ce)
			}
			_ = os.Remove(path)
			if attempt < 2 && ce == nil && (err != nil || count < item.File.Size) {
				continue
			}
			return "", fmt.Errorf("Windows 程序包下载不完整或 SHA256 校验失败（收到 %d / %d 字节）；未修改安装目录", count, item.File.Size)
		}
		return path, nil
	}
	return "", errors.New("下载重试失败，未修改安装目录")
}
func allowed(name string) bool {
	switch name {
	case "Mirroxy.exe", "upgrade.exe", "README.txt", "PLATFORM", "manifest.json", "SHA256SUMS", "bin/runtime/Mirroxy_core_A.exe", "bin/runtime/Mirroxy_core_B.exe", "bin/runtime/Mirroxy_downloader.exe", "bin/resources/Mirroxy_route_ip.bin", "bin/resources/Mirroxy_route_domain.bin", "bin/resources/Mirroxy_trust.pem":
		return true
	}
	for i := 1; i <= 12; i++ {
		if name == fmt.Sprintf("bin/resources/Mirroxy_set_%02d.bin", i) {
			return true
		}
	}
	if strings.HasPrefix(name, "licenses/") && strings.Count(name, "/") == 1 {
		return regexp.MustCompile(`^licenses/[A-Za-z0-9._-]+$`).MatchString(name)
	}
	return false
}
func amd64PE(data []byte) bool {
	if len(data) < 64 || string(data[:2]) != "MZ" {
		return false
	}
	offset := int(binary.LittleEndian.Uint32(data[60:64]))
	return offset >= 64 && offset+6 <= len(data) && string(data[offset:offset+4]) == "PE\x00\x00" && binary.LittleEndian.Uint16(data[offset+4:offset+6]) == 0x8664
}
func extract(path, work, version string) (string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", errors.New("程序 ZIP 无法打开")
	}
	defer archive.Close()
	files := map[string][]byte{}
	var total uint64
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimPrefix(entry.Name, "Mirroxy-windows-amd64/")
		if !entry.Mode().IsRegular() || strings.Contains(name, "\\") || !filepath.IsLocal(filepath.FromSlash(name)) || filepath.ToSlash(filepath.Clean(filepath.FromSlash(name))) != name || !allowed(name) || files[name] != nil {
			return "", errors.New("程序 ZIP 包含不允许的路径或文件")
		}
		total += entry.UncompressedSize64
		if entry.UncompressedSize64 > 128<<20 || total > 256<<20 {
			return "", errors.New("程序 ZIP 解压大小超出限制")
		}
		reader, e := entry.Open()
		if e != nil {
			return "", e
		}
		data, e := io.ReadAll(io.LimitReader(reader, 128<<20+1))
		reader.Close()
		if e != nil || len(data) > 128<<20 {
			return "", errors.New("程序 ZIP 内容损坏")
		}
		files[name] = data
	}
	var manifest struct {
		Application, Version, Platform string
		Files                          map[string]string
	}
	if json.Unmarshal(files["manifest.json"], &manifest) != nil || manifest.Application != "Mirroxy" || manifest.Version != version || manifest.Platform != "windows-amd64" || len(manifest.Files) != len(files)-2 {
		return "", errors.New("程序清单版本、平台或文件数量不匹配")
	}
	for name, digest := range manifest.Files {
		data, ok := files[name]
		hash := sha256.Sum256(data)
		if !ok || hex.EncodeToString(hash[:]) != digest {
			return "", errors.New("程序 ZIP 内部文件校验失败")
		}
	}
	for _, name := range []string{"Mirroxy.exe", "upgrade.exe", "bin/runtime/Mirroxy_core_A.exe", "bin/runtime/Mirroxy_core_B.exe", "bin/runtime/Mirroxy_downloader.exe"} {
		if !amd64PE(files[name]) {
			return "", errors.New("Windows AMD64 程序或引擎缺失/平台错误")
		}
	}
	for _, name := range []string{"README.txt", "PLATFORM", "SHA256SUMS", "bin/resources/Mirroxy_route_ip.bin", "bin/resources/Mirroxy_route_domain.bin", "bin/resources/Mirroxy_trust.pem"} {
		if len(files[name]) == 0 {
			return "", errors.New("完整安装资源缺失")
		}
	}
	for i := 1; i <= 12; i++ {
		if len(files[fmt.Sprintf("bin/resources/Mirroxy_set_%02d.bin", i)]) == 0 {
			return "", errors.New("路由规则资源缺失")
		}
	}
	destination, err := os.MkdirTemp(work, "payload-stage-*")
	if err != nil {
		return "", err
	}
	for name, data := range files {
		target := filepath.Join(destination, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", err
		}
		file, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return "", e
		}
		_, e = file.Write(data)
		ce := file.Close()
		if e != nil {
			return "", e
		}
		if ce != nil {
			return "", ce
		}
	}
	return destination, nil
}
func run(work, proxy string) error {
	if !filepath.IsAbs(work) {
		return errors.New("缓存目录必须是绝对路径")
	}
	if err := os.MkdirAll(work, 0700); err != nil {
		return err
	}
	f, err := newFetcher(feedBase, proxy, testBuild == "true")
	if err != nil {
		return err
	}
	item, err := f.latest()
	if err != nil {
		return err
	}
	fmt.Println("正在下载并校验 Mirroxy " + item.Version)
	path, err := f.download(item, work)
	if err != nil {
		return err
	}
	stage, err := extract(path, work, item.Version)
	if err != nil {
		return err
	}
	if strings.ContainsAny(stage, "\r\n") {
		return errors.New("缓存路径包含非法字符")
	}
	plan := fmt.Sprintf("[release]\r\nversion=%s\r\nsha256=%s\r\npayload=%s\r\n", item.Version, item.File.SHA256, stage)
	return os.WriteFile(filepath.Join(work, "release.ini"), []byte(plan), 0600)
}
func main() {
	work := flag.String("work", "", "Temporary installer cache directory")
	proxy := flag.String("proxy", "", "Explicit HTTP proxy; empty is direct")
	flag.Parse()
	if err := run(*work, *proxy); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
