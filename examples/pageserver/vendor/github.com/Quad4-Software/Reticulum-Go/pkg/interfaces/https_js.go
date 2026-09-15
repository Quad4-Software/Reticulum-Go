// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build js

package interfaces

import (
	"fmt"
	"time"
)

// HTTPSClientOptions holds optional TLS and long-poll settings for a client.
type HTTPSClientOptions struct {
	CertFile string
	KeyFile  string
	PeerKey  string
	SNI      string
	Path     string
	LongPoll time.Duration
}

// HTTPSServerOptions holds optional TLS and long-poll settings for a server.
type HTTPSServerOptions struct {
	CertFile string
	KeyFile  string
	PeerKey  string
	Path     string
	LongPoll time.Duration
}

// HTTPSClientInterface is a placeholder type for js/wasm builds.
type HTTPSClientInterface struct {
	BaseInterface
}

// HTTPSServerInterface is a placeholder type for js/wasm builds.
type HTTPSServerInterface struct {
	BaseInterface
}

// NewHTTPSClientInterface returns an error on js/wasm.
func NewHTTPSClientInterface(name, host string, port int, enabled bool, opts HTTPSClientOptions) (*HTTPSClientInterface, error) {
	return nil, fmt.Errorf("HTTPSClientInterface is not supported on js/wasm")
}

// NewHTTPSClientInterfaceWithRetries returns an error on js/wasm.
func NewHTTPSClientInterfaceWithRetries(name, host string, port int, enabled bool, maxTries int, opts HTTPSClientOptions) (*HTTPSClientInterface, error) {
	return nil, fmt.Errorf("HTTPSClientInterface is not supported on js/wasm")
}

// NewHTTPSServerInterface returns an error on js/wasm.
func NewHTTPSServerInterface(name, bindAddr string, bindPort int, opts HTTPSServerOptions) (*HTTPSServerInterface, error) {
	return nil, fmt.Errorf("HTTPSServerInterface is not supported on js/wasm")
}
