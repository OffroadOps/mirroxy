package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEncryptedStateDoesNotExposeNodeCredentials(t *testing.T) {
	root := t.TempDir()
	a := &App{root: root}
	secret := "test-node-password-that-must-not-be-plaintext"
	a.cfg = Settings{
		StartPort:     5060,
		Nodes:         []Node{{ID: "one", Name: "private-node", Protocol: "vless", Outbound: M{"password": secret, "uuid": secret}}},
		Subscriptions: []Subscription{{ID: "sub", Name: "private-subscription", URL: "https://example.invalid/" + secret}},
	}
	if err := a.save(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Mirroxy.db", "Mirroxy.key"} {
		payload, err := os.ReadFile(filepath.Join(root, "data", name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(payload, []byte(secret)) || bytes.Contains(payload, []byte("private-node")) {
			t.Fatalf("%s exposes encrypted settings", name)
		}
	}
	var loaded Settings
	if err := a.readState("configuration", &loaded); err != nil || len(loaded.Nodes) != 1 || loaded.Nodes[0].Outbound["password"] != secret {
		t.Fatalf("encrypted round trip failed: %+v, %v", loaded, err)
	}
}

func TestInstalledLegacyDataMigratesWithoutChangingNodes(t *testing.T) {
	legacyDir := os.Getenv("MIRROXY_TEST_LEGACY_DATA")
	if legacyDir == "" {
		t.Skip("set MIRROXY_TEST_LEGACY_DATA to inspect an installed 0.4.x data directory")
	}
	settingsBytes, err := os.ReadFile(filepath.Join(legacyDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var before Settings
	if err = json.Unmarshal(settingsBytes, &before); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err = os.MkdirAll(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dataDir, "settings.json"), settingsBytes, 0600); err != nil {
		t.Fatal(err)
	}
	trafficBytes, trafficErr := os.ReadFile(filepath.Join(legacyDir, "traffic.json"))
	if trafficErr == nil {
		if err = os.WriteFile(filepath.Join(dataDir, "traffic.json"), trafficBytes, 0600); err != nil {
			t.Fatal(err)
		}
	} else if !os.IsNotExist(trafficErr) {
		t.Fatal(trafficErr)
	}
	a := &App{root: root}
	if err = a.load(); err != nil {
		t.Fatal(err)
	}
	if err = a.loadTraffic(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.cfg.Nodes, before.Nodes) || !reflect.DeepEqual(a.cfg.Subscriptions, before.Subscriptions) || a.cfg.StartPort != before.StartPort || a.cfg.SystemProxyMode != before.SystemProxyMode || a.cfg.SystemProxyNode != before.SystemProxyNode {
		t.Fatal("installed node identity, port, state, subscription, or proxy selection changed")
	}
	removeLegacyPlaintext(dataDir)
	if _, err = os.Stat(filepath.Join(dataDir, "settings.json")); !os.IsNotExist(err) {
		t.Fatal("legacy settings remain after successful migration")
	}
	database, err := os.ReadFile(filepath.Join(dataDir, "Mirroxy.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range before.Nodes {
		for _, value := range []string{node.Server, stringVal(node.Outbound["password"]), stringVal(node.Outbound["id"])} {
			if len(value) >= 6 && bytes.Contains(database, []byte(value)) {
				t.Fatal("installed node secret is visible in encrypted database")
			}
		}
	}
}

func TestEncryptedStateRejectsTamperedPayload(t *testing.T) {
	a := &App{root: t.TempDir()}
	a.cfg = Settings{StartPort: 5060, Nodes: []Node{}, Subscriptions: []Subscription{}, Updated: map[string]string{}}
	if err := a.save(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", a.dataPath("Mirroxy.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`UPDATE state SET payload = payload || X'00' WHERE name='configuration'`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	var loaded Settings
	if err = a.readState("configuration", &loaded); err == nil {
		t.Fatal("tampered encrypted state was accepted")
	}
}

func TestLegacyJSONMigrationPreservesStateThenRemovesPlaintext(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	legacy := Settings{StartPort: 5099, Nodes: []Node{{ID: "legacy", Name: "legacy-name", Outbound: M{"password": "legacy-secret"}}}, Subscriptions: []Subscription{}, Updated: map[string]string{}}
	b, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dataDir, "settings.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	traffic := map[string]*Traffic{"legacy": {Uploaded: 123, Downloaded: 456}}
	b, _ = json.Marshal(traffic)
	if err := os.WriteFile(filepath.Join(dataDir, "traffic.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	a := &App{root: root}
	if err := a.load(); err != nil {
		t.Fatal(err)
	}
	if err := a.loadTraffic(); err != nil {
		t.Fatal(err)
	}
	removeLegacyPlaintext(dataDir)
	if a.cfg.StartPort != 5099 || len(a.cfg.Nodes) != 1 || a.cfg.Nodes[0].Outbound["password"] != "legacy-secret" || a.traffic["legacy"].Uploaded != 123 || a.traffic["legacy"].Downloaded != 456 {
		t.Fatal("legacy settings or counters changed during migration")
	}
	for _, name := range []string{"settings.json", "traffic.json"} {
		if _, err := os.Stat(filepath.Join(dataDir, name)); !os.IsNotExist(err) {
			t.Fatalf("legacy plaintext remains: %s", name)
		}
	}
	b, err := os.ReadFile(filepath.Join(dataDir, "Mirroxy.db"))
	if err != nil || bytes.Contains(b, []byte("legacy-secret")) {
		t.Fatal("migrated database exposes plaintext")
	}
}
