package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// shutdownExisting asks an already running local Mirroxy process to save its
// state and exit. Installers use this before replacing binaries so data and
// any system-proxy recovery state are handled by the application itself.
func shutdownExisting(port int) error {
	if port < 1024 || port > 65535 {
		return errors.New("invalid panel port")
	}
	// The panel serves HTTPS with the built-in CA; probe both schemes. The
	// certificate check is skipped because this call targets our own loopback
	// panel before any CA trust is guaranteed.
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "localhost"}}}
	var state struct {
		Version string `json:"version"`
		Token   string `json:"token"`
	}
	base := ""
	for _, scheme := range []string{"https", "http"} {
		candidate := fmt.Sprintf("%s://127.0.0.1:%d", scheme, port)
		response, err := client.Get(candidate + "/api/state")
		if err != nil {
			continue // Not serving this scheme (or nothing at all).
		}
		defer response.Body.Close()
		if response.StatusCode == http.StatusOK && json.NewDecoder(response.Body).Decode(&state) == nil && state.Version != "" && state.Token != "" {
			base = candidate
			break
		}
		return errors.New("panel port is occupied by a non-Mirroxy process")
	}
	if base == "" {
		return nil // No local backend is running.
	}
	request, err := http.NewRequest(http.MethodPost, base+"/api/shutdown", bytes.NewBufferString("{}"))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-App-Token", state.Token)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("shutdown request failed: HTTP %d", response.StatusCode)
	}
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		probe, probeErr := client.Get(base + "/api/state")
		if probeErr != nil {
			return nil
		}
		probe.Body.Close()
	}
	return errors.New("Mirroxy did not exit before the installer timeout")
}
