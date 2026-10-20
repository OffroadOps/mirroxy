package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type M = map[string]any
type Node struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	OriginalName string `json:"originalName,omitempty"` // Provider name retained when the user sets a custom name.
	ListBottom   bool   `json:"listBottom,omitempty"`   // Explicit list placement overrides automatic started-first sorting.
	Protocol     string `json:"protocol"`
	Engine       string `json:"engine"`
	Server       string `json:"server"`
	Port         int    `json:"port"`
	AppPort      int    `json:"appPort,omitempty"` // Authenticated loopback-only entry for application routing; traffic follows the node's exit policy.
	Enabled      bool   `json:"enabled"`
	Route        string `json:"route"`
	Source       string `json:"source"`
	Outbound     M      `json:"outbound"`
}
type AppRouteProfile struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Executables []string `json:"executables"`
	NodeID      string   `json:"nodeId,omitempty"` // Empty follows AppRouting.DefaultNode.
	Enabled     bool     `json:"enabled"`
}
type AppRoutingSettings struct {
	Enabled     bool              `json:"enabled"`
	DefaultNode string            `json:"defaultNode,omitempty"`
	StrictRoute bool              `json:"strictRoute"`
	CompatMode  bool              `json:"compatMode,omitempty"` // Relaxed TUN hardening for coexistence with game accelerators.
	Secret      string            `json:"secret,omitempty"`
	Profiles    []AppRouteProfile `json:"profiles"`
}
type Subscription struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Updated string `json:"updated"`
	Error   string `json:"error"`
}
type Settings struct {
	Nodes                         []Node             `json:"nodes"`
	Subscriptions                 []Subscription     `json:"subscriptions"`
	StartPort                     int                `json:"startPort"`
	AutoUpdate                    bool               `json:"autoUpdate"`
	AutoSubscribe                 bool               `json:"autoSubscribe"`
	Autostart                     bool               `json:"autostart"`
	Updated                       map[string]string  `json:"updated"`
	OpenBrowser                   bool               `json:"openBrowser"`
	ResumeNodes                   bool               `json:"resumeNodes"`
	DefaultRoute                  string             `json:"defaultRoute"`
	RoutingMode                   string             `json:"routingMode"`
	LogLevel                      string             `json:"logLevel"`
	SubscribeHours                int                `json:"subscribeHours"`
	HealthMonitor                 bool               `json:"healthMonitor"`
	HealthInterval                int                `json:"healthInterval"`
	AutoDownload                  bool               `json:"autoDownload"`
	AutoDeploy                    bool               `json:"autoDeploy"`
	Remote                        RemoteSettings     `json:"remote"`
	SystemProxyMode               string             `json:"systemProxyMode"`
	SystemProxyNode               string             `json:"systemProxyNode"`
	PauseSystemProxyOnAccelerator *bool              `json:"pauseSystemProxyOnAccelerator,omitempty"` // nil means enabled.
	PausedSystemProxy             *PausedSystemProxy `json:"pausedSystemProxy,omitempty"`
	BypassLAN                     *bool              `json:"bypassLAN,omitempty"` // nil means enabled: LAN and loopback stay direct.
	GeoAutoUpdate                 bool               `json:"geoAutoUpdate"`
	PanelPort                     int                `json:"panelPort,omitempty"` // Local dashboard port; 0 keeps the 9527 default.
	AppRouting                    AppRoutingSettings `json:"appRouting"`
}
type App struct {
	mu               sync.Mutex
	cfg              Settings
	root             string
	panelPort        int
	token            string
	cores            map[string]*Core
	events           []string
	job              uintptr
	shutdown         chan struct{}
	busyUpdate       bool
	metricsPorts     map[string]int
	metricsSecret    string
	traffic          map[string]*Traffic
	trafficSaved     time.Time
	remoteServer     *http.Server
	remoteError      string
	authMu           sync.Mutex
	authCache        map[string]time.Time
	authFailures     map[string]*loginFailures
	quitOnce         sync.Once
	trayWindow       uintptr
	trayDone         chan struct{}
	health           map[string]*healthRecord
	healthGate       sync.RWMutex
	healthWorkers    sync.WaitGroup
	healthDone       chan struct{}
	appUpdate        AppUpdateState
	pendingAppUpdate string
	store            *stateStore
	storeMu          sync.Mutex
	appRouterMu      sync.Mutex
	appRouter        *appRouterRuntime
	appRouterPhase   string
	appRouterError   string
	panelServer      *http.Server
	panelListener    net.Listener
	panelTLS         bool
	panelMoving      bool
	capabilities     map[string]NodeCapabilities
	downloadInitMu   sync.Mutex
	downloadManager  *DownloadManager
}

func newID() string                        { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (a *App) dataPath(name string) string { return filepath.Join(a.root, "data", name) }
func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func (a *App) load() error {
	a.cfg = Settings{StartPort: 5060, Nodes: []Node{}, Subscriptions: []Subscription{}, Updated: map[string]string{}, OpenBrowser: true, ResumeNodes: true, DefaultRoute: "inherit", LogLevel: "warning", SubscribeHours: 6, HealthMonitor: true, HealthInterval: 60, AutoDownload: true, AutoDeploy: serverPlatform()}
	err := a.readState("configuration", &a.cfg)
	if errors.Is(err, errStateMissing) {
		legacy, legacyErr := os.ReadFile(a.dataPath("settings.json"))
		if legacyErr == nil {
			if err = json.Unmarshal(legacy, &a.cfg); err != nil {
				return fmt.Errorf("旧配置文件损坏，请保留 data/settings.json: %w", err)
			}
			if err = a.save(); err != nil {
				return err
			}
		} else if !errors.Is(legacyErr, os.ErrNotExist) {
			return legacyErr
		} else {
			a.cfg.RoutingMode = "bypass-cn"
			return a.save()
		}
	} else if err != nil {
		return err
	}
	if a.cfg.StartPort < 1024 || a.cfg.StartPort > 65000 {
		a.cfg.StartPort = 5060
	}
	if a.cfg.Updated == nil {
		a.cfg.Updated = map[string]string{}
	}
	if a.cfg.SubscribeHours < 1 {
		a.cfg.SubscribeHours = 6
	}
	if a.cfg.HealthInterval < 30 || a.cfg.HealthInterval > 3600 {
		a.cfg.HealthInterval = 60
	}
	if a.cfg.DefaultRoute == "" {
		a.cfg.DefaultRoute = "inherit"
	}
	if a.cfg.RoutingMode == "" {
		// Adopt the requested global default for legacy default/proxy entries.
		// Existing bypass overrides, identity, names and ports stay intact.
		a.cfg.RoutingMode = "bypass-cn"
		if a.cfg.DefaultRoute == "proxy" {
			a.cfg.DefaultRoute = "inherit"
		}
		for i := range a.cfg.Nodes {
			if a.cfg.Nodes[i].Route == "" || a.cfg.Nodes[i].Route == "proxy" {
				a.cfg.Nodes[i].Route = "inherit"
			}
		}
	}
	if a.cfg.LogLevel == "" {
		a.cfg.LogLevel = "warning"
	}
	if a.cfg.SystemProxyMode == "" {
		a.cfg.SystemProxyMode = "unchanged"
	}
	// PAC mode was removed: migrate any stored choice back to the safe default.
	if a.cfg.SystemProxyMode == "pac" {
		a.cfg.SystemProxyMode = "unchanged"
		a.cfg.SystemProxyNode = ""
		a.cfg.PausedSystemProxy = nil
	}
	appRoutingChanged := false
	if a.cfg.AppRouting.Secret == "" {
		a.cfg.AppRouting.Secret = newID() + newID()
		appRoutingChanged = true
	}
	if a.cfg.AppRouting.Profiles == nil {
		a.cfg.AppRouting.Profiles = []AppRouteProfile{}
		appRoutingChanged = true
	}
	// TUN is never resumed silently: Windows must show a fresh UAC prompt and a
	// stale database flag must not claim routing is active after a crash.
	if a.cfg.AppRouting.Enabled {
		a.cfg.AppRouting.Enabled = false
		appRoutingChanged = true
	}
	if !a.cfg.AppRouting.StrictRoute {
		a.cfg.AppRouting.StrictRoute = true
		appRoutingChanged = true
	}
	if appRoutingChanged {
		return a.save()
	}
	return nil
}
func (a *App) save() error { return a.writeState("configuration", a.cfg) }
func cloneSettings(s Settings) Settings {
	b, _ := json.Marshal(s)
	var v Settings
	_ = json.Unmarshal(b, &v)
	return v
}
func fingerprint(n Node) string {
	b, _ := json.Marshal(n.Outbound)
	h := sha256.Sum256(append([]byte(n.Engine), b...))
	return hex.EncodeToString(h[:])
}
func (a *App) event(s string) {
	a.events = append(a.events, time.Now().Format("15:04:05")+"  "+s)
	if len(a.events) > 100 {
		a.events = a.events[len(a.events)-100:]
	}
}
func (a *App) node(id string) *Node {
	for i := range a.cfg.Nodes {
		if a.cfg.Nodes[i].ID == id {
			return &a.cfg.Nodes[i]
		}
	}
	return nil
}
func portAvailable(port int) bool {
	l, e := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if e != nil {
		return false
	}
	defer l.Close()
	u, e := net.ListenPacket("udp4", fmt.Sprintf("127.0.0.1:%d", port))
	if e != nil {
		return false
	}
	_ = u.Close()
	return true
}
func (a *App) allocatePort() (int, error) {
	used := map[int]bool{a.panelPort: true, a.cfg.Remote.Port: true}
	for _, p := range a.metricsPorts {
		used[p] = true
	}
	for _, n := range a.cfg.Nodes {
		if n.Port > 0 {
			used[n.Port] = true
		}
		if n.AppPort > 0 {
			used[n.AppPort] = true
		}
	}
	for p := a.cfg.StartPort; p <= 65535; p++ {
		if !used[p] && portAvailable(p) {
			return p, nil
		}
	}
	return 0, errors.New("没有可用本地端口")
}

// Application-routing ports are internal, authenticated and deliberately kept
// away from the user-facing 506x range.
func (a *App) allocateAppPort() (int, error) {
	used := map[int]bool{a.panelPort: true, a.cfg.Remote.Port: true}
	for _, p := range a.metricsPorts {
		used[p] = true
	}
	for _, n := range a.cfg.Nodes {
		if n.Port > 0 {
			used[n.Port] = true
		}
		if n.AppPort > 0 {
			used[n.AppPort] = true
		}
	}
	for p := 25060; p <= 45060; p++ {
		if !used[p] && portAvailable(p) {
			return p, nil
		}
	}
	return 0, errors.New("没有可用的应用代理内部端口")
}
func (a *App) importNodes(nodes []Node, source string) int {
	seen := map[string]bool{}
	for _, n := range a.cfg.Nodes {
		seen[fingerprint(n)] = true
	}
	count := 0
	for _, n := range nodes {
		k := fingerprint(n)
		if seen[k] {
			continue
		}
		seen[k] = true
		n.ID = newID()
		n.Enabled = false
		n.Port = 0
		n.Source = source
		n.Route = first(a.cfg.DefaultRoute, "inherit")
		a.cfg.Nodes = append(a.cfg.Nodes, n)
		count++
	}
	return count
}

// coreKey names the dedicated core process of a node. Every node runs in its
// own process so that starting or stopping one node never restarts the
// cores of the other nodes and never drops their connections.
func (n Node) coreKey() string {
	engine := n.Engine
	if engine == "" {
		engine = "xray"
	}
	return engine + "-" + n.ID
}
// coreEngine maps a core key back to the engine that provides it. Node keys
// look like "xray-<id>" or "sing-box-<id>".
func coreEngine(key string) string {
	if strings.HasPrefix(key, "sing-box") {
		return "sing-box"
	}
	if index := strings.IndexByte(key, '-'); index > 0 {
		return key[:index]
	}
	return key
}
func (a *App) engineRunning(engine string) bool {
	for k, c := range a.cores {
		if coreEngine(k) == engine && c.alive() {
			return true
		}
	}
	return false
}
func (a *App) requestQuit() { a.quitOnce.Do(func() { close(a.shutdown) }) }
