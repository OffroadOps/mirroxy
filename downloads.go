package main

import (
	"archive/zip"
	"bytes"
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
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The encrypted journal, not aria2's plaintext session file, owns task history.
// RPC is private, loopback-only, random-port and authenticated. No external RPC
// endpoint/options pass-through is accepted from the panel.
type DownloadSpec struct {
	URL         string `json:"url"`
	Name        string `json:"name"`
	Dir         string `json:"dir"`
	ProxyMode   string `json:"proxyMode"`
	NodeID      string `json:"nodeId"`
	ProxyURL    string `json:"proxyUrl"`
	Connections int    `json:"connections"`
	LimitKiB    int    `json:"limitKiB"`
}
type DownloadTask struct {
	ID string `json:"id"`
	DownloadSpec
	Status          string `json:"status"`
	Wanted          bool   `json:"wanted"`
	Created         string `json:"created"`
	Total           string `json:"totalLength"`
	Completed       string `json:"completedLength"`
	Speed           string `json:"downloadSpeed"`
	ConnectionsUsed string `json:"connectionsUsed"`
	Error           string `json:"error"`
	Path            string `json:"path"`
	GID             string `json:"-"`
	Stalled         bool   `json:"stalled,omitempty"`
	noProgressSince time.Time
	lastCompleted   string
}
type downloadJournal struct {
	Tasks      []*DownloadTask `json:"tasks"`
	DefaultDir string          `json:"defaultDir"`
}
type DownloadManager struct {
	mu               sync.Mutex
	a                *App
	journal          downloadJournal
	cmd              *exec.Cmd
	done             chan struct{}
	endpoint, secret string
	closing          bool
	client           *http.Client
	openTarget       func(string, bool) error
}

func (a *App) downloads() (*DownloadManager, error) {
	a.downloadInitMu.Lock()
	defer a.downloadInitMu.Unlock()
	if a.downloadManager != nil {
		return a.downloadManager, nil
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		home = a.root
	}
	d := &DownloadManager{a: a, journal: downloadJournal{Tasks: []*DownloadTask{}, DefaultDir: filepath.Join(home, "Downloads")}, client: &http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{Proxy: nil}}}
	if err := a.readState("downloads", &d.journal); err != nil && !errors.Is(err, errStateMissing) {
		return nil, err
	}
	if d.journal.Tasks == nil {
		d.journal.Tasks = []*DownloadTask{}
	}
	for _, task := range d.journal.Tasks {
		task.Speed = "0"
		task.GID = ""
	}
	a.downloadManager = d
	return d, nil
}
func (d *DownloadManager) binary() string {
	path := filepath.Join(d.a.root, "bin", "runtime", executableName("Mirroxy_downloader"))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path
	}
	path, _ = exec.LookPath("aria2c")
	return path
}

func (d *DownloadManager) install() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closing {
		return errors.New("后台正在退出")
	}
	if d.binary() != "" {
		return nil
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return errors.New("请通过系统包管理器安装 aria2（aria2c）")
	}
	payload, err := d.a.fetch("https://github.com/aria2/aria2/releases/download/release-1.37.0/aria2-1.37.0-win-64bit-build1.zip", 8<<20, 90*time.Second)
	if err != nil {
		return errors.New("无法下载官方 aria2，请检查网络或安装完整安装包")
	}
	if digestBytes(payload) != "67d015301eef0b612191212d564c5bb0a14b5b9c4796b76454276a4d28d9b288" {
		return errors.New("aria2 官方下载包校验失败，未安装")
	}
	archive, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return err
	}
	var engine []byte
	licenses := map[string][]byte{}
	for _, file := range archive.File {
		name := filepath.Base(file.Name)
		if name != "aria2c.exe" && name != "COPYING" && name != "LICENSE.OpenSSL" {
			continue
		}
		reader, e := file.Open()
		if e != nil {
			return e
		}
		content, e := io.ReadAll(io.LimitReader(reader, 16<<20))
		reader.Close()
		if e != nil {
			return e
		}
		if name == "aria2c.exe" {
			engine = content
		} else {
			licenses[name] = content
		}
	}
	if len(engine) == 0 {
		return errors.New("aria2 下载包缺少引擎")
	}
	licenseDir := filepath.Join(d.a.root, "licenses")
	if err = os.MkdirAll(licenseDir, 0755); err != nil {
		return err
	}
	licenses["NOTICE.txt"] = []byte("aria2 1.37.0: unmodified standalone executable, GPL-2.0-or-later.\nOfficial release: https://github.com/aria2/aria2/releases/tag/release-1.37.0\nCorresponding source: https://github.com/aria2/aria2/releases/download/release-1.37.0/aria2-1.37.0.tar.xz\n")
	for name, content := range licenses {
		file, e := os.OpenFile(filepath.Join(licenseDir, "ARIA2-"+name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if errors.Is(e, os.ErrExist) {
			continue
		}
		if e != nil {
			return e
		}
		_, e = file.Write(content)
		ce := file.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	}
	dir := filepath.Join(d.a.root, "bin", "runtime")
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "Mirroxy_downloader.exe")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = file.Write(engine)
	closeErr := file.Close()
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	return closeErr
}
func (d *DownloadManager) rpc(method string, out any, args ...any) error {
	params := append([]any{"token:" + d.secret}, args...)
	payload, _ := json.Marshal(M{"jsonrpc": "2.0", "id": "mirroxy", "method": "aria2." + method, "params": params})
	req, err := http.NewRequest("POST", d.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := d.client.Do(req)
	if err != nil {
		return errors.New("aria2 下载引擎未响应")
	}
	defer res.Body.Close()
	var result struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(&result); err != nil {
		return errors.New("aria2 响应异常")
	}
	// Never echo upstream messages: they can contain URLs, proxy credentials or paths.
	if result.Error != nil {
		return fmt.Errorf("aria2 操作失败（代码 %d）", result.Error.Code)
	}
	if out != nil {
		return json.Unmarshal(result.Result, out)
	}
	return nil
}
func (d *DownloadManager) alive() bool {
	if d.done == nil {
		return false
	}
	select {
	case <-d.done:
		return false
	default:
		return true
	}
}
func (d *DownloadManager) save() error { return d.a.writeState("downloads", d.journal) }

func (d *DownloadManager) validate(spec DownloadSpec) (DownloadSpec, error) {
	spec.URL = strings.TrimSpace(spec.URL)
	u, err := url.Parse(spec.URL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ftp") {
		return spec, errors.New("请填写有效的 HTTP、HTTPS 或 FTP 下载链接")
	}
	if len(spec.URL) > 8192 || strings.ContainsAny(spec.URL, "\r\n\x00") {
		return spec, errors.New("下载链接过长或包含无效字符")
	}
	spec.Name = strings.TrimSpace(spec.Name)
	if spec.Name != "" && (!filepath.IsLocal(spec.Name) || spec.Name == "." || spec.Name == ".." || strings.ContainsAny(spec.Name, "/\\:*?\"<>|\x00\r\n") || len(spec.Name) > 240 || strings.HasSuffix(spec.Name, ".") || strings.HasSuffix(spec.Name, " ")) {
		return spec, errors.New("文件名必须是不含路径的普通文件名")
	}
	if spec.Dir == "" {
		spec.Dir = d.journal.DefaultDir
	}
	if !filepath.IsAbs(spec.Dir) || strings.ContainsAny(spec.Dir, "\r\n\x00") {
		return spec, errors.New("保存目录必须是绝对路径")
	}
	spec.Dir = filepath.Clean(spec.Dir)
	// Do not let download names/paths overwrite application source or secrets.
	resolvedDir := resolveDownloadDirectory(spec.Dir)
	resolvedRoot := resolveDownloadDirectory(d.a.root)
	if rel, e := filepath.Rel(resolvedRoot, resolvedDir); e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		allowed := resolveDownloadDirectory(filepath.Join(d.a.root, "downloads"))
		rel, e = filepath.Rel(allowed, resolvedDir)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return spec, errors.New("不能保存到程序目录；请选择其他目录或程序目录下的 downloads")
		}
	}
	if spec.Connections < 1 || spec.Connections > 16 {
		return spec, errors.New("每个任务连接数必须为 1–16")
	}
	if spec.LimitKiB < 0 || spec.LimitKiB > 1048576 {
		return spec, errors.New("下载限速必须为 0–1048576 KiB/s；0 为不限速")
	}
	if spec.ProxyMode == "" {
		spec.ProxyMode = "direct"
	}
	switch spec.ProxyMode {
	case "direct":
		spec.NodeID = ""
		spec.ProxyURL = ""
	case "node":
		if spec.NodeID == "" {
			return spec, errors.New("请选择代理节点")
		}
		spec.ProxyURL = ""
	case "custom":
		p, e := url.Parse(strings.TrimSpace(spec.ProxyURL))
		if e != nil || p.Scheme != "http" || p.Hostname() == "" || p.RawQuery != "" || p.Fragment != "" || (p.Path != "" && p.Path != "/") || strings.ContainsAny(spec.ProxyURL, "\r\n\x00") {
			return spec, errors.New("aria2 自定义代理请填写 http://主机:端口，可包含用户名密码；不直接支持 SOCKS5")
		}
		spec.ProxyURL = p.String()
		spec.NodeID = ""
	default:
		return spec, errors.New("未知代理方式")
	}
	return spec, nil
}

func resolveDownloadDirectory(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	return filepath.Join(resolveDownloadDirectory(parent), filepath.Base(path))
}
func (d *DownloadManager) options(task *DownloadTask) (map[string]string, error) {
	proxy := ""
	if task.ProxyMode == "custom" {
		proxy = task.ProxyURL
	}
	if task.ProxyMode == "node" {
		d.a.mu.Lock()
		node := d.a.node(task.NodeID)
		if node == nil || !node.Enabled || !d.a.cores[node.coreKey()].alive() {
			d.a.mu.Unlock()
			return nil, errors.New("所选代理节点未运行；请先启动节点，不会自动切换为直连")
		}
		proxy = fmt.Sprintf("http://127.0.0.1:%d", node.Port)
		d.a.mu.Unlock()
	}
	opts := map[string]string{"dir": task.Dir, "split": strconv.Itoa(task.Connections), "max-connection-per-server": strconv.Itoa(task.Connections), "min-split-size": "1M", "max-download-limit": strconv.Itoa(task.LimitKiB) + "K", "all-proxy": proxy, "http-proxy": proxy, "https-proxy": proxy, "ftp-proxy": proxy, "no-proxy": "", "continue": "true", "allow-overwrite": "false", "auto-file-renaming": "true", "check-certificate": "true", "pause": "true"}
	opts["connect-timeout"] = "15"
	opts["timeout"] = "30"
	opts["max-tries"] = "5"
	opts["retry-wait"] = "3"
	opts["lowest-speed-limit"] = "1"
	opts["auto-save-interval"] = "5"
	opts["force-save"] = "true"
	if task.Name != "" {
		opts["out"] = task.Name
	}
	return opts, nil
}
func downloadEnv() []string {
	env := []string{}
	for _, item := range os.Environ() {
		key := strings.ToLower(strings.SplitN(item, "=", 2)[0])
		if key != "http_proxy" && key != "https_proxy" && key != "all_proxy" && key != "ftp_proxy" && key != "no_proxy" {
			env = append(env, item)
		}
	}
	return env
}
func (d *DownloadManager) start() error {
	if d.closing {
		return errors.New("后台正在退出")
	}
	if d.alive() {
		return nil
	}
	binary := d.binary()
	if binary == "" {
		return errors.New("未找到 aria2：Windows 请安装新版完整包；Linux/macOS 请先安装 aria2（aria2c）")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	d.secret = newID() + newID()
	d.endpoint = fmt.Sprintf("http://127.0.0.1:%d/jsonrpc", port)
	cmd := exec.Command(binary, "--no-conf=true", "--enable-rpc=true", "--rpc-listen-all=false", "--rpc-allow-origin-all=false", "--rpc-listen-port="+strconv.Itoa(port), "--rpc-secret="+d.secret, "--enable-dht=false", "--enable-dht6=false", "--enable-peer-exchange=false", "--bt-enable-lpd=false", "--max-concurrent-downloads=30", "--max-download-result=1000", "--keep-unfinished-download-result=true", "--allow-overwrite=false", "--auto-file-renaming=true", "--check-certificate=true", "--file-allocation=none", "--console-log-level=error", "--quiet=true", "--no-netrc=true")
	cmd.Env = downloadEnv()
	hideCommand(cmd)
	if err = cmd.Start(); err != nil {
		return errors.New("无法启动 aria2 下载引擎")
	}
	d.cmd = cmd
	d.done = make(chan struct{})
	done := d.done
	assignJob(d.a.job, cmd.Process.Pid)
	go func() { _ = cmd.Wait(); close(done) }()
	ready := false
	for i := 0; i < 30; i++ {
		if !d.alive() {
			break
		}
		if d.rpc("getVersion", nil) == nil {
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		_ = cmd.Process.Kill()
		return errors.New("aria2 下载引擎启动失败")
	}
	for _, task := range d.journal.Tasks {
		task.GID = ""
		task.Speed = "0"
		if task.Status == "complete" || task.Status == "removed" {
			continue
		}
		if err = d.enqueue(task); err != nil {
			task.Status = "error"
			task.Error = err.Error()
		}
	}
	return d.save()
}
func (d *DownloadManager) enqueue(task *DownloadTask) error {
	opts, err := d.options(task)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(task.Dir, 0755); err != nil {
		return errors.New("无法创建下载目录，请检查写入权限")
	}
	if task.Path != "" {
		opts["out"] = filepath.Base(task.Path)
	}
	var gid string
	if err = d.rpc("addUri", &gid, []string{task.URL}, opts); err != nil {
		return err
	}
	task.GID = gid
	task.Status = "paused"
	task.Error = ""
	if task.Wanted {
		if err = d.rpc("unpause", nil, gid); err != nil {
			return err
		}
		task.Status = "waiting"
	}
	return nil
}

type ariaFile struct {
	Path string `json:"path"`
	URIs []struct {
		URI string `json:"uri"`
	} `json:"uris"`
}
type ariaStatus struct {
	GID          string     `json:"gid"`
	Status       string     `json:"status"`
	Total        string     `json:"totalLength"`
	Completed    string     `json:"completedLength"`
	Speed        string     `json:"downloadSpeed"`
	Connections  string     `json:"connections"`
	ErrorCode    string     `json:"errorCode"`
	ErrorMessage string     `json:"errorMessage"`
	Files        []ariaFile `json:"files"`
}

func (d *DownloadManager) sync() error {
	if !d.alive() {
		return nil
	}
	all := []ariaStatus{}
	for _, request := range []struct {
		method string
		args   []any
	}{{"tellActive", nil}, {"tellWaiting", []any{0, 1000}}, {"tellStopped", []any{0, 1000}}} {
		var items []ariaStatus
		if err := d.rpc(request.method, &items, request.args...); err != nil {
			return err
		}
		all = append(all, items...)
	}
	indexed := map[string]ariaStatus{}
	for _, status := range all {
		indexed[status.GID] = status
	}
	for _, task := range d.journal.Tasks {
		status, ok := indexed[task.GID]
		if !ok {
			continue
		}
		task.Status = status.Status
		task.Total = status.Total
		task.Completed = status.Completed
		task.Speed = status.Speed
		task.ConnectionsUsed = status.Connections
		trackDownloadProgress(task, status, time.Now())
		if len(status.Files) > 0 && status.Files[0].Path != "" {
			task.Path = status.Files[0].Path
		}
		if status.Status == "error" {
			task.Error = downloadError(status.ErrorCode, status.ErrorMessage)
		} else {
			task.Error = ""
		}
		if status.Status == "complete" || status.Status == "error" {
			task.Wanted = false
		}
	}
	return nil
}

// Describe known failures without persisting raw upstream URLs/passwords/paths.
func downloadError(code, message string) string {
	message = strings.ToLower(message)
	detail := "下载失败"
	switch {
	case strings.Contains(message, "certificate"):
		detail = "TLS 证书校验失败，请检查下载站证书或代理链路；不会关闭证书校验"
	case strings.Contains(message, "handshake") || strings.Contains(message, "ssl/tls"):
		detail = "TLS 握手失败，请尝试其他节点或下载镜像；不会自动改为直连"
	case code == "2":
		detail = "连接超时，请检查节点和下载站"
	case code == "3" || code == "4":
		detail = "下载文件不存在或链接已失效"
	case code == "6":
		detail = "网络连接失败或被重置，请检查节点和下载站"
	case code == "8":
		detail = "下载站不支持断点续传"
	case code == "9":
		detail = "保存磁盘空间不足"
	case code == "19":
		detail = "域名解析失败"
	case code == "24":
		detail = "下载站认证失败"
	case code == "1":
		detail = "下载遇到未分类错误，请检查节点与下载站"
	}
	return detail + "（aria2 代码 " + code + "）"
}
func (d *DownloadManager) snapshot() (M, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.sync(); err != nil {
		return nil, err
	}
	tasks := []*DownloadTask{}
	for _, task := range d.journal.Tasks {
		copy := *task
		tasks = append(tasks, &copy)
	}
	sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].Created > tasks[j].Created })
	return M{"available": d.binary() != "", "running": d.alive(), "tasks": tasks, "defaultDir": d.journal.DefaultDir, "maxConnections": 16, "maxConcurrent": 30}, nil
}
func (d *DownloadManager) add(spec DownloadSpec, paused bool) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.journal.Tasks) >= 1000 {
		return "", errors.New("最多保留 1000 个下载任务，请删除旧记录")
	}
	validated, err := d.validate(spec)
	if err != nil {
		return "", err
	}
	task := &DownloadTask{ID: newID(), DownloadSpec: validated, Created: time.Now().Format(time.RFC3339Nano), Wanted: !paused, Status: "paused", Speed: "0"}
	if _, err = d.options(task); err != nil {
		return "", err
	}
	if err = d.start(); err != nil {
		return "", err
	}
	if err = d.enqueue(task); err != nil {
		return "", err
	}
	d.journal.Tasks = append(d.journal.Tasks, task)
	d.journal.DefaultDir = task.Dir
	if err = d.save(); err != nil {
		_ = d.rpc("forceRemove", nil, task.GID)
		d.journal.Tasks = d.journal.Tasks[:len(d.journal.Tasks)-1]
		return "", err
	}
	return task.ID, nil
}
func (d *DownloadManager) action(id, action string, spec DownloadSpec, deleteFiles ...bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.sync(); err != nil {
		return err
	}
	var task *DownloadTask
	index := -1
	for i, item := range d.journal.Tasks {
		if item.ID == id {
			task = item
			index = i
			break
		}
	}
	if task == nil {
		return errors.New("下载任务不存在")
	}
	if action == "open-file" || action == "open-folder" {
		target, err := d.downloadOpenTarget(task, action == "open-folder")
		if err != nil {
			return err
		}
		launch := d.openTarget
		if launch == nil {
			launch = platformOpenDownload
		}
		return launch(target, action == "open-folder")
	}
	if action == "reconnect" {
		if task.Status == "complete" {
			return errors.New("任务已经完成")
		}
		if _, err := d.options(task); err != nil {
			return err
		}
		if err := d.start(); err != nil {
			return err
		}
		if task.GID != "" {
			method := "removeDownloadResult"
			if task.Status == "active" || task.Status == "waiting" || task.Status == "paused" {
				if task.Status != "paused" {
					if err := d.rpc("forcePause", nil, task.GID); err != nil {
						return err
					}
				}
				method = "forceRemove"
			}
			if err := d.rpc(method, nil, task.GID); err != nil {
				return err
			}
		}
		task.GID = ""
		task.Wanted = true
		task.Stalled = false
		task.noProgressSince = time.Time{}
		if err := d.enqueue(task); err != nil {
			task.Wanted = false
			return err
		}
		return d.save()
	}
	if action == "remove" {
		previousStatus := task.Status
		if task.GID != "" && d.alive() {
			method := "forceRemove"
			if task.Status == "complete" || task.Status == "error" || task.Status == "removed" {
				method = "removeDownloadResult"
			}
			if err := d.rpc(method, nil, task.GID); err != nil {
				return err
			}
		}
		task.Wanted = false
		if len(deleteFiles) > 0 && deleteFiles[0] {
			if err := d.save(); err != nil {
				return err
			}
			task.Status = previousStatus
			if err := d.trashTaskFiles(task); err != nil {
				task.Status = "removed"
				task.Error = err.Error()
				_ = d.save()
				return err
			}
		}
		d.journal.Tasks = append(d.journal.Tasks[:index], d.journal.Tasks[index+1:]...)
		return d.save() // Deliberately retain downloaded files and .aria2 resume data.
	}
	if action != "pause" && action != "resume" && action != "retry" && action != "edit" {
		return errors.New("未知下载操作")
	}
	if action == "pause" {
		if task.Status == "complete" || task.Status == "error" {
			return nil
		}
		if d.alive() && task.GID != "" && task.Status != "paused" {
			if err := d.rpc("forcePause", nil, task.GID); err != nil {
				return err
			}
		}
		task.Wanted = false
		task.Status = "paused"
		task.Speed = "0"
		return d.save()
	}
	if action == "edit" {
		if task.Status != "paused" && task.Status != "error" {
			return errors.New("请先暂停任务再编辑")
		}
		validated, err := d.validate(spec)
		if err != nil {
			return err
		}
		if validated.Dir != task.Dir || validated.Name != task.Name {
			return errors.New("编辑时保留原保存目录与文件名，以避免破坏断点数据")
		}
		candidate := *task
		candidate.DownloadSpec = validated
		candidate.Wanted = false
		if _, err = d.options(&candidate); err != nil {
			return err
		}
		if d.alive() && task.GID != "" {
			method := "forceRemove"
			if task.Status == "error" {
				method = "removeDownloadResult"
			}
			if err = d.rpc(method, nil, task.GID); err != nil {
				return err
			}
		}
		candidate.GID = ""
		candidate.Status = "paused"
		candidate.Error = ""
		*task = candidate
		return d.save()
	}
	if task.Status == "complete" {
		return errors.New("任务已经完成")
	}
	if _, err := d.options(task); err != nil {
		return err
	}
	if err := d.start(); err != nil {
		return err
	}
	if task.GID != "" && (task.Status == "error" || task.Status == "removed") {
		if err := d.rpc("removeDownloadResult", nil, task.GID); err != nil {
			return err
		}
		task.GID = ""
	}
	task.Wanted = true
	if task.GID == "" {
		if err := d.enqueue(task); err != nil {
			task.Wanted = false
			return err
		}
	} else if task.Status == "paused" {
		if err := d.rpc("unpause", nil, task.GID); err != nil {
			task.Wanted = false
			return err
		}
		task.Status = "waiting"
	}
	return d.save()
}
func (a *App) restoreDownloads() {
	d, err := a.downloads()
	if err != nil {
		return
	}
	d.mu.Lock()
	if len(d.journal.Tasks) > 0 { // Completed history does not need a child process.
		for _, task := range d.journal.Tasks {
			if task.Wanted {
				_ = d.start()
				break
			}
		}
	}
	d.mu.Unlock()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.shutdown:
			return
		case <-ticker.C:
			d.mu.Lock()
			if d.alive() {
				if d.sync() == nil {
					_ = d.save()
				}
			}
			d.mu.Unlock()
		}
	}
}
func (a *App) stopDownloads() {
	a.downloadInitMu.Lock()
	d := a.downloadManager
	a.downloadInitMu.Unlock()
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closing = true
	_ = d.sync()
	_ = d.save()
	if d.alive() {
		_ = d.rpc("forceShutdown", nil)
		select {
		case <-d.done:
		case <-time.After(4 * time.Second):
			_ = d.cmd.Process.Kill()
			<-d.done
		}
	}
}
func (a *App) downloadAPI(w http.ResponseWriter, r *http.Request) {
	if !localAccess(r) {
		http.Error(w, "下载管理目前仅允许本机访问", 403)
		return
	}
	d, err := a.downloads()
	if err != nil {
		respond(w, nil, err)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/downloads" {
		snapshot, e := d.snapshot()
		respond(w, snapshot, e)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Not found", 404)
		return
	}
	if r.URL.Path == "/api/downloads/install" {
		err = d.install()
		respond(w, M{"ok": err == nil}, err)
		return
	}
	var body struct {
		ID          string `json:"id"`
		Action      string `json:"action"`
		Paused      bool   `json:"paused"`
		DeleteFiles bool   `json:"deleteFiles"`
		DownloadSpec
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&body); err != nil {
		respond(w, nil, errors.New("下载参数格式错误"))
		return
	}
	if r.URL.Path == "/api/downloads/add" {
		id, e := d.add(body.DownloadSpec, body.Paused)
		respond(w, M{"id": id}, e)
		return
	}
	if r.URL.Path == "/api/downloads/action" {
		err = d.action(body.ID, body.Action, body.DownloadSpec, body.DeleteFiles)
		respond(w, M{"ok": err == nil}, err)
		return
	}
	http.Error(w, "Not found", 404)
}
