//go:build linux

package main

import (
	"net"
	"os"
	"testing"
)

func TestLinuxConnectionCountTracksOwnedAcceptedSockets(t *testing.T) {
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	c, e := net.Dial("tcp4", l.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	accepted, e := l.Accept()
	if e != nil {
		t.Fatal(e)
	}
	defer accepted.Close()
	counts, e := tcpConnectionCounts(os.Getpid())
	if e != nil {
		t.Fatal(e)
	}
	if counts[l.Addr().(*net.TCPAddr).Port] != 1 {
		t.Fatal("accepted connection missing", counts)
	}
	accepted.Close()
	c.Close()
	counts, e = tcpConnectionCounts(os.Getpid())
	if e != nil {
		t.Fatal(e)
	}
	if counts[l.Addr().(*net.TCPAddr).Port] != 0 {
		t.Fatal("closed connection remains counted")
	}
}
