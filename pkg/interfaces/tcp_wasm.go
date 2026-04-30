// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build js && wasm

package interfaces

func (tc *TCPClientInterface) setTimeoutsLinux() error {
	return nil
}

func (tc *TCPClientInterface) setTimeoutsOSX() error {
	return nil
}
