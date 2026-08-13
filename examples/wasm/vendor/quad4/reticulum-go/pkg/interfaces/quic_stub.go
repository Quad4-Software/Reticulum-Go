// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build js

package interfaces

import "fmt"

// QUICClientOptions holds optional TLS settings for a QUIC client.
type QUICClientOptions struct {
	CertFile string
	KeyFile  string
	PeerKey  string
	SNI      string
}

// QUICServerOptions holds optional TLS settings for a QUIC server.
type QUICServerOptions struct {
	CertFile string
	KeyFile  string
	PeerKey  string
}

// QUICClientInterface is a placeholder type for js/wasm builds.
type QUICClientInterface struct {
	BaseInterface
}

// QUICServerInterface is a placeholder type for js/wasm builds.
type QUICServerInterface struct {
	BaseInterface
}

// NewQUICClientInterface returns an error on js/wasm.
func NewQUICClientInterface(name, targetHost string, targetPort int, enabled bool, opts QUICClientOptions) (*QUICClientInterface, error) {
	return nil, fmt.Errorf("QUICClientInterface is not supported on js/wasm")
}

// NewQUICClientInterfaceWithRetries returns an error on js/wasm.
func NewQUICClientInterfaceWithRetries(name, targetHost string, targetPort int, enabled bool, maxReconnectTries int, opts QUICClientOptions) (*QUICClientInterface, error) {
	return nil, fmt.Errorf("QUICClientInterface is not supported on js/wasm")
}

// NewQUICServerInterface returns an error on js/wasm.
func NewQUICServerInterface(name, bindAddr string, bindPort int, opts QUICServerOptions) (*QUICServerInterface, error) {
	return nil, fmt.Errorf("QUICServerInterface is not supported on js/wasm")
}
