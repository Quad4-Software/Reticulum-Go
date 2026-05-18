// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build js && wasm
// +build js,wasm

package main

import (
	"syscall/js"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/wasm"
)

func TestRegisterFunctions(t *testing.T) {
	// Register functions
	wasm.RegisterJSFunctions()

	reticulum := js.Global().Get("reticulum")
	if reticulum.IsUndefined() {
		t.Fatal("reticulum object not registered")
	}

	// Manually register chat functions since main() has select{}
	reticulum.Set("sendMessage", js.FuncOf(SendMessage))
	reticulum.Set("sendAnnounce", js.FuncOf(SendAnnounce))

	tests := []string{"sendMessage", "sendAnnounce", "init", "getIdentity", "sendData"}
	for _, name := range tests {
		if reticulum.Get(name).Type() != js.TypeFunction {
			t.Errorf("function %s not registered correctly", name)
		}
	}
}
