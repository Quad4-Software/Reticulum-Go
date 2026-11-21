// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build windows

package store

import (
	"crypto/sha256"
	"fmt"
	"os"
	"unsafe"

	"github.com/Quad4-Software/Reticulum-Go/internal/storage"
	"golang.org/x/sys/windows"
)

// DPAPIBackend protects secrets with the current user's DPAPI master key and
// stores the protected blob in a sidecar file next to the identity. Bound to
// the Windows user account: no passphrase prompt, decryptable only by the
// same user on the same machine (or with the roaming profile credential).
type DPAPIBackend struct{}

func NewDPAPIBackend() (*DPAPIBackend, error) {
	return &DPAPIBackend{}, nil
}

// dpapiEntropy binds a protected blob to the identity path it belongs to, so
// a .dpwrap sidecar copied next to a different identity file does not
// unprotect for it.
func dpapiEntropy(path string) *windows.DataBlob {
	sum := sha256.Sum256([]byte("reticulum-go-dpwrap:" + path))
	return &windows.DataBlob{Data: &sum[0], Size: uint32(len(sum))}
}

func dpapiProtect(plain []byte, entropy *windows.DataBlob) ([]byte, error) {
	if len(plain) == 0 {
		return nil, fmt.Errorf("dpapi protect: empty input")
	}
	in := windows.DataBlob{Data: &plain[0], Size: uint32(len(plain))}
	var out windows.DataBlob
	desc, err := windows.UTF16PtrFromString("Reticulum-Go identity passphrase")
	if err != nil {
		return nil, err
	}
	if err := windows.CryptProtectData(&in, desc, entropy, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("dpapi protect: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	blob := make([]byte, out.Size)
	copy(blob, unsafe.Slice(out.Data, out.Size))
	return blob, nil
}

func dpapiUnprotect(blob []byte, entropy *windows.DataBlob) ([]byte, error) {
	if len(blob) == 0 {
		return nil, ErrNotFound
	}
	in := windows.DataBlob{Data: &blob[0], Size: uint32(len(blob))}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, entropy, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("dpapi unprotect: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	plain := make([]byte, out.Size)
	copy(plain, unsafe.Slice(out.Data, out.Size))
	return plain, nil
}

func (DPAPIBackend) Get(attrs map[string]string) ([]byte, error) {
	path := attrs[AttrIdentityPath]
	if path == "" {
		return nil, ErrNotFound
	}
	blob, err := os.ReadFile(sidecarPath(path)) // #nosec G304
	if err != nil {
		return nil, ErrNotFound
	}
	return dpapiUnprotect(blob, dpapiEntropy(path))
}

func (DPAPIBackend) Set(attrs map[string]string, secret []byte, label string) error {
	_ = label
	path := attrs[AttrIdentityPath]
	if path == "" {
		return fmt.Errorf("dpapi backend: identity path attribute required")
	}
	blob, err := dpapiProtect(secret, dpapiEntropy(path))
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(sidecarPath(path), blob, 0o600)
}

func (DPAPIBackend) Delete(attrs map[string]string) error {
	path := attrs[AttrIdentityPath]
	if path == "" {
		return ErrNotFound
	}
	err := os.Remove(sidecarPath(path))
	if err != nil {
		return ErrNotFound
	}
	return nil
}
