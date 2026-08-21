// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package harness provides shared helpers for live Python-Go interop tests.
package harness

import (
	"bufio"
	"context"
	"sync"
	"time"
)

type lineMsg struct {
	s   string
	err error
}

// lineWaiter ensures at most one ReadString is in flight per bufio.Reader.
// Timed-out waits leave the in-flight result buffered for the next caller,
// avoiding concurrent bufio use (which panics).
//
// reading stays true from starting ReadString until the result is consumed
// from ch. Clearing it only after send races with the next caller, which can
// see reading still true, skip starting a new read, and hang until timeout.
type lineWaiter struct {
	mu      sync.Mutex
	reading bool
	ch      chan lineMsg
}

var lineWaiters sync.Map // *bufio.Reader -> *lineWaiter

// ReadLineTimeout reads one newline-terminated line with a deadline.
func ReadLineTimeout(ctx context.Context, br *bufio.Reader, d time.Duration) (string, error) {
	if br == nil {
		return "", context.DeadlineExceeded
	}
	v, _ := lineWaiters.LoadOrStore(br, &lineWaiter{ch: make(chan lineMsg, 1)})
	w := v.(*lineWaiter)

	w.mu.Lock()
	select {
	case r := <-w.ch:
		w.reading = false
		w.mu.Unlock()
		return r.s, r.err
	default:
	}
	if !w.reading {
		w.reading = true
		go func() {
			s, err := br.ReadString('\n')
			w.ch <- lineMsg{s: s, err: err}
		}()
	}
	w.mu.Unlock()

	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		select {
		case r := <-w.ch:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			w.mu.Lock()
			w.reading = false
			w.mu.Unlock()
			return r.s, r.err
		case <-timer.C:
			select {
			case r := <-w.ch:
				w.mu.Lock()
				w.reading = false
				w.mu.Unlock()
				return r.s, r.err
			default:
				return "", context.DeadlineExceeded
			}
		case <-ctx.Done():
			select {
			case r := <-w.ch:
				w.mu.Lock()
				w.reading = false
				w.mu.Unlock()
				return r.s, r.err
			default:
				return "", ctx.Err()
			}
		}
	}
}
