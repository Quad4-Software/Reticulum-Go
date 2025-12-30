//go:build js && wasm
// +build js,wasm

package main

import (
	"syscall/js"

	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/wasm"
)

func main() {
	debug.Init()
	debug.SetDebugLevel(debug.DEBUG_INFO)

	wasm.RegisterJSFunctions()

	// Notify JS that reticulum is ready
	js.Global().Call("reticulumReady")

	// Keep the Go program running
	select {}
}

