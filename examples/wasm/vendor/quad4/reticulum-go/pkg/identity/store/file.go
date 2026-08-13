// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package store

import (
	"fmt"
	"os"
	"sync"

	"quad4/reticulum-go/internal/storage"
	"quad4/reticulum-go/pkg/securemem"
)

// FileBackend stores secrets as ordinary files at AttrIdentityPath.
type FileBackend struct{}

func (FileBackend) Get(attrs map[string]string) ([]byte, error) {
	path := attrs[AttrIdentityPath]
	if path == "" {
		return nil, fmt.Errorf("identity store: missing %s", AttrIdentityPath)
	}
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if IsMarkerPayload(data) {
		return nil, fmt.Errorf("identity store: file backend cannot resolve RSSI marker")
	}
	return data, nil
}

func (FileBackend) Set(attrs map[string]string, secret []byte, label string) error {
	_ = label
	path := attrs[AttrIdentityPath]
	if path == "" {
		return fmt.Errorf("identity store: missing %s", AttrIdentityPath)
	}
	return storage.AtomicWriteFile(path, secret, 0o600)
}

func (FileBackend) Delete(attrs map[string]string) error {
	path := attrs[AttrIdentityPath]
	if path == "" {
		return fmt.Errorf("identity store: missing %s", AttrIdentityPath)
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return ErrNotFound
	}
	return err
}

// MemoryBackend is an in-process Backend for tests.
type MemoryBackend struct {
	mu   sync.Mutex
	data map[string][]byte
}

func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{data: make(map[string][]byte)}
}

func memKey(attrs map[string]string) string {
	return attrs[AttrIdentityPath]
}

func (m *MemoryBackend) Get(attrs map[string]string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := memKey(attrs)
	v, ok := m.data[k]
	if !ok {
		return nil, ErrNotFound
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}

func (m *MemoryBackend) Set(attrs map[string]string, secret []byte, label string) error {
	_ = label
	m.mu.Lock()
	defer m.mu.Unlock()
	k := memKey(attrs)
	cp := make([]byte, len(secret))
	copy(cp, secret)
	if old, ok := m.data[k]; ok {
		securemem.WipeBytes(old)
	}
	m.data[k] = cp
	return nil
}

func (m *MemoryBackend) Delete(attrs map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := memKey(attrs)
	old, ok := m.data[k]
	if !ok {
		return ErrNotFound
	}
	securemem.WipeBytes(old)
	delete(m.data, k)
	return nil
}
