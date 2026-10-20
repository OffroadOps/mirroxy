package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Totals survive process restarts; raw counters are scoped to the core instance.
type Traffic struct {
	Uploaded                 uint64    `json:"uploadedBytes"`
	Downloaded               uint64    `json:"downloadedBytes"`
	UploadRate               float64   `json:"uploadBytesPerSecond"`
	DownloadRate             float64   `json:"downloadBytesPerSecond"`
	Connections              int       `json:"connections"`
	Available                bool      `json:"available"`
	ConnectionCountAvailable bool      `json:"connectionCountAvailable"`
	UpdatedAt                time.Time `json:"updatedAt"`
	Since                    time.Time `json:"since"`
	rawUp, rawDown           uint64
	core                     *Core
}

var metricsClient = &http.Client{Timeout: 700 * time.Millisecond, Transport: &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 2}}

func (a *App) loadTraffic() error {
	a.traffic = map[string]*Traffic{}
	e := a.readState("counters", &a.traffic)
	if errors.Is(e, errStateMissing) {
		b, legacyErr := os.ReadFile(a.dataPath("traffic.json"))
		if errors.Is(legacyErr, os.ErrNotExist) {
			return nil
		}
		if legacyErr != nil {
			return legacyErr
		}
		if e = json.Unmarshal(b, &a.traffic); e != nil {
			return fmt.Errorf("旧流量记录无法读取，已保留原文件：%w", e)
		}
		if e = a.writeState("counters", a.traffic); e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	for id, s := range a.traffic {
		if s == nil {
			delete(a.traffic, id)
			continue
		}
		s.UploadRate = 0
		s.DownloadRate = 0
		s.Connections = 0
		s.Available = false
		s.ConnectionCountAvailable = false
	}
	return nil
}
func (a *App) trafficFor(id string) *Traffic {
	if a.traffic == nil {
		a.traffic = map[string]*Traffic{}
	}
	if a.traffic[id] == nil {
		a.traffic[id] = &Traffic{Since: time.Now()}
	}
	return a.traffic[id]
}
func (a *App) ensureMetricsPort(key string) error {
	if a.metricsPorts == nil {
		a.metricsPorts = map[string]int{}
	}
	if a.metricsSecret == "" {
		a.metricsSecret = newID() + newID()
	}
	if a.metricsPorts[key] > 0 {
		return nil
	}
	// Let Windows allocate an unused ephemeral loopback port. It never occupies a node's saved port.
	for attempt := 0; attempt < 20; attempt++ {
		l, e := net.Listen("tcp4", "127.0.0.1:0")
		if e != nil {
			return e
		}
		p := l.Addr().(*net.TCPAddr).Port
		l.Close()
		used := p == a.panelPort || p == a.cfg.Remote.Port
		for _, n := range a.cfg.Nodes {
			used = used || n.Port == p
		}
		for _, mp := range a.metricsPorts {
			used = used || mp == p
		}
		if !used {
			a.metricsPorts[key] = p
			return nil
		}
	}
	return errors.New("无法分配统计端口")
}
func metricsJSON(address, secret string, value any) error {
	r, _ := http.NewRequest("GET", address, nil)
	if secret != "" {
		r.Header.Set("Authorization", "Bearer "+secret)
	}
	res, e := metricsClient.Do(r)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("metrics HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(value)
}
func (s *Traffic) record(c *Core, up, down uint64, now time.Time) {
	du, dd := up, down
	elapsed := now.Sub(s.UpdatedAt).Seconds()
	if s.core == c {
		if up >= s.rawUp {
			du = up - s.rawUp
		}
		if down >= s.rawDown {
			dd = down - s.rawDown
		}
	}
	s.Uploaded += du
	s.Downloaded += dd
	s.UploadRate = 0
	s.DownloadRate = 0
	if s.core == c && elapsed > 0 && elapsed < 10 {
		s.UploadRate = float64(du) / elapsed
		s.DownloadRate = float64(dd) / elapsed
	}
	s.rawUp = up
	s.rawDown = down
	s.core = c
	s.UpdatedAt = now
	s.Available = true
}

// Caller holds App.mu. Native cumulative counters include traffic from connections
// that start and finish between samples, unlike summing a connection snapshot.
func (a *App) sampleTrafficKey(key string) {
	c := a.cores[key]
	p := a.metricsPorts[key]
	if !c.alive() || p == 0 {
		return
	}
	now := time.Now()
	counts, countErr := tcpConnectionCounts(c.cmd.Process.Pid)
	if coreEngine(key) == "xray" {
		var data struct {
			Stats struct {
				Inbound map[string]struct {
					Uplink   uint64
					Downlink uint64
				}
			}
		}
		err := metricsJSON(fmt.Sprintf("http://127.0.0.1:%d/debug/vars", p), "", &data)
		type totals struct{ up, down uint64 }
		perNode := map[string]totals{}
		for tag, v := range data.Stats.Inbound {
			id := ""
			if strings.HasPrefix(tag, "app-in-") {
				id = strings.TrimPrefix(tag, "app-in-")
			} else if strings.HasPrefix(tag, "in-") {
				id = strings.TrimPrefix(tag, "in-")
			}
			if id != "" {
				t := perNode[id]
				t.up += v.Uplink
				t.down += v.Downlink
				perNode[id] = t
			}
		}
		for id, v := range perNode {
			a.trafficFor(id).record(c, v.up, v.down, now)
		}
		for _, n := range a.cfg.Nodes {
			if n.coreKey() != key {
				continue
			}
			s := a.trafficFor(n.ID)
			s.Available = err == nil && n.Enabled
			s.Connections = counts[n.Port] + counts[n.AppPort]
			s.ConnectionCountAvailable = countErr == nil
			if err != nil {
				s.UploadRate = 0
				s.DownloadRate = 0
			}
		}
	} else {
		var data struct {
			UploadTotal   uint64
			DownloadTotal uint64
		}
		e := metricsJSON(fmt.Sprintf("http://127.0.0.1:%d/connections", p), a.metricsSecret, &data)
		id := strings.TrimPrefix(key, "sing-box-")
		s := a.trafficFor(id)
		if e == nil {
			s.record(c, data.UploadTotal, data.DownloadTotal, now)
		} else {
			s.Available = false
			s.UploadRate = 0
			s.DownloadRate = 0
		}
		if n := a.node(id); n != nil {
			s.Connections = counts[n.Port] + counts[n.AppPort]
			s.ConnectionCountAvailable = countErr == nil
		}
	}
}
func (a *App) sampleTraffic() {
	for k := range a.cores {
		a.sampleTrafficKey(k)
	}
	for _, n := range a.cfg.Nodes {
		if !n.Enabled || !a.cores[n.coreKey()].alive() {
			s := a.trafficFor(n.ID)
			s.Connections = 0
			s.UploadRate = 0
			s.DownloadRate = 0
			s.Available = false
			s.ConnectionCountAvailable = true
		}
	}
}
func (a *App) trafficLoop() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-a.shutdown:
			return
		case <-tick.C:
			a.mu.Lock()
			a.sampleTraffic()
			if time.Since(a.trafficSaved) > 10*time.Second {
				if e := a.writeState("counters", a.traffic); e != nil {
					a.event("流量保存失败：" + e.Error())
				}
				a.trafficSaved = time.Now()
			}
			a.mu.Unlock()
		}
	}
}
func (a *App) trafficState() M {
	nodes := M{}
	var up, down uint64
	var ur, dr float64
	count := 0
	for _, n := range a.cfg.Nodes {
		s := *a.trafficFor(n.ID)
		nodes[n.ID] = s
		up += s.Uploaded
		down += s.Downloaded
		ur += s.UploadRate
		dr += s.DownloadRate
		count += s.Connections
	}
	return M{"nodes": nodes, "health": a.healthState(), "uploadedBytes": up, "downloadedBytes": down, "uploadBytesPerSecond": ur, "downloadBytesPerSecond": dr, "connections": count, "sampleIntervalSeconds": 1, "connectionScope": "established inbound TCP connections (including SOCKS UDP control TCP); UDP flows are not counted", "trafficScope": "per local inbound; upload/download include bypassed traffic, exclude outer tunnel overhead"}
}
