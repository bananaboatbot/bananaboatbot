// +build go1.12

package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

func printVersion() {
	version := fmt.Sprintf("%s.x.x", majorVersion)
	info, ok := debug.ReadBuildInfo()
	if ok {
		version = info.Main.Version
	}
	fmt.Println(fmt.Sprintf("%s (%s)", version, runtime.Version()))
}
