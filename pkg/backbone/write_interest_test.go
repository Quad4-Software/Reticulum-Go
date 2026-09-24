// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package backbone

import (
	"io"
	"net"
	"sync"
	"testing"
)

// recordingPoller captures the last interest mask registered for a stream.
type recordingPoller struct {
	mu   sync.Mutex
	mods []int
}

func (p *recordingPoller) Add(fd int, events int) error { return nil }
func (p *recordingPoller) Del(fd int) error             { return nil }
func (p *recordingPoller) Close() error                 { return nil }
func (p *recordingPoller) Wait(int) ([]pollEvent, error) {
	select {}
}

func (p *recordingPoller) Mod(fd int, events int) error {
	p.mu.Lock()
	p.mods = append(p.mods, events)
	p.mu.Unlock()
	return nil
}

func (p *recordingPoller) last() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.mods) == 0 {
		return -1
	}
	return p.mods[len(p.mods)-1]
}

// QueueSend racing writeStream's empty-buffer disarm must never leave queued
// bytes with write interest cleared: the poller state after any interleave has
// to match the stream's buffered state.
func TestWriteStreamInterestRace(t *testing.T) {
	for i := range 400 {
		p := &recordingPoller{}
		h := &Hub{
			backend: BackendEpoll,
			poller:  p,
			streams: make(map[int]*Stream),
			stop:    make(chan struct{}),
		}
		c1, c2 := net.Pipe()
		drainDone := make(chan struct{})
		go func() {
			_, _ = io.Copy(io.Discard, c2)
			close(drainDone)
		}()

		s := &Stream{hub: h, conn: c1, fd: 1, decoder: NewHDLCDecoder(4096, nil)}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); h.writeStream(s) }()
		go func() { defer wg.Done(); s.QueueSend([]byte{byte(i), byte(i >> 8)}) }()
		wg.Wait()

		s.mu.Lock()
		pending := len(s.txBuf) > 0
		s.mu.Unlock()
		armed := p.last()&evWrite != 0
		// armed with an empty buffer is harmless (one spurious EPOLLOUT wakeup
		// self-corrects); pending bytes with write interest cleared is the bug.
		if pending && !armed {
			t.Fatalf("iter %d: %d queued bytes but last poller mod=%d disarmed evWrite",
				i, len(s.txBuf), p.last())
		}

		_ = c1.Close()
		_ = c2.Close()
		<-drainDone
	}
}
