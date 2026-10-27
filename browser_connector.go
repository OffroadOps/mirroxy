package main

import (
	"archive/zip"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// The unpacked browser extension is embedded so the panel can hand out the
// exact build it shipped with. Users install it once as an unpacked extension.
//
//go:embed browser-extension
var browserExtensionFS embed.FS

func (a *App) browserConnectorInfo(w http.ResponseWriter, _ *http.Request) {
	a.mu.Lock()
	token := a.cfg.BrowserDownloadToken
	seen := a.browserSeen
	a.mu.Unlock()
	respond(w, M{
		"version":      version,
		"scheme":       a.panelScheme(),
		"port":         a.panelPort,
		"url":          a.panelURL(),
		"token":        token,
		"connected":    !seen.IsZero() && time.Since(seen) < 5*time.Minute,
		"extensionZip": "/api/downloads/connector/extension.zip",
	}, nil)
}

func (a *App) browserConnectorAPI(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.browserSeen = time.Now()
	a.mu.Unlock()
	switch r.URL.Path {
	case "/api/downloads/connector/ping":
		respond(w, M{"ok": true, "version": version}, nil)
		return
	case "/api/downloads/connector/add":
		d, err := a.downloads()
		if err != nil {
			respond(w, nil, err)
			return
		}
		var body struct {
			URL  string `json:"url"`
			Name string `json:"name"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&body); err != nil {
			respond(w, nil, errors.New("浏览器接力参数格式错误"))
			return
		}
		name := strings.TrimSpace(body.Name)
		if name != "" {
			name = filepath.Base(name)
			if name == "." || name == ".." {
				name = ""
			}
		}
		id, err := d.add(DownloadSpec{URL: body.URL, Name: name, Connections: 8, ProxyMode: "direct"}, false)
		if err != nil {
			respond(w, nil, err)
			return
		}
		a.event("浏览器下载已转入 Mirroxy：" + first(name, body.URL))
		respond(w, M{"ok": true, "id": id}, nil)
		return
	}
	http.NotFound(w, r)
}

func (a *App) browserExtensionZip(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="Mirroxy-browser-extension.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	archive := zip.NewWriter(w)
	defer archive.Close()
	_ = fs.WalkDir(browserExtensionFS, "browser-extension", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative := strings.TrimPrefix(path, "browser-extension/")
		if relative == path || relative == "" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relative
		header.Method = zip.Deflate
		header.Modified = time.Time{}
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		data, err := browserExtensionFS.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	})
}
