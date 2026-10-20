//go:build !windows

package main

func installerPanelPortOwner(port int) (int, string) { return 0, "" }
