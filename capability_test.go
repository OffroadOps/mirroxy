package main

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestParseSTUNXORMappedAddress(t *testing.T) {
	transaction := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	wantIP := net.ParseIP("203.0.113.7").To4()
	wantPort := uint16(54321)
	packet := make([]byte, 32)
	binary.BigEndian.PutUint16(packet[0:2], 0x0101)
	binary.BigEndian.PutUint16(packet[2:4], 12)
	binary.BigEndian.PutUint32(packet[4:8], 0x2112a442)
	copy(packet[8:20], transaction)
	binary.BigEndian.PutUint16(packet[20:22], 0x0020)
	binary.BigEndian.PutUint16(packet[22:24], 8)
	packet[25] = 1
	binary.BigEndian.PutUint16(packet[26:28], wantPort^0x2112)
	mask := []byte{0x21, 0x12, 0xa4, 0x42}
	for i := range wantIP {
		packet[28+i] = wantIP[i] ^ mask[i]
	}
	got, err := parseSTUNMapping(packet, transaction)
	if err != nil {
		t.Fatal(err)
	}
	if got != "203.0.113.7:54321" {
		t.Fatalf("mapping = %q", got)
	}
}

func TestSOCKSUDPPayloadRejectsFragments(t *testing.T) {
	packet := []byte{0, 0, 1, 1, 127, 0, 0, 1, 0, 53, 1, 2, 3}
	if _, err := socksUDPPayload(packet); err == nil {
		t.Fatal("fragmented SOCKS UDP packet was accepted")
	}
	packet[2] = 0
	payload, err := socksUDPPayload(packet)
	if err != nil || len(payload) != 3 {
		t.Fatalf("payload=%v err=%v", payload, err)
	}
}
