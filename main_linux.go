//go:build linux

package main

import (
	"fmt"
)

var version = "unset"

func main() {
	logChannel = make(chan string, 100)
	fmt.Println("OnnxTracker Version:", version)
	enforceSingleInstance()
	serverMain()
}
