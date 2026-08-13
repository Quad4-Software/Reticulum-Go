// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"bytes"
	"io"
	"sync"
	"time"
)

// Coalescer batches stdin writes for high-RTT links.
type Coalescer struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	lineMode bool
	window   time.Duration
	flush    func([]byte)
	timer    *time.Timer
	closed   bool
}

// NewCoalescer creates a stdin coalescer. flush is invoked with batched bytes.
// window <= 0 with lineMode false means immediate flush (raw mode).
func NewCoalescer(lineMode bool, window time.Duration, flush func([]byte)) *Coalescer {
	if lineMode && window <= 0 {
		window = DefaultCoalesceWindow
	}
	if !lineMode && window < 0 {
		window = 0
	}
	return &Coalescer{
		lineMode: lineMode,
		window:   window,
		flush:    flush,
	}
}

// PreferLineForRTT returns whether line mode should be auto-enabled.
func PreferLineForRTT(rtt time.Duration) bool {
	return rtt >= AutoLineRTT
}

// Write accepts stdin bytes.
func (c *Coalescer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, io.ErrClosedPipe
	}
	if !c.lineMode && c.window <= 0 {
		c.flushUnlocked(p)
		return len(p), nil
	}
	_, _ = c.buf.Write(p)
	if c.lineMode {
		if i := bytes.LastIndexByte(c.buf.Bytes(), '\n'); i >= 0 {
			out := append([]byte(nil), c.buf.Bytes()[:i+1]...)
			rest := append([]byte(nil), c.buf.Bytes()[i+1:]...)
			c.buf.Reset()
			_, _ = c.buf.Write(rest)
			c.flushUnlocked(out)
		}
		return len(p), nil
	}
	if c.timer == nil {
		c.timer = time.AfterFunc(c.window, c.timerFlush)
	}
	return len(p), nil
}

// Flush sends any buffered bytes.
func (c *Coalescer) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopTimerLocked()
	if c.buf.Len() > 0 {
		out := append([]byte(nil), c.buf.Bytes()...)
		c.buf.Reset()
		c.flushUnlocked(out)
	}
}

// Close flushes and marks closed.
func (c *Coalescer) Close() {
	c.Flush()
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
}

// SetLineMode switches line buffering. Flushes when disabling.
func (c *Coalescer) SetLineMode(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lineMode = on
	if !on && c.buf.Len() > 0 {
		out := append([]byte(nil), c.buf.Bytes()...)
		c.buf.Reset()
		c.flushUnlocked(out)
	}
}

func (c *Coalescer) timerFlush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timer = nil
	if c.buf.Len() == 0 {
		return
	}
	out := append([]byte(nil), c.buf.Bytes()...)
	c.buf.Reset()
	c.flushUnlocked(out)
}

func (c *Coalescer) flushUnlocked(p []byte) {
	if len(p) == 0 || c.flush == nil {
		return
	}
	c.flush(p)
}

func (c *Coalescer) stopTimerLocked() {
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}
