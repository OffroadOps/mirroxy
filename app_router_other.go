//go:build !windows

package main

import "errors"

func platformApplicationRoutingSupported() bool { return false }

func startElevatedApplicationRouter(root, config string, controlPort int, secret string, parentPID int) error {
	return errors.New("应用代理目前仅支持 Windows")
}

func runApplicationRouterHelper(root, config string, controlPort int, secret string, parentPID int) error {
	return errors.New("应用代理目前仅支持 Windows")
}

func listApplicationProcesses() ([]applicationProcess, error) {
	return nil, errors.New("应用代理目前仅支持 Windows")
}
