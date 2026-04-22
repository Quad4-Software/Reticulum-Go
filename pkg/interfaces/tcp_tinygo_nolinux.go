// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
//go:build tinygo && !linux
// +build tinygo,!linux

package interfaces

func (tc *TCPClientInterface) setTimeoutsLinux() error {
	return nil
}

func (tc *TCPClientInterface) setTimeoutsOSX() error {
	return nil
}
