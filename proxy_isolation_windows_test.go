//go:build windows

package main

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// isolateSystemProxy points the system proxy lookup at a disposable registry
// key. A test that downloads from a local server must not be routed through
// the proxy that happens to be configured on the machine.
func isolateSystemProxy(t *testing.T) {
	t.Helper()
	path := `Software\Mirroxy-tests\` + newID()
	k, _, e := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if e != nil {
		t.Fatal(e)
	}
	k.Close()
	before := proxySettingsPath
	proxySettingsPath = path
	t.Cleanup(func() {
		proxySettingsPath = before
		registry.DeleteKey(registry.CURRENT_USER, path)
	})
}
