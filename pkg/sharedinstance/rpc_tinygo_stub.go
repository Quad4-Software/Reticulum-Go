// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build tinygo

package sharedinstance

// RPCServer is a placeholder on TinyGo where shared-instance RPC is disabled.
type RPCServer struct{}

func (r *RPCServer) Close() error {
	return nil
}
