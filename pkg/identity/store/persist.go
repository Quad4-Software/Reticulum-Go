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
)

var (
	backendMu     sync.RWMutex
	activeName            = BackendFile
	activeBackend Backend = FileBackend{}
	ssFactory             = func() (Backend, error) {
		return NewSecretServiceBackend()
	}
)

// SetBackendName selects file or secretservice as the process default.
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

// SaveIdentityBlob persists secret using the active backend.
// For secretservice, writes an RSSI marker at path and stores bytes in the keyring.
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

	switch name {
	case BackendSecretService:
		if err := b.Set(attrs, secret, ""); err != nil {
			return err
		}
		return WriteMarkerFile(path)
	default:
		return b.Set(attrs, secret, "")
	}
}

// LoadIdentityBlob loads identity bytes from path.
// RSSI markers resolve through Secret Service (or the active secretservice backend).
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
	if name == BackendSecretService {
		return b.Get(attrs)
	}
	ss, err := ssFactory()
	if err != nil {
		return nil, err
	}
	return ss.Get(attrs)
}

// MigrateToSecretService moves a plaintext identity file into Secret Service.
func MigrateToSecretService(path, kind string) error {
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
	if err := SetBackendName(BackendSecretService); err != nil {
		return err
	}
	err = SaveIdentityBlob(path, data, kind)
	if err != nil {
		SetActiveBackend(prevName, prev)
		return err
	}
	return nil
}

// MigrateToFile exports a secretservice-backed identity back to a 64/72-byte file.
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
	ss, err := ssFactory()
	if err != nil {
		return err
	}
	_ = ss.Delete(AttrsForPath(abs, ""))
	fb := FileBackend{}
	return fb.Set(AttrsForPath(abs, ""), data, "")
}
