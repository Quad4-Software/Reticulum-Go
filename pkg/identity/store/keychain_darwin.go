// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build darwin

package store

import (
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Quad4-Software/Reticulum-Go/pkg/securemem"
)

// KeychainBackend stores secrets in the macOS Keychain through the
// /usr/bin/security CLI. Items land in the stable apple-tool partition, so
// unsigned rebuilds of the binary keep access. No cgo required.
type KeychainBackend struct{}

func NewKeychainBackend() (*KeychainBackend, error) {
	if _, err := exec.LookPath("security"); err != nil {
		return nil, fmt.Errorf("%w: /usr/bin/security not found", ErrUnsupported)
	}
	return &KeychainBackend{}, nil
}

func keychainAccount(attrs map[string]string) string {
	return wrapAccount(attrs)
}

func (KeychainBackend) Get(attrs map[string]string) ([]byte, error) {
	out, err := exec.Command("/usr/bin/security", "find-generic-password",
		"-s", ApplicationName, "-a", keychainAccount(attrs), "-w").Output()
	if err != nil {
		return nil, ErrNotFound
	}
	defer securemem.WipeBytes(out)
	dec, err := hex.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		return nil, fmt.Errorf("keychain: bad stored payload: %w", err)
	}
	return dec, nil
}

func (KeychainBackend) Set(attrs map[string]string, secret []byte, label string) error {
	_ = label
	// The security CLI only accepts the item secret as a -w argument, so the
	// hex passphrase is briefly visible in the process list during Set. That
	// is the accepted tradeoff of the no-cgo CLI approach. The window is a
	// single exec and other local users are outside this backend's model.
	err := exec.Command("/usr/bin/security", "add-generic-password", "-U",
		"-s", ApplicationName, "-a", keychainAccount(attrs),
		"-w", hex.EncodeToString(secret)).Run()
	if err != nil {
		return fmt.Errorf("keychain add-generic-password: %w", err)
	}
	return nil
}

func (KeychainBackend) Delete(attrs map[string]string) error {
	if attrs[AttrIdentityPath] == "" {
		return ErrNotFound
	}
	err := exec.Command("/usr/bin/security", "delete-generic-password",
		"-s", ApplicationName, "-a", keychainAccount(attrs)).Run()
	if err != nil {
		return ErrNotFound
	}
	return nil
}
