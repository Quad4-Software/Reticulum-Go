// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !linux

package store

// NewSecretServiceBackend returns ErrUnsupported off Linux.
func NewSecretServiceBackend() (*SecretServiceBackend, error) {
	return nil, ErrUnsupported
}

// SecretServiceBackend is a stub on non-Linux platforms.
type SecretServiceBackend struct{}

func (SecretServiceBackend) Get(map[string]string) ([]byte, error) {
	return nil, ErrUnsupported
}

func (SecretServiceBackend) Set(map[string]string, []byte, string) error {
	return ErrUnsupported
}

func (SecretServiceBackend) Delete(map[string]string) error {
	return ErrUnsupported
}
