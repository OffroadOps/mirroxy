//go:build darwin

package main

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func tcpConnectionCounts(pid int) (map[int]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// -a intersects the PID, protocol and established-state filters; -Fn emits
	// machine-readable addresses. lsof exits 1 without output for no matches.
	output, err := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "-a", "-p", strconv.Itoa(pid), "-iTCP", "-sTCP:ESTABLISHED", "-Fn").Output()
	counts := map[int]int{}
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 && len(output) == 0 && len(exit.Stderr) == 0 {
			return counts, nil
		}
		return nil, err
	}
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.HasPrefix(line, "n") {
			continue
		}
		local, _, ok := strings.Cut(line[1:], "->")
		if !ok {
			continue
		}
		_, port, e := net.SplitHostPort(local)
		if e != nil {
			return nil, e
		}
		p, e := strconv.Atoi(port)
		if e != nil {
			return nil, e
		}
		counts[p]++
	}
	return counts, nil
}
