//go:build linux

package main

import (
	"fmt"
	"os"
)

var version = "unset"

func main() {
	logChannel = make(chan string, 100)
	fmt.Println("OnnxTracker Version:", version)
	for _, dir := range []string{"config", "static"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create directory %s: %v\n", dir, err)
			os.Exit(1)
		}
	}
	enforceSingleInstance()
	serverMain()
}
