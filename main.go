//go:build !linux

package main

import (
	"flag"
	"fmt"
	"os"
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

var gui GUI

func main() {
	runtime.LockOSThread()
	flag.Parse()

	// Initialize log channel first so logging works during init
	logChannel = make(chan string, 100)

	// Create directories if they don't exist
	for _, dir := range []string{"config", "static"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			showErrorDialog(fmt.Sprintf("Failed to create directory %s: %v", dir, err))
			return
		}
	}

	// Check for required files before starting
	requiredFiles := []string{
		"model.onnx",
		"onnxruntime.dll",
		"static/index.html",
	}
	for _, file := range requiredFiles {
		if _, err := os.Stat(file); err != nil {
			msg := fmt.Sprintf("Missing required file: %s", file)
			showErrorDialog(msg)
			return
		}
	}

	enforceSingleInstance()
	Log("OnnxTracker Version: %s", version)

	// Initialize inference once
	if inference == nil {
		inference = &Inference{}
		if err := inference.init(); err != nil {
			msg := fmt.Sprintf("Failed to initialize inference: %v", err)
			Log(msg)
			fmt.Fprintf(os.Stderr, "%s\n", msg)
			showErrorDialog(msg)
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

	// Create platform-specific GUI
	gui = NewGUI()

	// Start server before GUI so it's always running on launch
	if err := StartWebServer(logChannel); err != nil {
		msg := fmt.Sprintf("Failed to start server: %v", err)
		Log(msg)
		fmt.Fprintf(os.Stderr, "%s\n", msg)
		showErrorDialog(msg)
	}

	// Run platform-specific GUI (blocks)
	if err := gui.Run(logChannel); err != nil {
		msg := fmt.Sprintf("GUI error: %v", err)
		Log(msg)
		fmt.Fprintf(os.Stderr, "%s\n", msg)
		showErrorDialog(msg)
	}
}
