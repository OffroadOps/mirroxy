package main

import (
	"errors"
	"testing"
)

func TestAcquireInstanceLockExclusiveAndRelease(t *testing.T) {
	root := t.TempDir()
	release, err := acquireInstanceLock(root)
	if err != nil {
		t.Fatalf("first acquireInstanceLock failed: %v", err)
	}
	defer release()

	secondRelease, err := acquireInstanceLock(root)
	if !errors.Is(err, errAlreadyRunning) {
		if secondRelease != nil {
			secondRelease()
		}
		t.Fatalf("expected errAlreadyRunning for same root, got release=%v err=%v", secondRelease != nil, err)
	}

	otherRelease, err := acquireInstanceLock(t.TempDir())
	if err != nil {
		t.Fatalf("different data directory should acquire its own lock: %v", err)
	}
	otherRelease()

	release()
	// Idempotent second release must be safe.
	release()

	reacquired, err := acquireInstanceLock(root)
	if err != nil {
		t.Fatalf("acquireInstanceLock after release failed: %v", err)
	}
	reacquired()
}
