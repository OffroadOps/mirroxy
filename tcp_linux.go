//go:build linux

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func tcpConnectionCounts(pid int) (map[int]int, error) {
	base := filepath.Join("/proc", strconv.Itoa(pid))
	fds, err := os.ReadDir(filepath.Join(base, "fd"))
	if err != nil {
		return nil, err
	}
	inodes := map[string]bool{}
	for _, fd := range fds {
		target, e := os.Readlink(filepath.Join(base, "fd", fd.Name()))
		if e == nil && strings.HasPrefix(target, "socket:[") {
			inodes[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] = true
		}
	}
	counts := map[int]int{}
	for _, name := range []string{"tcp", "tcp6"} {
		f, e := os.Open(filepath.Join(base, "net", name))
		if os.IsNotExist(e) && name == "tcp6" {
			continue
		}
		if e != nil {
			return nil, e
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 10 || fields[3] != "01" || !inodes[fields[9]] {
				continue
			}
			_, port, ok := strings.Cut(fields[1], ":")
			if !ok {
				f.Close()
				return nil, fmt.Errorf("invalid /proc TCP address")
			}
			p, e := strconv.ParseUint(port, 16, 16)
			if e != nil {
				f.Close()
				return nil, e
			}
			counts[int(p)]++
		}
		err = scanner.Err()
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return counts, nil
}
