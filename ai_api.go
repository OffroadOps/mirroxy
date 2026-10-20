package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// List/status responses omit credentials. Only explicit share export returns them.
func (a *App) apiV1(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	if r.Method == "GET" {
		switch path {
		case "openapi.json":
			respond(w, openAPI(), nil)
			return
		case "guide":
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.Write([]byte(aiGuide))
			return
		case "status", "nodes", "traffic", "subscriptions":
			a.mu.Lock()
			defer a.mu.Unlock()
			s := a.state()
			switch path {
			case "status":
				respond(w, M{"version": version, "appUpdate": a.appUpdate, "platform": platformState(), "remote": s["remote"], "nodes": s["nodes"], "cores": s["cores"], "traffic": s["traffic"], "systemProxy": s["systemProxy"], "systemProxyEnabled": s["systemProxyEnabled"], "systemProxyMode": a.cfg.SystemProxyMode, "routingMode": a.cfg.RoutingMode}, nil)
			case "traffic":
				respond(w, s["traffic"], nil)
			default:
				respond(w, M{path: s[path]}, nil)
			}
			return
		}
		if strings.HasPrefix(path, "nodes/") {
			id := strings.TrimPrefix(path, "nodes/")
			a.mu.Lock()
			defer a.mu.Unlock()
			for _, v := range a.state()["nodes"].([]any) {
				if v.(M)["id"] == id {
					respond(w, v, nil)
					return
				}
			}
		}
		http.NotFound(w, r)
		return
	}
	if r.Method != "POST" && r.Method != "DELETE" {
		w.Header().Set("Allow", "GET, POST, DELETE")
		http.Error(w, "Method not allowed", 405)
		return
	}
	body := M{}
	if r.ContentLength != 0 {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "Use Content-Type: application/json", 415)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
		defer r.Body.Close()
		dec := json.NewDecoder(r.Body)
		if e := dec.Decode(&body); e != nil || body == nil {
			http.Error(w, "Invalid JSON object", 400)
			return
		}
		var extra any
		if e := dec.Decode(&extra); e != io.EOF {
			http.Error(w, "One JSON object required", 400)
			return
		}
	}
	target := ""
	mapped := M{}
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && parts[0] == "nodes" && parts[1] != "import" {
		mapped["id"] = parts[1]
		if len(parts) == 2 && r.Method == "DELETE" {
			target = "node"
			mapped["action"] = "delete"
		}
		if len(parts) == 3 && r.Method == "POST" {
			switch parts[2] {
			case "start", "stop", "reassign", "move-bottom", "auto-order":
				target = "node"
				mapped["action"] = parts[2]
			case "route":
				target = "node"
				mapped["action"] = "route"
				mapped["route"] = body["route"]
			case "test":
				target = "test"
			case "share":
				target = "node/share"
			case "rename":
				target = "node/rename"
				mapped["name"] = body["name"]
			case "system-proxy":
				target = "proxy"
				mapped["action"] = "set"
			}
		}
	} else if r.Method == "POST" {
		switch path {
		case "nodes/import":
			target = "import"
			mapped["text"] = body["text"]
			if deploy, ok := body["deploy"]; ok {
				mapped["deploy"] = deploy
			}
		case "stop-all":
			target = "stop-all"
		case "system-proxy/restore":
			target = "proxy"
			mapped["action"] = "restore"
		case "updates/check":
			target = "app-update/check"
		case "updates/apply":
			target = "app-update/apply"
		case "routing":
			target = "routing"
			mapped["mode"] = body["mode"]
		case "system-proxy":
			target = "proxy"
			mapped["action"] = "mode"
			mapped["mode"] = body["mode"]
			mapped["id"] = body["nodeId"]
		case "subscriptions":
			target = "subscription/add"
			mapped["name"] = body["name"]
			mapped["url"] = body["url"]
		default:
			if len(parts) == 3 && parts[0] == "subscriptions" && parts[2] == "refresh" {
				target = "subscription/refresh"
				mapped["id"] = parts[1]
			}
		}
	}
	if target == "" {
		http.NotFound(w, r)
		return
	}
	b, e := json.Marshal(mapped)
	if e != nil {
		respond(w, nil, e)
		return
	}
	clone := r.Clone(r.Context())
	u := *r.URL
	u.Path = "/api/" + target
	clone.URL = &u
	clone.Method = "POST"
	clone.Body = io.NopCloser(bytes.NewReader(b))
	clone.ContentLength = int64(len(b))
	a.api(w, clone)
}

const aiGuide = `# Mirroxy AI API v1

本机地址：http://127.0.0.1:15060/api/v1/ 。本机原生 HTTP 客户端无需登录或 token。
OpenAPI：GET openapi.json 。状态：GET status 。节点：GET nodes 。每秒统计：GET traffic 。
远程使用独立管理端口（默认 15061），每次请求通过 HTTP Basic 提交账号密码。
Linux 安装器默认 HTTP，Windows 默认 HTTPS；以 status.remote.scheme 为准。
Linux 安装器会开启远程管理并生成账号密码。Windows 远程默认关闭，在本机“AI API / 远程访问”设置。
远程监听器即使从 127.0.0.1 或反向代理连接也要求认证。带转发头的本机请求同样要求认证。
所有修改接口使用 application/json；空参数可以传 {}。列表、状态不返回节点密码和订阅 URL。
POST nodes/{id}/share 返回 {name, protocol, url}，url 是含认证信息的完整节点分享链接。
仅在用户明确要导出/分享节点时调用；不要把链接写入日志或普通状态汇报。

流程：先 GET nodes 获取 id → POST nodes/{id}/start → 从返回后的 GET nodes 读取固定 port。
连接代理使用 HTTP http://127.0.0.1:port 或 SOCKS5 socks5h://127.0.0.1:port。
POST nodes/{id}/stop 停止；POST nodes/{id}/test 联网检测（停止的节点也可检测；忙时返回 error）。
节点 health.status 为 unknown / online / offline，与节点进程 status、流量采样 available 分开。
GET traffic 返回 health 汇总：nodes、online、offline、unknown、checking、enabled、intervalSeconds。
每个节点 health 含 checking、checkedAt、latencyMs、error。默认每 60 秒检测所有节点，主站失败再试备用站。
默认绿色的 unknown 仅表示尚未检测；online 表示最近一次通过检测，offline 表示最近一次未通过。
POST routing，JSON {"mode":"bypass-cn|blacklist|proxy"}，默认绕过大陆，不改变系统代理。
POST nodes/{id}/route，JSON {"route":"inherit|proxy|bypass-cn|blacklist"}。inherit 跟随全局，其余为单节点覆盖。effectiveRoute 返回实际生效模式。
绕过大陆：局域网、大陆域名/IP 直连，其他走该节点；黑名单：命中 GFW/GreatFire/Google 与常用海外 IP 名单走节点，其他直连；全局：全部走该节点。
POST nodes/{id}/rename，JSON {"name":"我的下载线路"}。名称需为 1–128 个字符，不含换行。
自定义名称会保留到后续订阅更新；改名不重启代理，端口及累计流量保持不变。
POST nodes/import，JSON {"text":"分享链接，每行一个"}。
Linux 默认自动部署，Windows 默认只导入；可传 deploy:true/false 覆盖。返回 count、deployed、nodeIds、warnings。
缺少核心时可自动下载适合当前系统/架构的官方核心并校验，优先使用预置核心。
DELETE nodes/{id} 删除；POST stop-all 全部停止。
GET subscriptions / POST subscriptions（name, url）/ POST subscriptions/{id}/refresh。

POST updates/check 检查程序更新；POST updates/apply 开始在线更新，立即返回。GET status.appUpdate 可跟踪 phase/message。
只有用户要求升级时调用 apply；后台将短暂重启，已启用节点自动恢复。节点、端口、名称与累计流量保留。

默认不改变系统代理，不影响 v2rayN。只有明确需要时才调用系统代理接口。
POST system-proxy：{"mode":"unchanged|clear|auto","nodeId":"节点 id"}。
auto 要求运行中的 nodeId。clear 会清除当前 Windows 静态代理。
POST system-proxy/restore 恢复接管前的设置（仅当仍由本程序管理）。
远程机器不能直接使用这些 127.0.0.1 代理地址；它们属于运行 Mirroxy 的主机。

统计字段：connections 是当前已建立的入站 TCP 连接数（SOCKS UDP 控制连接也算 TCP），不含 UDP 流数量。
uploadBytesPerSecond / downloadBytesPerSecond 单位 B/s；uploadedBytes / downloadedBytes 单位 bytes。
流量按本地节点入站统计，包含该节点绕过规则走直连的流量，不含外层隧道开销，不代表链路速率上限。
统计每秒采样，累计值跨网页刷新和正常重启保存；每 10 秒落盘，异常掉电/核心崩溃可能丢失末次采样。
available=false 表示尚无有效流量采样，不能把当前速度解释为可靠的零；connectionCountAvailable 单独表示连接数可用性。
首次启用统计前的历史流量无法补回；since 为本节点统计起点。
核心 A 的节点配置变化会短暂重启共享核心；核心 B 的节点各自独立运行和统计。
`

func openAPI() M {
	paths := M{}
	response := M{"200": M{"description": "JSON result; nodes contain no credentials"}, "400": M{"description": "Invalid request or operation failed; JSON error"}, "401": M{"description": "Remote credentials required"}, "403": M{"description": "Cross-site request rejected"}, "404": M{"description": "Not found"}}
	add := func(path, method, summary string, fields M, required []string) {
		op := M{"summary": summary, "responses": response, "security": []any{M{}, M{"basicAuth": []string{}}}}
		if strings.Contains(path, "{id}") {
			op["parameters"] = []any{M{"name": "id", "in": "path", "required": true, "schema": M{"type": "string"}}}
		}
		if method == "post" {
			op["requestBody"] = M{"required": len(required) > 0, "content": M{"application/json": M{"schema": M{"type": "object", "properties": fields}}}}
		}
		if method == "post" && len(required) > 0 {
			op["requestBody"].(M)["content"].(M)["application/json"].(M)["schema"].(M)["required"] = required
		}
		if paths[path] == nil {
			paths[path] = M{}
		}
		paths[path].(M)[method] = op
	}
	str := M{"type": "string"}
	for _, v := range []string{"status", "nodes", "traffic", "subscriptions", "nodes/{id}"} {
		add("/"+v, "get", "Read "+v, nil, nil)
	}
	for _, v := range []string{"start", "stop", "reassign", "test", "system-proxy"} {
		add("/nodes/{id}/"+v, "post", v+" node (system-proxy changes Windows settings)", M{}, nil)
	}
	add("/nodes/{id}/route", "post", "Change route", M{"route": M{"type": "string", "enum": []string{"inherit", "proxy", "bypass-cn", "blacklist"}}}, []string{"route"})
	add("/updates/check", "post", "Check for an application update", M{}, nil)
	add("/updates/apply", "post", "Download, verify and restart application; preserves user data", M{}, nil)
	add("/routing", "post", "Set global routing; explicit node overrides are preserved", M{"mode": M{"type": "string", "enum": []string{"bypass-cn", "blacklist", "proxy"}}}, []string{"mode"})
	add("/nodes/{id}/rename", "post", "Rename node without restarting; preserve custom name on subscription refresh", M{"name": M{"type": "string", "minLength": 1, "maxLength": 128}}, []string{"name"})
	add("/nodes/{id}/share", "post", "Export node share URL including credentials; only on explicit request", M{}, nil)
	paths["/nodes/{id}/share"].(M)["post"].(M)["responses"] = M{"200": M{"description": "Node share link containing credentials", "content": M{"application/json": M{"schema": M{"type": "object", "properties": M{"name": str, "protocol": str, "url": str}}}}}, "400": M{"description": "Node missing or connection options cannot be preserved"}, "401": M{"description": "Remote credentials required"}, "403": M{"description": "Cross-site request rejected"}}
	add("/nodes/{id}", "delete", "Delete node", nil, nil)
	add("/nodes/import", "post", "Import share links; deploy uses the global default unless overridden", M{"text": str, "deploy": M{"type": "boolean"}}, []string{"text"})
	add("/stop-all", "post", "Stop all nodes", M{}, nil)
	add("/system-proxy", "post", "Set Windows proxy mode; default unchanged", M{"mode": M{"type": "string", "enum": []string{"unchanged", "clear", "auto"}}, "nodeId": str}, []string{"mode"})
	add("/system-proxy/restore", "post", "Restore owned Windows proxy", M{}, nil)
	add("/subscriptions", "post", "Add a subscription", M{"name": str, "url": str}, []string{"url"})
	add("/subscriptions/{id}/refresh", "post", "Refresh subscription", M{}, nil)
	add("/openapi.json", "get", "OpenAPI schema", nil, nil)
	add("/guide", "get", "Usage guide (text/markdown)", nil, nil)
	return M{"openapi": "3.0.3", "info": M{"title": "Mirroxy AI API", "version": version, "description": aiGuide}, "servers": []any{M{"url": "/api/v1"}}, "paths": paths, "components": M{"securitySchemes": M{"basicAuth": M{"type": "http", "scheme": "basic", "description": "Required on the remote HTTP/HTTPS listener; loopback local listener needs no credentials."}}}}
}
