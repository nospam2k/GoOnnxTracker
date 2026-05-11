//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
)

// lockPath is chosen to be per-user so that multiple users on the same
// machine can each run their own instance independently.
func lockPath() string {
	return fmt.Sprintf("/tmp/onnxtracker-%d.lock", os.Getuid())
}

// enforceSingleInstance opens (or creates) a lock file and attempts a
// non-blocking exclusive flock. If the lock is already held by another
// process the call fails immediately and we exit.
//
// The file descriptor is intentionally kept open for the lifetime of the
// process; the kernel releases the lock automatically on exit.
func enforceSingleInstance() {
	path := lockPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		// If we can't even open the file, warn but don't block startup.
		fmt.Fprintf(os.Stderr, "singleinstance: could not open lock file %s: %v\n", path, err)
		return
	}

	// LOCK_EX | LOCK_NB: exclusive, non-blocking.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		fmt.Fprintln(os.Stderr, "OnnxTracker is already running.")
		os.Exit(0)
	}

	// Keep f open — dropping it would release the lock.
	// Store in a package-level var so the GC never closes it.
	globalLockFile = f
}

// globalLockFile holds the open lock file for the process lifetime.
var globalLockFile *os.File
