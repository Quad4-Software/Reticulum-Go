// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
//go:build linux && tinygo
// +build linux,tinygo

package interfaces

func (tc *TCPClientInterface) setTimeoutsLinux() error {
	return nil
}

func (tc *TCPClientInterface) setTimeoutsOSX() error {
	return nil
}
