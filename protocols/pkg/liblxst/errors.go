// SPDX-License-Identifier: LicenseRef-Reticulum
package liblxst

import (
	"errors"
	"sync"
	"sync/atomic"
)

const (
	OK               = 0
	ErrInvalidArg    = 1
	ErrInvalidHandle = 2
	ErrInternal      = 6
	ErrTruncated     = 8
)

var (
	errInvalidHandle = errors.New("invalid handle")
	errInvalidArg    = errors.New("invalid argument")
	lastErrMu        sync.RWMutex
	lastErr          string
)

type handleKind uint8

const (
	kindPacket handleKind = iota + 1
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
	return &handleTable{next: 1, entries: make(map[uint64]handleEntry)}
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

var handles = newHandleTable()

func setLastError(err error) int {
	if err == nil {
		lastErrMu.Lock()
		lastErr = ""
		lastErrMu.Unlock()
		return OK
	}
	lastErrMu.Lock()
	lastErr = err.Error()
	lastErrMu.Unlock()
	switch {
	case errors.Is(err, errInvalidHandle):
		return ErrInvalidHandle
	case errors.Is(err, errInvalidArg):
		return ErrInvalidArg
	default:
		return ErrInternal
	}
}

func LastError() string {
	lastErrMu.RLock()
	defer lastErrMu.RUnlock()
	return lastErr
}
