// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !linux

package store

// NewKeyringBackend returns ErrUnsupported off Linux.
func NewKeyringBackend() (*KeyringBackend, error) {
	return nil, ErrUnsupported
}

// KeyringBackend is a stub on non-Linux platforms.
type KeyringBackend struct{}

func (KeyringBackend) Get(map[string]string) ([]byte, error) {
	return nil, ErrUnsupported
}

func (KeyringBackend) Set(map[string]string, []byte, string) error {
	return ErrUnsupported
}

func (KeyringBackend) Delete(map[string]string) error {
	return ErrUnsupported
}
