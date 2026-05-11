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

func main() {
	runtime.LockOSThread()
	flag.Parse()

	enforceSingleInstance()
	fmt.Println("OnnxTracker Version:", version)

	// Initialize inference once
	if inference == nil {
		inference = &Inference{}
		if err := inference.init(); err != nil {
			fmt.Printf("Failed to initialize inference: %v\n", err)
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
		fmt.Printf("Failed to start server: %v\n", err)
	}

	// Run platform-specific GUI (blocks)
	if err := gui.Run(logChannel); err != nil {
		fmt.Printf("GUI error: %v\n", err)
	}
}
