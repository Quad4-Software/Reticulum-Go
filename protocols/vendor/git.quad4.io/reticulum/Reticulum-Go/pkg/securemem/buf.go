// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Package securemem holds secret key material outside the Go heap.
//
// Small buffers (New) are carved from shared mmap'd arenas with PROT_NONE
// guard pages, best-effort mlock, MADV_DONTDUMP and MADV_WIPEONFORK on
// Linux, MAP_CONCEAL on OpenBSD, and per-cell canaries. NewProtected gives
// a dedicated mapping that stays PROT_NONE except inside the scoped Use
// accessor, for high-value long-term secrets. Platforms without guarded
// mapping support (js/wasm, haiku, plan9) fall back to wiped heap bytes.
//
// The Go heap and goroutine stacks are not covered: passing Bytes() or a
// CopyOut result to a crypto API copies the secret into runtime-managed
// memory this package cannot wipe. See docs/en/security.md.
package securemem

import (
	"crypto/rand"
	"errors"
	"runtime"
	"sync"
)

// ErrCanary reports that a buffer's canary bytes were overwritten, meaning
// memory was written past the buffer bounds while it was allocated.
var ErrCanary = errors.New("securemem: canary check failed")

// Buf holds a fixed-size secret region. Bytes() must not be retained past
// Close.
type Buf struct {
	mu        sync.Mutex
	cell      *cellRef // slab cell, when class-sized
	class     int      // cell payload class, when slab-allocated
	mapping   []byte   // dedicated mapping incl. guards, when large/protected
	usable    []byte   // usable region of the dedicated mapping
	data      []byte   // payload slice, length as requested
	locked    bool
	protected bool // PROT_NONE between scoped accessors
	closed    bool
	native    bool // off-heap mapping (false on heap fallback targets)
	canB      [canarySize]byte
	canOff    int // payload offset inside usable for dedicated regions
}

// New allocates an n-byte secret buffer in a slab cell (or a dedicated
// mapping when n exceeds the largest size class). On supported platforms the
// region is mlock'd when the OS allows it. Soft-fails to unlocked memory.
func New(n int) (*Buf, error) {
	return newBuf(n, false)
}

// NewProtected allocates an n-byte secret buffer in a dedicated mapping that
// is PROT_NONE except inside Use, CopyFrom, CopyOut and Wipe. Bytes returns
// nil for protected buffers. Access the secret through Use.
func NewProtected(n int) (*Buf, error) {
	return newBuf(n, true)
}

func newBuf(n int, protected bool) (*Buf, error) {
	if n <= 0 {
		return nil, errors.New("securemem: size must be positive")
	}
	b := &Buf{protected: protected}
	class := classFor(n)
	switch {
	case !protected && class >= 0:
		ref, payload, err := allocCell(class)
		if err != nil {
			return nil, err
		}
		b.cell = ref
		b.class = class
		b.data = payload[:n]
		b.locked = ref.ar.locked
	case class >= 0:
		// Small protected buffers still get a dedicated mapping so the idle
		// PROT_NONE window does not expose neighbouring cells.
		fallthrough
	default:
		mapping, usable, err := mapSecret(n + canarySize)
		if err != nil {
			return nil, err
		}
		if err := lockRange(usable); err != nil {
			debugLockDenied(err)
		} else {
			b.locked = true
		}
		_, _ = rand.Read(b.canB[:])
		copy(usable[n:], b.canB[:])
		b.mapping = mapping
		b.usable = usable
		b.data = usable[:n]
		b.canOff = n
		if protected {
			if err := protectRange(usable, false); err != nil {
				unmapSecret(mapping)
				return nil, err
			}
		}
	}
	b.native = !isHeapFallback
	runtime.SetFinalizer(b, (*Buf).finalize)
	return b, nil
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
// Protected buffers keep their pages PROT_NONE while idle, so Bytes returns
// nil for them. Use Use instead.
func (b *Buf) Bytes() []byte {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.protected {
		return nil
	}
	return b.data
}

// Use runs fn with the secret readable (and writable) inside the callback.
// For protected buffers the pages are made read-write for the duration and
// returned to PROT_NONE afterwards. fn must not retain the slice.
func (b *Buf) Use(fn func(secret []byte)) error {
	if b == nil {
		return errors.New("securemem: nil buffer")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return errors.New("securemem: buffer closed")
	}
	if err := b.unprotectLocked(); err != nil {
		return err
	}
	defer b.reprotectLocked()
	if err := b.checkCanaryLocked(); err != nil {
		return err
	}
	fn(b.data)
	return nil
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

// Native reports whether the secret lives in an off-heap mapping rather
// than Go-heap fallback memory.
func (b *Buf) Native() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.native && !b.closed
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
	if err := b.unprotectLocked(); err != nil {
		return err
	}
	defer b.reprotectLocked()
	if err := b.checkCanaryLocked(); err != nil {
		return err
	}
	copy(b.data, src)
	return nil
}

// CopyOut returns a heap copy of the secret. Caller must WipeBytes the
// result.
func (b *Buf) CopyOut() []byte {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || len(b.data) == 0 {
		return nil
	}
	if err := b.unprotectLocked(); err != nil {
		return nil
	}
	defer b.reprotectLocked()
	if err := b.checkCanaryLocked(); err != nil {
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
	if err := b.unprotectLocked(); err != nil {
		return
	}
	defer b.reprotectLocked()
	WipeBytes(b.data)
}

// Close checks canaries, wipes, unlocks and releases the buffer. Idempotent.
// A canary failure is returned but the buffer is still wiped and released.
func (b *Buf) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	err := b.closeLocked()
	b.mu.Unlock()
	runtime.SetFinalizer(b, nil)
	runtime.KeepAlive(b)
	return err
}

func (b *Buf) closeLocked() error {
	if b.closed {
		return nil
	}
	b.closed = true
	var retErr error
	if b.cell != nil {
		retErr = freeCell(b.cell, b.class)
		b.cell = nil
	} else if len(b.usable) > 0 {
		if b.protected {
			_ = protectRange(b.usable, true)
		}
		if len(b.usable) >= b.canOff+canarySize && !equalBytes(b.usable[b.canOff:b.canOff+canarySize], b.canB[:]) {
			retErr = ErrCanary
		}
		WipeBytes(b.usable)
		if b.locked {
			unlockRange(b.usable)
		}
		unmapSecret(b.mapping)
	}
	b.data = nil
	b.mapping = nil
	b.usable = nil
	b.locked = false
	b.native = false
	return retErr
}

// finalize is the GC safety net for buffers whose owner forgot Close. It is
// not a substitute for Close: the finalizer may run long after the secret
// was last used.
func (b *Buf) finalize() {
	b.mu.Lock()
	defer b.mu.Unlock()
	_ = b.closeLocked()
}

func (b *Buf) unprotectLocked() error {
	if !b.protected || len(b.usable) == 0 {
		return nil
	}
	return protectRange(b.usable, true)
}

func (b *Buf) reprotectLocked() {
	if !b.protected || len(b.usable) == 0 {
		return
	}
	_ = protectRange(b.usable, false)
}

func (b *Buf) checkCanaryLocked() error {
	if b.cell != nil {
		ar := b.cell.ar
		off := b.cell.idx * ar.stride
		cell := ar.data[off : off+ar.stride]
		if !equalBytes(cell[:canarySize], b.cell.canA[:]) ||
			!equalBytes(cell[canarySize+b.class:], b.cell.canB[:]) {
			return ErrCanary
		}
		return nil
	}
	if len(b.usable) >= b.canOff+canarySize {
		if !equalBytes(b.usable[b.canOff:b.canOff+canarySize], b.canB[:]) {
			return ErrCanary
		}
	}
	return nil
}
