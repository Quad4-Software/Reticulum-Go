// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
//go:build js && wasm
// +build js,wasm

package main

import (
	"syscall/js"

	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/wasm"
)

func main() {
	run()
	// Keep the Go program running
	select {}
}

func run() {
	debug.Init()
	debug.SetDebugLevel(debug.DEBUG_INFO)

	wasm.RegisterJSFunctions()

	// Notify JS that reticulum is ready
	js.Global().Call("reticulumReady")
}
