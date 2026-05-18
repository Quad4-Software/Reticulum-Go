// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build js && wasm
// +build js,wasm

package main

import (
	"syscall/js"

	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/wasm"
)

func main() {
	run()
	// Keep the Go program running
	select {}
}

func run() {
	debug.Init()
	debug.SetDebugLevel(debug.DebugInfo)

	wasm.RegisterJSFunctions()

	// Notify JS that reticulum is ready
	js.Global().Call("reticulumReady")
}
