//go:build linux || darwin

package main

import (
	"crypto/rand"
	"errors"
	"os"
)

func readExistingDataKey(path string) ([]byte, error) {
	if key, e := os.ReadFile(path); e == nil {
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
	if e := os.WriteFile(path, key, 0600); e != nil {
		return nil, e
	}
	return key, nil
}
