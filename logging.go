package main

import "fmt"

// Log sends a message to the log channel (GUI on Windows, or discarded on Linux)
func Log(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	select {
	case logChannel <- msg:
	default:
		// Channel full, discard to avoid blocking
	}
}
