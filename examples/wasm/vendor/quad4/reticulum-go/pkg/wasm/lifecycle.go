// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build js && wasm
// +build js,wasm

package wasm

import (
	"encoding/hex"
	"syscall/js"
	"time"

	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/transport"
)

func OnNetworkAvailableJS(this js.Value, args []js.Value) interface{} {
	if reticulumTransport == nil {
		return js.ValueOf(map[string]interface{}{"error": "Reticulum not initialized"})
	}
	link.SetGlobalPaused(false)
	down := time.Duration(0)
	if !lastNetworkDown.IsZero() {
		down = time.Since(lastNetworkDown)
	}
	_ = ConnectWebSocket(this, args)
	refreshWASMPaths(down)
	return js.ValueOf(map[string]interface{}{"success": true})
}

func OnNetworkLostJS(this js.Value, args []js.Value) interface{} {
	if reticulumTransport == nil {
		return js.ValueOf(map[string]interface{}{"error": "Reticulum not initialized"})
	}
	lastNetworkDown = time.Now()
	link.SetGlobalPaused(true)
	_ = DisconnectWebSocket(this, args)
	return js.ValueOf(map[string]interface{}{"success": true})
}

func SetWatchedDestinationsJS(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 || args[0].Type() != js.TypeObject {
		return js.ValueOf(map[string]interface{}{"error": "destinations array required"})
	}
	arr := args[0]
	watchedDestsWasm = make(map[string][]byte)
	for i := range arr.Length() {
		h, err := hex.DecodeString(arr.Index(i).String())
		if err != nil || len(h) != 16 {
			return js.ValueOf(map[string]interface{}{"error": "invalid destination hash"})
		}
		watchedDestsWasm[hex.EncodeToString(h)] = h
	}
	return js.ValueOf(map[string]interface{}{"success": true})
}

func refreshWASMPaths(downDuration time.Duration) {
	if reticulumTransport == nil {
		return
	}
	ttl := time.Duration(transport.PathRequestTTL) * time.Second
	for _, hash := range watchedDestsWasm {
		if downDuration > ttl {
			reticulumTransport.ExpirePath(hash)
		}
		reticulumTransport.PrepareFreshPathRequest(hash)
	}
}
