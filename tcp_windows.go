//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"golang.org/x/sys/windows"
	"unsafe"
)

var getTCPTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

func tcpConnectionCounts(pid int) (map[int]int, error) {
	var size uint32
	getTCPTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, 2, 5, 0)
	for attempt := 0; attempt < 3; attempt++ {
		if size < 4 || size > 64<<20 {
			return nil, fmt.Errorf("invalid TCP table size")
		}
		b := make([]byte, size)
		code, _, _ := getTCPTable.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(unsafe.Pointer(&size)), 0, 2, 5, 0)
		if code == 122 {
			continue
		}
		if code != 0 {
			return nil, fmt.Errorf("TCP table: %d", code)
		}
		counts := map[int]int{}
		rows := int(binary.LittleEndian.Uint32(b))
		for i := 0; i < rows; i++ {
			off := 4 + i*24
			if off+24 > len(b) {
				break
			}
			r := b[off : off+24]
			// MIB_TCPROW_OWNER_PID: state, local addr, local port, remote addr, remote port, PID.
			if binary.LittleEndian.Uint32(r) == 5 && int(binary.LittleEndian.Uint32(r[20:])) == pid {
				counts[int(binary.BigEndian.Uint16(r[8:10]))]++
			}
		}
		return counts, nil
	}
	return nil, fmt.Errorf("TCP table changed while reading")
}
