package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxActivityEntries = 200

type ActivityEntry struct {
	Time     string    `json:"time"`
	Host     string    `json:"host"`
	Port     string    `json:"port,omitempty"`
	Network  string    `json:"network,omitempty"`
	NodeID   string    `json:"nodeId,omitempty"`
	NodeName string    `json:"nodeName,omitempty"`
	Engine   string    `json:"engine,omitempty"`
	Action   string    `json:"action"`
	Rule     string    `json:"rule,omitempty"`
	Live     bool      `json:"live,omitempty"`
	ts       time.Time `json:"-"`
}

func parseXrayAccessLine(line string) (ActivityEntry, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ActivityEntry{}, false
	}
	verb := "accepted"
	verbToken := " accepted "
	idx := strings.Index(trimmed, verbToken)
	if idx < 0 {
		verb = "rejected"
		verbToken = " rejected "
		idx = strings.Index(trimmed, verbToken)
	}
	if idx < 0 {
		return ActivityEntry{}, false
	}
	left := strings.TrimSpace(trimmed[:idx])
	right := strings.TrimSpace(trimmed[idx+len(verbToken):])
	if right == "" {
		return ActivityEntry{}, false
	}
	destToken := right
	rest := ""
	if sp := strings.IndexAny(right, " \t"); sp >= 0 {
		destToken = right[:sp]
		rest = strings.TrimSpace(right[sp+1:])
	}
	lb := strings.IndexByte(rest, '[')
	rb := strings.LastIndexByte(rest, ']')
	if lb < 0 || rb <= lb {
		return ActivityEntry{}, false
	}
	tagExpr := strings.TrimSpace(rest[lb+1 : rb])
	sep := " -> "
	sepIdx := strings.Index(tagExpr, sep)
	if sepIdx < 0 {
		sep = " >> "
		sepIdx = strings.Index(tagExpr, sep)
	}
	if sepIdx < 0 {
		return ActivityEntry{}, false
	}
	inTag := strings.TrimSpace(tagExpr[:sepIdx])
	outTag := strings.TrimSpace(tagExpr[sepIdx+len(sep):])
	// Xray routes its own DoH transport through this internal tag; those
	// connections are not user traffic and would otherwise be attributed to
	// the current node in the live activity view.
	if inTag == "dns-in" {
		return ActivityEntry{}, false
	}

	network := ""
	if strings.HasPrefix(destToken, "tcp:") {
		network = "tcp"
		destToken = destToken[4:]
	} else if strings.HasPrefix(destToken, "udp:") {
		network = "udp"
		destToken = destToken[4:]
	}
	if destToken == "" {
		return ActivityEntry{}, false
	}
	host := destToken
	port := ""
	if h, p, err := net.SplitHostPort(destToken); err == nil && h != "" {
		host = h
		port = p
	} else {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	host = strings.TrimSpace(host)
	if host == "" || strings.ContainsAny(host, " \t\r\n") {
		return ActivityEntry{}, false
	}

	nodeID := ""
	if strings.HasPrefix(inTag, "app-in-") {
		nodeID = strings.TrimPrefix(inTag, "app-in-")
	} else if strings.HasPrefix(inTag, "in-") {
		nodeID = strings.TrimPrefix(inTag, "in-")
	}
	action := "proxy"
	switch strings.ToLower(outTag) {
	case "direct":
		action = "direct"
	case "block", "reject":
		action = "block"
	default:
		if verb == "rejected" {
			action = "block"
		}
	}

	ts := time.Now()
	if fields := strings.Fields(left); len(fields) >= 2 {
		candidate := fields[0] + " " + fields[1]
		if parsed, err := time.ParseInLocation("2006/01/02 15:04:05.999999", candidate, time.Local); err == nil {
			ts = parsed
		} else if parsed, err := time.ParseInLocation("2006/01/02 15:04:05", candidate, time.Local); err == nil {
			ts = parsed
		}
	}
	return ActivityEntry{
		Time:    ts.Format("15:04:05"),
		Host:    host,
		Port:    port,
		Network: network,
		NodeID:  nodeID,
		Engine:  "A",
		Action:  action,
		ts:      ts,
	}, true
}

type coreLogWriter struct {
	mu       sync.Mutex
	app      *App
	out      *os.File
	nodeID   string
	nodeName string
	buf      []byte
}

func newCoreLogWriter(app *App, out *os.File, nodeID, nodeName string) *coreLogWriter {
	return &coreLogWriter{app: app, out: out, nodeID: nodeID, nodeName: nodeName}
}

func (w *coreLogWriter) processLineLocked(line string) {
	if entry, ok := parseXrayAccessLine(line); ok {
		if entry.NodeID == "" {
			entry.NodeID = w.nodeID
		}
		if entry.NodeName == "" {
			entry.NodeName = w.nodeName
		}
		if w.app != nil {
			w.app.recordActivity(entry)
		}
		return
	}
	if w.out != nil {
		_, _ = w.out.WriteString(line + "\n")
	}
}

func (w *coreLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(bytes.TrimRight(w.buf[:idx], "\r"))
		w.buf = w.buf[idx+1:]
		w.processLineLocked(line)
	}
	if len(w.buf) > 64<<10 {
		line := string(bytes.TrimRight(w.buf, "\r"))
		w.buf = nil
		w.processLineLocked(line)
	}
	return len(p), nil
}

func (w *coreLogWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) == 0 {
		return
	}
	line := string(bytes.TrimRight(w.buf, "\r"))
	w.buf = nil
	if line != "" {
		w.processLineLocked(line)
	}
}

func (a *App) recordActivity(entry ActivityEntry) {
	if entry.ts.IsZero() {
		entry.ts = time.Now()
		entry.Time = entry.ts.Format("15:04:05")
	}
	a.activityMu.Lock()
	defer a.activityMu.Unlock()
	a.activity = append(a.activity, entry)
	if len(a.activity) > maxActivityEntries {
		a.activity = append([]ActivityEntry(nil), a.activity[len(a.activity)-maxActivityEntries:]...)
	}
}

func (a *App) clearActivity() {
	a.activityMu.Lock()
	a.activity = nil
	a.activityMu.Unlock()
}

func (a *App) snapshotActivity() []ActivityEntry {
	a.activityMu.Lock()
	items := make([]ActivityEntry, len(a.activity))
	copy(items, a.activity)
	a.activityMu.Unlock()

	type singTarget struct {
		nodeID   string
		nodeName string
		port     int
		secret   string
	}
	var targets []singTarget
	a.mu.Lock()
	secret := a.metricsSecret
	for k, c := range a.cores {
		if coreEngine(k) != "sing-box" || !c.alive() {
			continue
		}
		p := a.metricsPorts[k]
		if p <= 0 {
			continue
		}
		nodeID := strings.TrimPrefix(k, "sing-box-")
		nodeName := ""
		if n := a.node(nodeID); n != nil {
			nodeName = n.Name
		}
		targets = append(targets, singTarget{nodeID: nodeID, nodeName: nodeName, port: p, secret: secret})
	}
	a.mu.Unlock()

	for _, t := range targets {
		var resp struct {
			Connections []struct {
				Metadata struct {
					Network         string `json:"network"`
					Host            string `json:"host"`
					DestinationIP   string `json:"destinationIP"`
					DestinationPort string `json:"destinationPort"`
				} `json:"metadata"`
				Start  string   `json:"start"`
				Chains []string `json:"chains"`
				Rule   string   `json:"rule"`
			} `json:"connections"`
		}
		if err := metricsJSON(fmt.Sprintf("http://127.0.0.1:%d/connections", t.port), t.secret, &resp); err != nil {
			continue
		}
		for _, conn := range resp.Connections {
			host := strings.TrimSpace(conn.Metadata.Host)
			if host == "" {
				host = strings.TrimSpace(conn.Metadata.DestinationIP)
			}
			if host == "" {
				continue
			}
			action := "proxy"
			for _, ch := range conn.Chains {
				low := strings.ToLower(strings.TrimSpace(ch))
				if low == "block" || low == "reject" {
					action = "block"
					break
				}
				if low == "direct" {
					action = "direct"
				}
			}
			ts := time.Now()
			if conn.Start != "" {
				if parsed, err := time.Parse(time.RFC3339Nano, conn.Start); err == nil {
					ts = parsed.Local()
				} else if parsed, err := time.Parse(time.RFC3339, conn.Start); err == nil {
					ts = parsed.Local()
				}
			}
			items = append(items, ActivityEntry{
				Time:     ts.Format("15:04:05"),
				Host:     host,
				Port:     strings.TrimSpace(conn.Metadata.DestinationPort),
				Network:  strings.ToLower(strings.TrimSpace(conn.Metadata.Network)),
				NodeID:   t.nodeID,
				NodeName: t.nodeName,
				Engine:   "B",
				Action:   action,
				Rule:     strings.TrimSpace(conn.Rule),
				Live:     true,
				ts:       ts,
			})
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].ts.Equal(items[j].ts) {
			return items[i].ts.After(items[j].ts)
		}
		return items[i].Live && !items[j].Live
	})
	if len(items) > maxActivityEntries {
		items = items[:maxActivityEntries]
	}
	return items
}
