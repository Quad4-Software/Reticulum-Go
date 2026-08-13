// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build js && wasm
// +build js,wasm

package main

import (
	"encoding/hex"
	"fmt"
	"syscall/js"

	"quad4/reticulum-go-protocols/pkg/mf"
	"quad4/reticulum-go/pkg/wasm"
)

var messenger *mf.Messenger

func main() {
	// Register the generic WASM bridge functions first
	wasm.RegisterJSFunctions()

	// Add chat-specific functions to the "reticulum" JS object
	reticulum := js.Global().Get("reticulum")
	reticulum.Set("sendMessage", js.FuncOf(SendMessage))
	reticulum.Set("sendAnnounce", js.FuncOf(SendAnnounce))

	// Keep the Go program running
	select {}
}

func SendMessage(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return js.ValueOf(map[string]interface{}{
			"error": "Destination hash and message required",
		})
	}

	destHashHex := args[0].String()
	message := args[1].String()

	destHash, err := hex.DecodeString(destHashHex)
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Invalid destination hash: %v", err),
		})
	}

	// Initialize messenger if not already done
	if messenger == nil {
		t := wasm.GetTransport()
		d := wasm.GetDestinationPointer()
		if t == nil || d == nil {
			return js.ValueOf(map[string]interface{}{
				"error": "Reticulum not initialized",
			})
		}
		messenger = mf.NewMessenger(t, d)
	}

	// Use the high-level Messenger from mf package
	if err := messenger.SendMessage(destHash, message); err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": fmt.Sprintf("Send failed: %v", err),
		})
	}

	return js.ValueOf(map[string]interface{}{
		"success": true,
	})
}

func SendAnnounce(this js.Value, args []js.Value) interface{} {
	var appData []byte
	if len(args) >= 1 && args[0].String() != "" {
		appData = []byte(args[0].String())
	}

	return wasm.SendAnnounce(appData)
}
