package main

import (
	"archive/zip"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
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
	panelPort := a.panelPort
	a.mu.Unlock()
	connector := a.panelURL()
	if panelPort != legacyPanelPort {
		connector = fmt.Sprintf("http://127.0.0.1:%d", legacyPanelPort)
	}
	respond(w, M{
		"version":      version,
		"scheme":       a.panelScheme(),
		"port":         panelPort,
		"url":          a.panelURL(),
		"connector":    connector,
		"token":        token,
		"connected":    !seen.IsZero() && time.Since(seen) < 5*time.Minute,
		"extensionZip": "/api/downloads/connector/extension.zip",
	}, nil)
}

// legacyBrowserConnector exposes automatic pairing on the plain-HTTP legacy
// port. It exists so the extension needs no copied token and no trusted local
// certificate on first install. Only extension origins may POST; the token is
// still required for downloads.
func (a *App) legacyBrowserConnector(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/downloads/connector/") {
		return false
	}
	origin := r.Header.Get("Origin")
	extension := isExtensionOrigin(origin)
	if extension {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Mirroxy-Download-Token")
		w.Header().Set("Access-Control-Max-Age", "600")
		if strings.EqualFold(r.Header.Get("Access-Control-Request-Private-Network"), "true") {
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == "OPTIONS" {
		if !extension {
			http.Error(w, "跨站请求被拒绝", 403)
			return true
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	if forwardedRequest(r) || !loopbackRequest(r) {
		http.Error(w, "浏览器接力仅允许本机访问", 403)
		return true
	}
	if !extension {
		http.Error(w, "跨站请求被拒绝", 403)
		return true
	}
	if r.URL.Path == "/api/downloads/connector/auto" && r.Method == "GET" {
		a.mu.Lock()
		token := a.cfg.BrowserDownloadToken
		panelPort := a.panelPort
		a.mu.Unlock()
		respond(w, M{
			"version":   version,
			"url":       a.panelURL(),
			"connector": fmt.Sprintf("http://127.0.0.1:%d", legacyPanelPort),
			"token":     token,
			"port":      panelPort,
		}, nil)
		return true
	}
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST, OPTIONS")
		http.Error(w, "Method not allowed", 405)
		return true
	}
	ctx := context.WithValue(r.Context(), localAccessKey{}, true)
	r = r.WithContext(ctx)
	if !a.browserConnectorAuthorized(r) {
		respond(w, nil, errors.New("浏览器接力令牌无效"))
		return true
	}
	a.browserConnectorAPI(w, r)
	return true
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
			URL     string `json:"url"`
			Name    string `json:"name"`
			Headers struct {
				Referer   string `json:"referer"`
				UserAgent string `json:"userAgent"`
				Cookie    string `json:"cookie"`
			} `json:"headers"`
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
		id, err := d.add(DownloadSpec{
			URL:         body.URL,
			Name:        name,
			Connections: 8,
			ProxyMode:   "direct",
			Referer:     body.Headers.Referer,
			UserAgent:   body.Headers.UserAgent,
			Cookie:      body.Headers.Cookie,
		}, false)
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
