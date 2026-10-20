//go:build windows

package main

import (
	"encoding/binary"
	"unsafe"
)

func installerPanelPortOwner(port int) (int, string) {
	var size uint32
	getTCPTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, 2, 5, 0)
	for attempt := 0; attempt < 3; attempt++ {
		if size < 4 || size > 64<<20 {
			return 0, ""
		}
		buffer := make([]byte, size)
		code, _, _ := getTCPTable.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), 0, 2, 5, 0)
		if code == 122 {
			continue
		}
		if code != 0 {
			return 0, ""
		}
		rows := int(binary.LittleEndian.Uint32(buffer))
		for i := 0; i < rows && 4+(i+1)*24 <= len(buffer); i++ {
			row := buffer[4+i*24 : 4+(i+1)*24]
			if binary.LittleEndian.Uint32(row) != 2 || int(binary.BigEndian.Uint16(row[8:10])) != port {
				continue
			}
			// Only wildcard or loopback listeners can block our IPv4 dashboard.
			address := binary.BigEndian.Uint32(row[4:8])
			if address != 0 && address != 0x7f000001 {
				continue
			}
			pid := int(binary.LittleEndian.Uint32(row[20:]))
			return pid, queryApplicationProcessPath(uint32(pid))
		}
		return 0, ""
	}
	return 0, ""
}
