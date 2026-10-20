package main

import (
	"encoding/json"
	"net"
	"net/http"
	"testing"
)

func TestShutdownExisting(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"version": "0.5.0", "token": "test-token"})
	})
	mux.HandleFunc("/api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-App-Token") != "test-token" {
			t.Errorf("missing app token")
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		go server.Close()
	})
	server.Handler = mux
	go server.Serve(listener)
	if err := shutdownExisting(port); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownExistingNoProcess(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	if err := shutdownExisting(port); err != nil {
		t.Fatal(err)
	}
}
