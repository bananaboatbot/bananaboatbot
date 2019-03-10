// +build !go1.12

package main

import (
	"fmt"
	"runtime"
)

func printVersion() {
	fmt.Println(fmt.Sprintf("(unknown) (%s)", runtime.Version()))
}
