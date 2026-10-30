package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func subscriptionTestApp(t *testing.T) *App {
	t.Helper()
	a := renameTestApp(t)
	other, err := parseLink("http://user:password@other.invalid:24443#Manual")
	if err != nil {
		t.Fatal(err)
	}
	other.ID, other.Port = "manual-node", 5067
	a.cfg.Nodes[0].Source = "sub"
	a.cfg.Nodes = append(a.cfg.Nodes, other)
	a.cfg.Subscriptions = []Subscription{{ID: "sub", Name: "Airport", URL: "https://provider.invalid/api/v1/client?token=secret-token", Error: "old error"}}
	return a
}

func TestEditSubscriptionKeepsNodesAndValidatesAddress(t *testing.T) {
	a := subscriptionTestApp(t)
	if err := a.editSubscription("sub", "  New name  ", ""); err != nil {
		t.Fatal(err)
	}
	s := a.cfg.Subscriptions[0]
	if s.Name != "New name" || s.URL != "https://provider.invalid/api/v1/client?token=secret-token" || s.Error != "old error" {
		t.Fatalf("rename without an address must keep the address: %+v", s)
	}
	if err := a.editSubscription("sub", "New name", "ftp://bad.invalid/x"); err == nil {
		t.Fatal("non-HTTP address accepted")
	}
	if err := a.editSubscription("sub", "", ""); err == nil {
		t.Fatal("empty name accepted")
	}
	if err := a.editSubscription("sub", "New name", "https://mirror.invalid/sub?token=2"); err != nil {
		t.Fatal(err)
	}
	if s = a.cfg.Subscriptions[0]; s.URL != "https://mirror.invalid/sub?token=2" || s.Error != "" {
		t.Fatalf("new address not stored or stale error kept: %+v", s)
	}
	if a.cfg.Nodes[0].Source != "sub" || a.cfg.Nodes[0].Port != 5066 || len(a.cfg.Nodes) != 2 {
		t.Fatal("editing a subscription changed its nodes")
	}
	reloaded := &App{root: a.root}
	if err := reloaded.load(); err != nil || reloaded.cfg.Subscriptions[0].Name != "New name" {
		t.Fatal("edit did not persist", err)
	}
	if err := a.editSubscription("missing", "x", ""); err == nil {
		t.Fatal("unknown subscription accepted")
	}
}

func TestDeleteSubscriptionKeepsNodesAsManual(t *testing.T) {
	a := subscriptionTestApp(t)
	if err := a.deleteSubscription("sub", true); err != nil {
		t.Fatal(err)
	}
	if len(a.cfg.Subscriptions) != 0 || len(a.cfg.Nodes) != 2 {
		t.Fatalf("keep mode removed nodes: subs=%d nodes=%d", len(a.cfg.Subscriptions), len(a.cfg.Nodes))
	}
	if n := a.cfg.Nodes[0]; n.Source != "" || n.Port != 5066 || n.Name != "Provider" {
		t.Fatalf("kept node changed: %+v", n)
	}
}

func TestDeleteSubscriptionRemovesOnlyItsNodes(t *testing.T) {
	a := subscriptionTestApp(t)
	if err := a.deleteSubscription("sub", false); err != nil {
		t.Fatal(err)
	}
	if len(a.cfg.Nodes) != 1 || a.cfg.Nodes[0].ID != "manual-node" || a.cfg.Nodes[0].Port != 5067 {
		t.Fatalf("delete mode touched other nodes: %+v", a.cfg.Nodes)
	}
	reloaded := &App{root: a.root}
	if err := reloaded.load(); err != nil || len(reloaded.cfg.Subscriptions) != 0 || len(reloaded.cfg.Nodes) != 1 {
		t.Fatal("delete did not persist", err)
	}
	if err := a.deleteSubscription("sub", false); err == nil {
		t.Fatal("deleting a missing subscription succeeded")
	}
}

func TestStateShowsSubscriptionHostButNotToken(t *testing.T) {
	a := subscriptionTestApp(t)
	a.traffic = map[string]*Traffic{}
	b, err := json.Marshal(a.state())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret-token") || strings.Contains(string(b), "/api/v1/client") {
		t.Fatalf("state exposes the subscription address: %s", b)
	}
	if !strings.Contains(string(b), `"host":"provider.invalid"`) {
		t.Fatal("state lacks the subscription host")
	}
}
