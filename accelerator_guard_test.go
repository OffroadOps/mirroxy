package main

import "testing"

func TestAcceleratorGuardDefaultsAndToggle(t *testing.T) {
	a := &App{cfg: Settings{}}
	if !a.acceleratorGuardEnabled() {
		t.Fatal("the accelerator guard must default to enabled")
	}
	disabled := false
	a.cfg.PauseSystemProxyOnAccelerator = &disabled
	if a.acceleratorGuardEnabled() {
		t.Fatal("the guard must respect an explicit disable")
	}
	enabled := true
	a.cfg.PauseSystemProxyOnAccelerator = &enabled
	if !a.acceleratorGuardEnabled() {
		t.Fatal("an explicit enable must keep the guard on")
	}
}
