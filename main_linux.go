//go:build linux

package main

import (
	"fmt"
)

var version = "unset"

func main() {
	fmt.Println("OnnxTracker Version:", version)
	enforceSingleInstance()
	serverMain()
}
