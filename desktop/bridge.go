package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// Only local backend endpoints are exposed to the embedded WebView2 shell.
type backend struct {
	mu     sync.Mutex
	port   int
	scheme string
	base   string
	client *http.Client
}

func newBackend(port int) *backend {
	return &backend{
		port: port,
		client: &http.Client{
			Timeout: 100 * time.Second,
			Transport: &http.Transport{
				Proxy:           nil,
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "localhost"},
			},
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return errors.New("后台重定向被拒绝")
			},
		},
	}
}

func (b *backend) currentPort() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.port
}

func (b *backend) setPort(port int) {
	if port < 1 || port > 65535 {
		return
	}
	b.mu.Lock()
	b.port = port
	b.scheme = ""
	b.mu.Unlock()
}

func (b *backend) baseURL() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.base != "" {
		return strings.TrimRight(b.base, "/")
	}
	scheme := b.scheme
	if scheme == "" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://127.0.0.1:%d", scheme, b.port)
}

func (b *backend) candidates() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.base != "" {
		return []string{strings.TrimRight(b.base, "/")}
	}
	first := b.scheme
	if first == "" {
		first = "https"
	}
	second := "http"
	if first == "http" {
		second = "https"
	}
	return []string{
		fmt.Sprintf("%s://127.0.0.1:%d", first, b.port),
		fmt.Sprintf("%s://127.0.0.1:%d", second, b.port),
	}
}

func (b *backend) rememberScheme(base string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.base != "" {
		return
	}
	if strings.HasPrefix(base, "https://") {
		b.scheme = "https"
	} else if strings.HasPrefix(base, "http://") {
		b.scheme = "http"
	}
}

func (b *backend) do(build func(base string) (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	for _, base := range b.candidates() {
		req, err := build(base)
		if err != nil {
			return nil, err
		}
		res, err := b.client.Do(req)
		if err == nil {
			b.rememberScheme(base)
			return res, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("无法连接 Mirroxy 后台，请先启动主程序")
	}
	return nil, lastErr
}

func (b *backend) call(apiPath, body, token string) (string, error) {
	method := "GET"
	if body != "" {
		method = "POST"
	}
	res, err := b.do(func(base string) (*http.Request, error) {
		req, e := http.NewRequest(method, base+"/api/"+apiPath, bytes.NewBufferString(body))
		if e != nil {
			return nil, e
		}
		if method == "POST" {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-App-Token", token)
		}
		return req, nil
	})
	if err != nil {
		return "", errors.New("无法连接 Mirroxy 后台，请先启动主程序")
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return "", err
	}
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return "", errors.New("后台响应格式错误")
	}
	if payload.Error != "" {
		return "", errors.New(payload.Error)
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("后台返回 HTTP %d", res.StatusCode)
	}
	return string(data), nil
}

func (b *backend) request(apiPath, body string) (string, error) {
	read := apiPath == "state" || apiPath == "downloads"
	write := apiPath == "downloads/add" || apiPath == "downloads/action" || apiPath == "downloads/settings" || apiPath == "downloads/install"
	if (!read && !write) || (read && body != "") || (write && body == "") || len(body) > 16<<10 {
		return "", errors.New("不允许的桌面请求")
	}
	token := ""
	if write {
		state, err := b.call("state", "", "")
		if err != nil {
			return "", err
		}
		var auth struct {
			Token string `json:"token"`
		}
		if json.Unmarshal([]byte(state), &auth) != nil || auth.Token == "" {
			return "", errors.New("无法获取本机授权")
		}
		token = auth.Token
	}
	return b.call(apiPath, body, token)
}

// apiHandler proxies /api/* requests from the embedded WebView2 panel to the
// local Mirroxy daemon, stripping browser origin/forwarding headers so the
// daemon treats the native window as direct loopback access.
func (b *backend) apiHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleaned := path.Clean(r.URL.Path)
		if !strings.HasPrefix(cleaned, "/api/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var bodyBytes []byte
		if r.Body != nil {
			var err error
			bodyBytes, err = io.ReadAll(io.LimitReader(r.Body, (8<<20)+1))
			if err != nil {
				http.Error(w, "请求读取失败", http.StatusBadRequest)
				return
			}
			if len(bodyBytes) > 8<<20 {
				http.Error(w, "请求体积过大", http.StatusRequestEntityTooLarge)
				return
			}
		}
		targetPath := r.URL.Path
		if r.URL.RawQuery != "" {
			targetPath += "?" + r.URL.RawQuery
		}
		res, err := b.do(func(base string) (*http.Request, error) {
			var reader io.Reader
			if len(bodyBytes) > 0 {
				reader = bytes.NewReader(bodyBytes)
			}
			req, e := http.NewRequestWithContext(r.Context(), r.Method, base+targetPath, reader)
			if e != nil {
				return nil, e
			}
			for _, header := range []string{"Content-Type", "X-App-Token", "Accept"} {
				if value := r.Header.Get(header); value != "" {
					req.Header.Set(header, value)
				}
			}
			return req, nil
		})
		if err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "无法连接 Mirroxy 后台，请先启动主程序"})
			return
		}
		defer res.Body.Close()
		for _, header := range []string{"Content-Type", "Content-Disposition", "Cache-Control"} {
			if value := res.Header.Get(header); value != "" {
				w.Header().Set(header, value)
			}
		}
		w.WriteHeader(res.StatusCode)
		_, _ = io.Copy(w, io.LimitReader(res.Body, 32<<20))
	})
}
