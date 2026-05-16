//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// mutexName must be unique to this application. Prefix with "Local\" so it
// works inside a user session even without admin rights.
const mutexName = `Local\OnnxTrackerControlPanel`

// enforceSingleInstance creates a named Win32 mutex. If a second instance
// tries to start, CreateMutexW succeeds but GetLastError returns
// ERROR_ALREADY_EXISTS, which is our signal to exit.
//
// The mutex handle is intentionally leaked — it lives for the full process
// lifetime and is released automatically when the process exits.
func enforceSingleInstance() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	createMutex := kernel32.NewProc("CreateMutexW")

	name, err := syscall.UTF16PtrFromString(mutexName)
	if err != nil {
		msg := fmt.Sprintf("singleinstance: UTF16 conversion failed: %v", err)
		fmt.Fprintf(os.Stderr, "%s\n", msg)
		os.Exit(1)
	}

	// CreateMutexW(lpMutexAttributes, bInitialOwner, lpName)
	_, _, lastErr := createMutex.Call(
		0,                        // default security attributes
		1,                        // claim initial ownership
		uintptr(unsafe.Pointer(name)),
	)

	// syscall wraps GetLastError; ERROR_ALREADY_EXISTS == 183
	const errorAlreadyExists = syscall.Errno(183)
	if lastErr == errorAlreadyExists {
		fmt.Fprintln(os.Stderr, "OnnxTracker is already running.")
		// Optionally bring the existing window to the foreground.
		bringExistingWindowToFront()
		os.Exit(0)
	}
}

// bringExistingWindowToFront finds the first top-level window with our class
// name and flashes / restores it so the user can see the existing instance.
func bringExistingWindowToFront() {
	user32 := syscall.NewLazyDLL("user32.dll")
	findWindow := user32.NewProc("FindWindowW")
	showWindow := user32.NewProc("ShowWindow")
	setForeground := user32.NewProc("SetForegroundWindow")

	className, _ := syscall.UTF16PtrFromString("OnnxTrackerClass")
	hwnd, _, _ := findWindow.Call(uintptr(unsafe.Pointer(className)), 0)
	if hwnd == 0 {
		return
	}

	const swRestore = 9
	showWindow.Call(hwnd, swRestore)
	setForeground.Call(hwnd)
}
