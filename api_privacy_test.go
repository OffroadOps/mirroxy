package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicStateUsesGenericCoreIdentifiersAndReadsGenericLog(t *testing.T) {
	root := t.TempDir()
	a := &App{root: root, panelPort: 15060, cores: map[string]*Core{}, traffic: map[string]*Traffic{}, cfg: Settings{
		Nodes: []Node{{ID: "a", Engine: "xray", AppPort: 25060}, {ID: "b", Engine: "sing-box"}}, Updated: map[string]string{},
		AppRouting: AppRoutingSettings{Secret: "must-not-leak", Profiles: []AppRouteProfile{{ID: "p", Name: "Game", Executables: []string{"game.exe"}}}},
	}}
	b, err := json.Marshal(a.state())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"engine":"xray"`) || strings.Contains(string(b), `"engine":"sing-box"`) || !strings.Contains(string(b), `"engine":"A"`) || !strings.Contains(string(b), `"engine":"B"`) {
		t.Fatalf("public state exposes internal core identifiers: %s", b)
	}
	routing, _ := json.Marshal(a.appRoutingState())
	if strings.Contains(string(b), "25060") || strings.Contains(string(b), "must-not-leak") || strings.Contains(string(routing), "must-not-leak") {
		t.Fatalf("public state exposes application routing credentials or internal ports: state=%s routing=%s", b, routing)
	}
	logPath := a.dataPath(filepath.Join("runtime", "Mirroxy_worker_"+runtimeFileID("xray")+".log"))
	if err = os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(logPath, []byte("generic log proof"), 0600); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/log", strings.NewReader(`{"engine":"A"}`))
	rec := httptest.NewRecorder()
	a.api(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "generic log proof") {
		t.Fatalf("generic log not returned: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRemoteAPIHidesAndCannotControlApplicationRouting(t *testing.T) {
	a := &App{
		root: t.TempDir(), panelPort: 15060, token: "remote-token", cores: map[string]*Core{}, traffic: map[string]*Traffic{},
		cfg: Settings{
			Nodes: []Node{{ID: "a", Name: "Node", Engine: "xray", Port: 5060, AppPort: 25060}}, Updated: map[string]string{},
			AppRouting: AppRoutingSettings{Secret: "must-not-leak", DefaultNode: "a", Profiles: []AppRouteProfile{{ID: "p", Name: "Private game", Executables: []string{`C:\\Games\\private.exe`}, Enabled: true}}},
		},
		appRouter: &appRouterRuntime{ControlPort: 32123, Secret: "control-secret"}, appRouterPhase: "running",
	}
	remote := func(method, path, body string) *http.Request {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		return req.WithContext(context.WithValue(req.Context(), localAccessKey{}, false))
	}

	state := httptest.NewRecorder()
	a.api(state, remote(http.MethodGet, "/api/state", ""))
	if state.Code != http.StatusOK || strings.Contains(state.Body.String(), "appRouting") || strings.Contains(state.Body.String(), "Private game") || strings.Contains(state.Body.String(), "private.exe") {
		t.Fatalf("remote state exposed local application routing: %d %s", state.Code, state.Body.String())
	}

	processes := httptest.NewRecorder()
	a.api(processes, remote(http.MethodGet, "/api/processes", ""))
	if !strings.Contains(processes.Body.String(), "只能在本机读取运行中的程序") {
		t.Fatalf("remote process listing was not blocked: %d %s", processes.Code, processes.Body.String())
	}

	stopAll := httptest.NewRecorder()
	a.api(stopAll, remote(http.MethodPost, "/api/stop-all", `{}`))
	if !strings.Contains(stopAll.Body.String(), "请在本机面板停止") || a.appRouter == nil || a.appRouterPhase != "running" {
		t.Fatalf("remote stop-all controlled local application routing: %d %s", stopAll.Code, stopAll.Body.String())
	}
}
