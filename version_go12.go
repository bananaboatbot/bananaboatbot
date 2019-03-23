// +build go1.12

package main

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/bananaboatbot/bananaboatbot/resources"
)

func printVersion() {
	var version string
	info, ok := debug.ReadBuildInfo()
	if ok {
		if info.Main.Version != "(devel)" {
			version = info.Main.Version
		} else {
			version = fmt.Sprintf("%s.x.x", resources.MajorVersion)
		}
	}
	fmt.Println(fmt.Sprintf("%s (%s)", version, runtime.Version()))
}
