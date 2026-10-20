//go:build windows

package main

import (
	"crypto/rand"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"unsafe"
)

func blob(data []byte) windows.DataBlob {
	if len(data) == 0 {
		return windows.DataBlob{}
	}
	return windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
}
func dpapi(data []byte, protect bool) ([]byte, error) {
	in := blob(data)
	var out windows.DataBlob
	var err error
	if protect {
		name, _ := windows.UTF16PtrFromString("Mirroxy local state key")
		err = windows.CryptProtectData(&in, name, nil, 0, nil, 0x1, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, 0x1, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
func readExistingDataKey(path string) ([]byte, error) {
	if wrapped, e := os.ReadFile(path); e == nil {
		key, e := dpapi(wrapped, false)
		if e != nil {
			return nil, e
		}
		if len(key) != 32 {
			return nil, errors.New("密钥长度无效")
		}
		return key, nil
	} else {
		return nil, e
	}
}
func loadOrCreateDataKey(path string) ([]byte, error) {
	if key, e := readExistingDataKey(path); e == nil {
		return key, nil
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	key := make([]byte, 32)
	if _, e := rand.Read(key); e != nil {
		return nil, e
	}
	wrapped, e := dpapi(key, true)
	if e != nil {
		return nil, e
	}
	if e = os.WriteFile(path, wrapped, 0600); e != nil {
		return nil, e
	}
	return key, nil
}
