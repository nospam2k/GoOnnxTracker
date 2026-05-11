//go:build !linux

package main

import (
	"flag"
	"fmt"
	"runtime"
)

var version = "unset"

// GUI interface - implemented per-platform
type GUI interface {
	Run(logChan chan string) error
	AppendLog(text string)
	SetButtonText(button int, text string)
	Cleanup()
}

var (
	gui        GUI
	logChannel chan string
)

// Log sends a message to the GUI log window
func Log(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	select {
	case logChannel <- msg:
	default:
		// Channel full, discard to avoid blocking
	}
}

func main() {
	runtime.LockOSThread()
	flag.Parse()

	enforceSingleInstance()
	Log("OnnxTracker Version: %s", version)

	// Initialize inference once
	if inference == nil {
		inference = &Inference{}
		if err := inference.init(); err != nil {
			Log("Failed to initialize inference: %v", err)
			return
		}
	}

	// Initialize hub and tracker
	if hub == nil {
		hub = NewHub()
	}
	if tracker == nil {
		trackingCh = make(chan []byte, 8)
		tracker = &Tracker{
			inference: inference,
			hub:       hub,
			frameCh:   trackingCh,
		}
	}

	// Initialize log channel
	logChannel = make(chan string, 100)

	// Create platform-specific GUI
	gui = NewGUI()

	// Start server before GUI so it's always running on launch
	if err := StartWebServer(logChannel); err != nil {
		Log("Failed to start server: %v", err)
	}

	// Run platform-specific GUI (blocks)
	if err := gui.Run(logChannel); err != nil {
		Log("GUI error: %v", err)
	}
}
