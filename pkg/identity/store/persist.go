// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package store

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"quad4/reticulum-go/pkg/securemem"
)

const (
	BackendFile          = "file"
	BackendSecretService = "secretservice"
	BackendKeyring       = "keyring"
)

var (
	backendMu     sync.RWMutex
	activeName            = BackendFile
	activeBackend Backend = FileBackend{}
	ssFactory             = func() (Backend, error) {
		return NewSecretServiceBackend()
	}
	keyringFactory = func() (Backend, error) {
		return NewKeyringBackend()
	}
)

// SetBackendName selects file, secretservice, or keyring as the process default.
func SetBackendName(name string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = BackendFile
	}
	var b Backend
	switch name {
	case BackendFile:
		b = FileBackend{}
	case BackendSecretService:
		ss, err := ssFactory()
		if err != nil {
			backendMu.Lock()
			activeName = BackendSecretService
			activeBackend = failingBackend{err: err}
			backendMu.Unlock()
			return err
		}
		b = ss
	case BackendKeyring:
		kr, err := keyringFactory()
		if err != nil {
			backendMu.Lock()
			activeName = BackendKeyring
			activeBackend = failingBackend{err: err}
			backendMu.Unlock()
			return err
		}
		b = kr
	default:
		return fmt.Errorf("identity store: unknown backend %q", name)
	}
	backendMu.Lock()
	activeName = name
	activeBackend = b
	backendMu.Unlock()
	return nil
}

type failingBackend struct{ err error }

func (f failingBackend) Get(map[string]string) ([]byte, error) { return nil, f.err }
func (f failingBackend) Set(map[string]string, []byte, string) error {
	return f.err
}
func (f failingBackend) Delete(map[string]string) error { return f.err }

// BackendName returns the configured backend name.
func BackendName() string {
	backendMu.RLock()
	defer backendMu.RUnlock()
	return activeName
}

// Active returns the current Backend.
func Active() Backend {
	backendMu.RLock()
	defer backendMu.RUnlock()
	return activeBackend
}

// SetActiveBackend installs a Backend for tests (e.g. MemoryBackend).
func SetActiveBackend(name string, b Backend) {
	backendMu.Lock()
	activeName = name
	activeBackend = b
	backendMu.Unlock()
}

func usesMarker(name string) bool {
	return name == BackendSecretService || name == BackendKeyring
}

// SaveIdentityBlob persists secret using the active backend.
// For secretservice and keyring, writes an RSSI marker at path and stores bytes off-disk.
func SaveIdentityBlob(path string, secret []byte, kind string) error {
	abs, err := AbsolutePath(path)
	if err != nil {
		return err
	}
	attrs := AttrsForPath(abs, kind)
	backendMu.RLock()
	name := activeName
	b := activeBackend
	backendMu.RUnlock()

	if usesMarker(name) {
		if err := b.Set(attrs, secret, ""); err != nil {
			return err
		}
		return WriteMarkerFile(path)
	}
	return b.Set(attrs, secret, "")
}

// LoadIdentityBlob loads identity bytes from path.
// RSSI markers resolve through the active marker backend or by probing keyring then secretservice.
func LoadIdentityBlob(path string) ([]byte, error) {
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return nil, err
	}
	if !IsMarkerPayload(data) {
		return data, nil
	}
	if data[5] != 0 || data[6] != 0 || data[7] != 0 {
		return nil, errorsReserved()
	}
	abs, err := AbsolutePath(path)
	if err != nil {
		return nil, err
	}
	attrs := AttrsForPath(abs, "")

	backendMu.RLock()
	name := activeName
	b := activeBackend
	backendMu.RUnlock()
	if usesMarker(name) {
		return b.Get(attrs)
	}

	if kr, err := keyringFactory(); err == nil {
		if secret, err := kr.Get(attrs); err == nil {
			return secret, nil
		}
	}
	ss, err := ssFactory()
	if err != nil {
		return nil, err
	}
	return ss.Get(attrs)
}

// MigrateToBackend moves a plaintext identity file into the named marker backend.
func MigrateToBackend(path, kind, backendName string) error {
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return err
	}
	if IsMarkerPayload(data) {
		return nil
	}
	defer securemem.WipeBytes(data)
	prevName := BackendName()
	prev := Active()
	if err := SetBackendName(backendName); err != nil {
		return err
	}
	if err := SaveIdentityBlob(path, data, kind); err != nil {
		SetActiveBackend(prevName, prev)
		return err
	}
	return nil
}

// MigrateToSecretService moves a plaintext identity file into Secret Service.
func MigrateToSecretService(path, kind string) error {
	return MigrateToBackend(path, kind, BackendSecretService)
}

// MigrateToKeyring moves a plaintext identity file into the kernel keyring.
func MigrateToKeyring(path, kind string) error {
	return MigrateToBackend(path, kind, BackendKeyring)
}

// MigrateToFile exports a marker-backed identity back to a plaintext file.
func MigrateToFile(path string) error {
	data, err := LoadIdentityBlob(path)
	if err != nil {
		return err
	}
	defer securemem.WipeBytes(data)
	abs, err := AbsolutePath(path)
	if err != nil {
		return err
	}
	attrs := AttrsForPath(abs, "")
	backendMu.RLock()
	name := activeName
	b := activeBackend
	backendMu.RUnlock()
	if usesMarker(name) {
		_ = b.Delete(attrs)
	} else {
		if kr, err := keyringFactory(); err == nil {
			_ = kr.Delete(attrs)
		}
		if ss, err := ssFactory(); err == nil {
			_ = ss.Delete(attrs)
		}
	}
	fb := FileBackend{}
	return fb.Set(attrs, data, "")
}
