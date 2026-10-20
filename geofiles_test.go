package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGeoCatalogMatchesRuleAssets(t *testing.T) {
	seenName, seenAsset := map[string]bool{}, map[string]bool{}
	for _, spec := range geoCatalog {
		if seenName[spec.name] {
			t.Fatalf("duplicate geo spec %s", spec.name)
		}
		seenName[spec.name] = true
		if ruleAssetName(spec.name) != spec.asset {
			t.Fatalf("%s maps to %s, catalog says %s", spec.name, ruleAssetName(spec.name), spec.asset)
		}
		if seenAsset[spec.asset] {
			t.Fatalf("duplicate asset %s", spec.asset)
		}
		seenAsset[spec.asset] = true
		if spec.engine != "xray" && spec.engine != "sing-box" {
			t.Fatalf("%s has invalid engine %s", spec.name, spec.engine)
		}
		if !strings.HasPrefix(spec.source, "https://") {
			t.Fatalf("%s source must be https", spec.name)
		}
		if spec.kind == "dat" && spec.checksum == "" {
			t.Fatalf("%s must ship a checksum URL", spec.name)
		}
		if spec.kind == "srs" && spec.checksum != "" {
			t.Fatalf("%s should not declare a checksum", spec.name)
		}
	}
	if len(geoCatalog) != 14 {
		t.Fatalf("expected 14 geo specs, got %d", len(geoCatalog))
	}
}

func TestGeoFilesStateReportsFiles(t *testing.T) {
	a := geoTestApp(t)
	a.cfg.RoutingMode = "bypass-cn"
	resources := filepath.Join(a.root, "bin", "resources")
	if e := os.MkdirAll(resources, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(resources, "Mirroxy_set_01.bin"), []byte("0123456789"), 0600); e != nil {
		t.Fatal(e)
	}
	a.cfg.Updated["geo:geoip-cn"] = "srs@202609290000"
	byName := map[string]M{}
	for _, item := range a.geoFilesState() {
		byName[item["name"].(string)] = item
	}
	if len(byName) != 14 {
		t.Fatalf("expected 14 entries, got %d", len(byName))
	}
	cn := byName["geoip-cn"]
	if cn["exists"] != true || cn["size"] != int64(10) || cn["version"] != "srs@202609290000" {
		t.Fatalf("geoip-cn state wrong: %v", cn)
	}
	if cn["used"] != true || byName["geosite-private"]["used"] != true {
		t.Fatal("bypass-cn mode should use cn sets")
	}
	if byName["geoip-netflix"]["used"] != false {
		t.Fatal("blacklist-only set must be unused in bypass-cn mode")
	}
	if byName["geoip"]["exists"] != false {
		t.Fatal("missing dat must report exists=false")
	}
	a.cfg.RoutingMode = "blacklist"
	byName = map[string]M{}
	for _, item := range a.geoFilesState() {
		byName[item["name"].(string)] = item
	}
	if byName["geoip-netflix"]["used"] != true || byName["geoip-cn"]["used"] != false {
		t.Fatal("blacklist mode usage wrong")
	}
}

// geoTestServer serves the release tag plus fake rule payloads and counts hits.
func geoTestServer(t *testing.T, tag *string, datPayload, srsPayload []byte, datSum string) (*httptest.Server, *int64) {
	var hits int64
	mux := http.NewServeMux()
	mux.HandleFunc("/release", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		fmt.Fprintf(w, `{"tag_name":%q}`, *tag)
	})
	mux.HandleFunc("/geoip.dat", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Write(datPayload)
	})
	mux.HandleFunc("/geoip.dat.sha256sum", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		fmt.Fprintf(w, "%s  geoip.dat\n", datSum)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Write(srsPayload)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, &hits
}

func geoTestApp(t *testing.T) *App {
	a := &App{root: t.TempDir(), panelPort: freeTestPort(t), token: "ui-token", cores: map[string]*Core{}, shutdown: make(chan struct{})}
	if e := a.load(); e != nil {
		t.Fatal(e)
	}
	return a
}

// geoOverride repoints the whole catalog (release endpoint included) at the
// given test server so tests never touch the live sources.
func geoOverride(t *testing.T, serverURL string) (restore func()) {
	original := append([]geoSpec{}, geoCatalog...)
	endpoint := geoReleaseEndpoint
	for i := range geoCatalog {
		if geoCatalog[i].kind == "dat" {
			geoCatalog[i].source = serverURL + "/geoip.dat"
			geoCatalog[i].checksum = serverURL + "/geoip.dat.sha256sum"
		} else {
			geoCatalog[i].source = serverURL + "/set.srs"
		}
	}
	geoReleaseEndpoint = serverURL + "/release"
	return func() {
		geoCatalog = original
		geoReleaseEndpoint = endpoint
	}
}

func TestGeoUpdateFlow(t *testing.T) {
	datPayload := strings.Repeat("D", 2<<20)
	srsPayload := strings.Repeat("S", 4096)
	sum := sha256.Sum256([]byte(datPayload))
	tag := "202609290000"
	server, hits := geoTestServer(t, &tag, []byte(datPayload), []byte(srsPayload), hex.EncodeToString(sum[:]))
	a := geoTestApp(t)
	restore := geoOverride(t, server.URL)
	defer restore()

	resources := filepath.Join(a.root, "bin", "resources")
	if e := os.MkdirAll(resources, 0700); e != nil {
		t.Fatal(e)
	}
	// Pre-existing file keeps a rollback copy after the swap.
	if e := os.WriteFile(filepath.Join(resources, "Mirroxy_set_01.bin"), []byte("old-content"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := a.updateGeoFiles(nil, false); e != nil {
		t.Fatalf("update failed: %v", e)
	}
	if got, e := os.ReadFile(filepath.Join(resources, "Mirroxy_route_ip.bin")); e != nil || string(got) != datPayload {
		t.Fatal("dat payload not swapped")
	}
	if got, e := os.ReadFile(filepath.Join(resources, "Mirroxy_set_01.bin")); e != nil || string(got) != srsPayload {
		t.Fatal("srs payload not swapped")
	}
	if _, e := os.Stat(filepath.Join(resources, "Mirroxy_set_01.bin.previous")); e != nil {
		t.Fatal("previous file missing after swap")
	}
	if a.cfg.Updated["geo:geoip"] != "202609290000" || a.cfg.Updated["geo:geoip-cn"] != "srs@202609290000" {
		t.Fatalf("versions not recorded: %v", a.cfg.Updated)
	}
	if !strings.Contains(strings.Join(a.events, "\n"), "Geo 规则文件已更新") {
		t.Fatal("update event missing")
	}

	// Auto mode with an unchanged tag downloads nothing.
	before := atomic.LoadInt64(hits)
	if e := a.updateGeoFiles(nil, true); e != nil {
		t.Fatalf("skip run failed: %v", e)
	}
	if after := atomic.LoadInt64(hits); after != before+1 {
		t.Fatalf("skip run must only fetch the release tag: %d -> %d", before, after)
	}

	// A new tag triggers a refresh even in auto mode.
	tag = "202609300000"
	if e := a.updateGeoFiles(nil, true); e != nil {
		t.Fatalf("refresh run failed: %v", e)
	}
	if a.cfg.Updated["geo:geoip"] != "202609300000" {
		t.Fatalf("version not refreshed: %v", a.cfg.Updated["geo:geoip"])
	}

	// A broken checksum keeps the installed file in place.
	tag = "202610010000"
	badServer, _ := geoTestServer(t, &tag, []byte(datPayload), []byte(srsPayload), strings.Repeat("0", 64))
	restore2 := geoOverride(t, badServer.URL)
	defer restore2()
	if e := a.updateGeoFiles([]string{"geoip"}, false); e == nil || !strings.Contains(e.Error(), "校验") {
		t.Fatalf("expected checksum failure, got %v", e)
	}
	if got, e := os.ReadFile(filepath.Join(resources, "Mirroxy_route_ip.bin")); e != nil || string(got) != datPayload {
		t.Fatal("installed dat must survive a failed update")
	}
}

func TestGeoUpdateIdenticalContentSkipsRestart(t *testing.T) {
	payload := strings.Repeat("S", 2048)
	tag := "202609290000"
	server, _ := geoTestServer(t, &tag, []byte(strings.Repeat("D", 2<<20)), []byte(payload), strings.Repeat("0", 64))
	a := geoTestApp(t)
	restore := geoOverride(t, server.URL)
	defer restore()
	resources := filepath.Join(a.root, "bin", "resources")
	if e := os.MkdirAll(resources, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(resources, "Mirroxy_set_01.bin"), []byte(payload), 0600); e != nil {
		t.Fatal(e)
	}
	if e := a.updateGeoFiles([]string{"geoip-cn"}, false); e != nil {
		t.Fatalf("update failed: %v", e)
	}
	if _, e := os.Stat(filepath.Join(resources, "Mirroxy_set_01.bin.previous")); !os.IsNotExist(e) {
		t.Fatal("identical content must not swap files")
	}
	if a.cfg.Updated["geo:geoip-cn"] != "srs@202609290000" {
		t.Fatalf("identical content should still record the version: %v", a.cfg.Updated)
	}
}

func TestGeoFilesAPI(t *testing.T) {
	a := geoTestApp(t)
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:"+fmt.Sprint(a.panelPort)+path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("X-App-Token", token)
		}
		w := httptest.NewRecorder()
		a.handler(false).ServeHTTP(w, r)
		return w
	}
	w := request("POST", "/api/geofiles/update", `{}`, "")
	if w.Code != 403 {
		t.Fatalf("update without token: %d", w.Code)
	}
	w = request("POST", "/api/geofiles/update", `{"files":["nope"]}`, "ui-token")
	if w.Code != 400 || !strings.Contains(w.Body.String(), "未知的规则文件") {
		t.Fatalf("invalid name: %d %s", w.Code, w.Body.String())
	}
	w = request("GET", "/api/state", "", "")
	var state struct {
		GeoFiles []struct {
			Name string `json:"name"`
			Used bool   `json:"used"`
		} `json:"geoFiles"`
		Settings struct {
			GeoAutoUpdate bool `json:"geoAutoUpdate"`
		} `json:"settings"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &state); e != nil {
		t.Fatal(e)
	}
	if len(state.GeoFiles) != 14 {
		t.Fatalf("state geoFiles wrong: %d", len(state.GeoFiles))
	}
	// Persisting the auto-update switch through the settings endpoint.
	w = request("POST", "/api/settings", `{"geoAutoUpdate":true}`, "ui-token")
	if w.Code != 200 {
		t.Fatalf("settings patch failed: %d %s", w.Code, w.Body.String())
	}
	if !a.cfg.GeoAutoUpdate {
		t.Fatal("geoAutoUpdate not persisted")
	}
}
