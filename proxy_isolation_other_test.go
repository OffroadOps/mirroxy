//go:build !windows

package main

import "testing"

// isolateSystemProxy is a no-op outside Windows, where the system proxy
// lookup never returns a proxy.
func isolateSystemProxy(t *testing.T) {
	t.Helper()
}
