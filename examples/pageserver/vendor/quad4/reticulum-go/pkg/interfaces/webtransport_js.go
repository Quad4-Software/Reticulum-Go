// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build js

package interfaces

import "fmt"

// WebTransportClientOptions holds optional TLS and carriage settings for a client.
type WebTransportClientOptions struct {
	CertFile      string
	KeyFile       string
	PeerKey       string
	SNI           string
	TransportMode string
}

// WebTransportServerOptions holds optional TLS and carriage settings for a server.
type WebTransportServerOptions struct {
	CertFile      string
	KeyFile       string
	PeerKey       string
	TransportMode string
}

// WebTransportClientInterface is a placeholder type for js/wasm builds.
type WebTransportClientInterface struct {
	BaseInterface
}

// WebTransportServerInterface is a placeholder type for js/wasm builds.
type WebTransportServerInterface struct {
	BaseInterface
}

// NewWebTransportClientInterface returns an error on js/wasm.
func NewWebTransportClientInterface(name, host string, port int, path string, enabled bool, opts WebTransportClientOptions) (*WebTransportClientInterface, error) {
	return nil, fmt.Errorf("WebTransportClientInterface is not supported on js/wasm")
}

// NewWebTransportClientInterfaceWithRetries returns an error on js/wasm.
func NewWebTransportClientInterfaceWithRetries(name, host string, port int, path string, enabled bool, maxTries int, opts WebTransportClientOptions) (*WebTransportClientInterface, error) {
	return nil, fmt.Errorf("WebTransportClientInterface is not supported on js/wasm")
}

// NewWebTransportServerInterface returns an error on js/wasm.
func NewWebTransportServerInterface(name, bindAddr string, bindPort int, path string, opts WebTransportServerOptions) (*WebTransportServerInterface, error) {
	return nil, fmt.Errorf("WebTransportServerInterface is not supported on js/wasm")
}
