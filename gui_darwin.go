//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>

void runCocoaApp(void);
void appendTextToLog(const char* text);
*/
import "C"
import (
	"unsafe"
)

type macOSGUI struct{}

func NewGUI() GUI {
	return &macOSGUI{}
}

func (g *macOSGUI) Run(logChan chan string) error {
    // Store logChan so onCocoaAppReady can drain it once the Cocoa
    // event loop is spinning. We can't start the goroutine here because
    // the run loop hasn't started yet.
	logChannel = logChan
	C.runCocoaApp()
	return nil
}

func (g *macOSGUI) AppendLog(text string) {
	cText := C.CString(text)
	C.appendTextToLog(cText)
	C.free(unsafe.Pointer(cText))
}

func (g *macOSGUI) SetButtonText(button int, text string) {
	// No-op: log-only window has no button
}

func (g *macOSGUI) Cleanup() {
	// No manual cleanup needed — Cocoa objects are managed by ARC.
}

//export onCocoaAppReady
func onCocoaAppReady() {
    go func() {
        for msg := range logChannel {
            gui.AppendLog(msg + "\n")
        }
    }()
}
