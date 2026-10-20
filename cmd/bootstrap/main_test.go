package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureZIP(t *testing.T, version string, mutate func(map[string][]byte)) []byte {
	t.Helper()
	pe := make([]byte, 80)
	copy(pe, "MZ")
	binary.LittleEndian.PutUint32(pe[60:], 64)
	copy(pe[64:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(pe[68:], 0x8664)
	files := map[string][]byte{}
	for _, name := range []string{"Mirroxy.exe", "upgrade.exe", "bin/runtime/Mirroxy_core_A.exe", "bin/runtime/Mirroxy_core_B.exe", "bin/runtime/Mirroxy_downloader.exe"} {
		files[name] = pe
	}
	for _, name := range []string{"README.txt", "PLATFORM", "bin/resources/Mirroxy_route_ip.bin", "bin/resources/Mirroxy_route_domain.bin", "bin/resources/Mirroxy_trust.pem", "licenses/NOTICE.txt"} {
		files[name] = []byte("fixture")
	}
	for i := 1; i <= 12; i++ {
		name := []string{"01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11", "12"}[i-1]
		files["bin/resources/Mirroxy_set_"+name+".bin"] = []byte("fixture")
	}
	hashes := map[string]string{}
	for name, data := range files {
		sum := sha256.Sum256(data)
		hashes[name] = hex.EncodeToString(sum[:])
	}
	files["manifest.json"], _ = json.Marshal(map[string]any{"application": "Mirroxy", "version": version, "platform": "windows-amd64", "files": hashes})
	files["SHA256SUMS"] = []byte("fixture")
	if mutate != nil {
		mutate(files)
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, data := range files {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write(data)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestFixedBootstrapFetchesLatestAndChecksArchive(t *testing.T) {
	version := "0.7.3"
	payload := fixtureZIP(t, version, nil)
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/scrcpyproxy/releases.json") {
			sum := sha256.Sum256(payload)
			json.NewEncoder(w).Encode(map[string]any{"latest": version, "releases": []any{map[string]any{"version": version, "files": []fileOffer{{payloadName, base + "v" + version + "/" + payloadName, hex.EncodeToString(sum[:]), int64(len(payload))}}}}})
			return
		}
		w.Write(payload)
	}))
	defer server.Close()
	base = server.URL + "/scrcpyproxy/"
	fetch, err := newFetcher(base, "", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, next := range []string{"0.7.3", "0.7.4"} {
		version = next
		payload = fixtureZIP(t, version, nil)
		item, err := fetch.latest()
		if err != nil || item.Version != next {
			t.Fatalf("latest: %+v %v", item, err)
		}
		work := t.TempDir()
		path, err := fetch.download(item, work)
		if err != nil {
			t.Fatal(err)
		}
		stage, err := extract(path, work, item.Version)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = os.Stat(filepath.Join(stage, "Mirroxy.exe")); err != nil {
			t.Fatal(err)
		}
		item.File.SHA256 = strings.Repeat("0", 64)
		if _, err = fetch.download(item, work); err == nil {
			t.Fatal("accepted tampered download")
		}
	}
}

func TestBootstrapRejectsUntrustedContents(t *testing.T) {
	cases := map[string]func(map[string][]byte){
		"traversal": func(f map[string][]byte) { f["../outside.exe"] = []byte("bad") },
		"user-data": func(f map[string][]byte) { f["data/Mirroxy.db"] = []byte("secret") },
		"source":    func(f map[string][]byte) { f["main.go"] = []byte("source") },
		"hash":      func(f map[string][]byte) { f["Mirroxy.exe"] = []byte("bad") },
		"platform": func(f map[string][]byte) {
			f["manifest.json"] = bytes.ReplaceAll(f["manifest.json"], []byte("windows-amd64"), []byte("linux-amd64"))
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			work := t.TempDir()
			path := filepath.Join(work, "package.zip")
			os.WriteFile(path, fixtureZIP(t, "0.7.3", mutate), 0600)
			if _, err := extract(path, work, "0.7.3"); err == nil {
				t.Fatal("accepted untrusted contents")
			}
			stages, _ := filepath.Glob(filepath.Join(work, "payload-stage-*"))
			if len(stages) > 0 {
				t.Fatal("staged unverified files")
			}
		})
	}
}

func TestBootstrapSourceAndProxyValidation(t *testing.T) {
	for _, base := range []string{"http://example.com/", "file:///tmp/", "https://u:p@example.com/", "https://example.com/?url=other"} {
		if _, err := newFetcher(base, "", false); err == nil {
			t.Fatal("accepted", base)
		}
	}
	for _, proxy := range []string{"socks5://localhost:10808", "http://localhost:10808/other", "http://localhost:10808?key=secret"} {
		if _, err := newFetcher("https://example.com/releases/", proxy, false); err == nil {
			t.Fatal("accepted proxy", proxy)
		}
	}
	if _, err := newFetcher("http://127.0.0.1:1234/releases/", "", false); err == nil {
		t.Fatal("production allows insecure test source")
	}
}

func TestBootstrapRejectsOffSourceCatalogURL(t *testing.T) {
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"latest": "0.7.3", "releases": []any{map[string]any{"version": "0.7.3", "files": []fileOffer{{payloadName, "https://example.org/program.zip", strings.Repeat("0", 64), 100}}}}})
	}))
	defer server.Close()
	base = server.URL + "/scrcpyproxy/"
	fetch, _ := newFetcher(base, "", true)
	if _, err := fetch.latest(); err == nil {
		t.Fatal("accepted off-source catalog URL")
	}
}
