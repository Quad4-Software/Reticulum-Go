//go:build js && wasm
// +build js,wasm

package main

import (
	"syscall/js"
	"testing"
)

func TestRun(t *testing.T) {
	readyCalled := false
	js.Global().Set("reticulumReady", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		readyCalled = true
		return nil
	}))

	run()

	if !readyCalled {
		t.Error("reticulumReady was not called by run()")
	}

	reticulum := js.Global().Get("reticulum")
	if reticulum.IsUndefined() {
		t.Error("reticulum functions were not registered")
	}
}

