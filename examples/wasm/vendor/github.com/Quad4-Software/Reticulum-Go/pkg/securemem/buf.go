// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package securemem provides fixed-size secret buffers with best-effort
// mlock and wipe on close for long-term key material. Buffers use the Go
// heap so they remain compatible with crypto APIs that register weak handles.
package securemem

import (
	"errors"
	"runtime"
	"sync"
)

// Buf holds a fixed-size secret region. Bytes() must not be retained past Close.
type Buf struct {
	mu     sync.Mutex
	data   []byte
	locked bool
	closed bool
	native bool // reserved for platform-mapped pages (unused with heap mlock)
}

// New allocates an n-byte secret buffer. On supported platforms the region is
// mlock'd when the OS allows it. Soft-fails to unlocked heap if lock is denied.
func New(n int) (*Buf, error) {
	if n <= 0 {
		return nil, errors.New("securemem: size must be positive")
	}
	data, locked, native, err := allocLocked(n)
	if err != nil {
		return nil, err
	}
	return &Buf{data: data, locked: locked, native: native}, nil
}

// Len returns the buffer capacity.
func (b *Buf) Len() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0
	}
	return len(b.data)
}

// Bytes returns the live secret region. Do not retain past Close or Wipe.
func (b *Buf) Bytes() []byte {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	return b.data
}

// Locked reports whether the OS locked the pages into RAM.
func (b *Buf) Locked() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.locked && !b.closed
}

// CopyFrom writes src into the buffer. src length must equal Len().
func (b *Buf) CopyFrom(src []byte) error {
	if b == nil {
		return errors.New("securemem: nil buffer")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return errors.New("securemem: buffer closed")
	}
	if len(src) != len(b.data) {
		return errors.New("securemem: length mismatch")
	}
	copy(b.data, src)
	return nil
}

// CopyOut returns a heap copy of the secret. Caller must WipeBytes the result.
func (b *Buf) CopyOut() []byte {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || len(b.data) == 0 {
		return nil
	}
	out := make([]byte, len(b.data))
	copy(out, b.data)
	return out
}

// Wipe overwrites the secret with zeros without releasing the region.
func (b *Buf) Wipe() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	WipeBytes(b.data)
}

// Close wipes, unlocks, and releases the buffer. Idempotent.
func (b *Buf) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	WipeBytes(b.data)
	err := freeLocked(b.data, b.locked, b.native)
	b.data = nil
	b.locked = false
	b.native = false
	b.closed = true
	runtime.KeepAlive(b)
	return err
}
