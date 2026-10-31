package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Only download endpoints are exposed. Never accepts arbitrary URLs.
type backend struct {
	base   string
	client *http.Client
}

func newBackend(port int) *backend {
	return &backend{base: fmt.Sprintf("http://127.0.0.1:%d", port), client: &http.Client{Timeout: 100 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("后台重定向被拒绝") }}}
}
func (b *backend) call(path, body, token string) (string, error) {
	method := "GET"
	if body != "" {
		method = "POST"
	}
	req, err := http.NewRequest(method, b.base+"/api/"+path, bytes.NewBufferString(body))
	if err != nil {
		return "", err
	}
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-App-Token", token)
	}
	res, err := b.client.Do(req)
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
func (b *backend) request(path, body string) (string, error) {
	read := path == "state" || path == "downloads"
	write := path == "downloads/add" || path == "downloads/action" || path == "downloads/settings" || path == "downloads/install"
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
	return b.call(path, body, token)
}
