// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"sync"
	"sync/atomic"
)

type handleKind uint8

const (
	kindNode handleKind = iota + 1
	kindIdentity
	kindDestination
	kindLink
)

type handleEntry struct {
	kind handleKind
	ref  any
}

type handleTable struct {
	mu      sync.RWMutex
	next    uint64
	entries map[uint64]handleEntry
}

func newHandleTable() *handleTable {
	return &handleTable{
		next:    1,
		entries: make(map[uint64]handleEntry),
	}
}

func (t *handleTable) insert(kind handleKind, ref any) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	id := atomic.AddUint64(&t.next, 1)
	t.entries[id] = handleEntry{kind: kind, ref: ref}
	return id
}

func (t *handleTable) get(id uint64, kind handleKind) (any, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	e, ok := t.entries[id]
	if !ok || e.kind != kind {
		return nil, errInvalidHandle
	}
	return e.ref, nil
}

func (t *handleTable) delete(id uint64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.entries[id]; !ok {
		return false
	}
	delete(t.entries, id)
	return true
}

func validatePath(path string) error {
	if path == "" {
		return errInvalidArg
	}
	for i := 0; i < len(path); i++ {
		if path[i] == 0 {
			return errInvalidArg
		}
	}
	return nil
}
