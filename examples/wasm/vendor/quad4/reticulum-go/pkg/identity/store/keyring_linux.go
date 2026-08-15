// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"golang.org/x/sys/unix"
	"quad4/reticulum-go/pkg/securemem"
)

// KeyringBackend stores secrets in the Linux user keyring (no D-Bus).
type KeyringBackend struct{}

func NewKeyringBackend() (*KeyringBackend, error) {
	if _, err := unix.KeyctlGetKeyringID(unix.KEY_SPEC_USER_KEYRING, true); err != nil {
		return nil, fmt.Errorf("%w: user keyring: %v", ErrLocked, err)
	}
	b := &KeyringBackend{}
	b.ensurePersistent()
	return b, nil
}

func (KeyringBackend) ensurePersistent() {
	uid := unix.Getuid()
	_, _ = unix.KeyctlInt(unix.KEYCTL_GET_PERSISTENT, uid, unix.KEY_SPEC_USER_KEYRING, 0, 0)
}

func keyDesc(attrs map[string]string) string {
	path := attrs[AttrIdentityPath]
	sum := sha256.Sum256([]byte(path))
	return "reticulum-go:id:" + hex.EncodeToString(sum[:8])
}

func (b KeyringBackend) find(attrs map[string]string) (int, error) {
	desc := keyDesc(attrs)
	id, err := unix.KeyctlSearch(unix.KEY_SPEC_USER_KEYRING, "user", desc, 0)
	if err != nil {
		return 0, ErrNotFound
	}
	return id, nil
}

func (b KeyringBackend) Get(attrs map[string]string) ([]byte, error) {
	id, err := b.find(attrs)
	if err != nil {
		return nil, err
	}
	n, err := unix.KeyctlBuffer(unix.KEYCTL_READ, id, nil, 0)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, n)
	n, err = unix.KeyctlBuffer(unix.KEYCTL_READ, id, buf, 0)
	if err != nil {
		return nil, err
	}
	out := append([]byte(nil), buf[:n]...)
	securemem.WipeBytes(buf)
	return out, nil
}

func (b KeyringBackend) Set(attrs map[string]string, secret []byte, label string) error {
	_ = label
	desc := keyDesc(attrs)
	if id, err := b.find(attrs); err == nil {
		_, _ = unix.KeyctlInt(unix.KEYCTL_INVALIDATE, id, 0, 0, 0)
	}
	_, err := unix.AddKey("user", desc, secret, unix.KEY_SPEC_USER_KEYRING)
	if err != nil {
		return fmt.Errorf("keyring add_key: %w", err)
	}
	b.ensurePersistent()
	return nil
}

func (b KeyringBackend) Delete(attrs map[string]string) error {
	id, err := b.find(attrs)
	if err != nil {
		return err
	}
	_, err = unix.KeyctlInt(unix.KEYCTL_INVALIDATE, id, 0, 0, 0)
	return err
}
