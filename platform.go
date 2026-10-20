package main

import "runtime"

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
func serverPlatform() bool { return runtime.GOOS == "linux" }
func platformState() M {
	return M{"os": runtime.GOOS, "arch": runtime.GOARCH, "server": serverPlatform(), "systemProxy": runtime.GOOS == "windows", "tray": runtime.GOOS == "windows", "desktopAutostart": runtime.GOOS == "windows"}
}
