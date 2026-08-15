// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build tinygo && !js

package interfaces

import "fmt"

type WebSocketInterface struct {
	BaseInterface
}

func NewWebSocketInterface(name string, wsURL string, enabled bool) (*WebSocketInterface, error) {
	_ = name
	_ = wsURL
	_ = enabled
	return nil, fmt.Errorf("WebSocketInterface is not supported on this TinyGo target")
}
