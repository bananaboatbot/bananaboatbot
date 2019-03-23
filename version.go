// +build !go1.12

package main

import (
	"fmt"
	"runtime"

	"github.com/bananaboatbot/bananaboatbot/resources"
)

func printVersion() {
	fmt.Println(fmt.Sprintf("%s.x.x (%s)", resources.MajorVersion, runtime.Version()))
}
