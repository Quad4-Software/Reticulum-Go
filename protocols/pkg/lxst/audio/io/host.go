// SPDX-License-Identifier: LicenseRef-Reticulum
//
//revive:disable:var-naming
package io

import (
	"context"
	"sync"
)

const defaultHostQueue = 32

// Host is a Device that an app feeds and drains in process.
// Capture is 48 kHz mono PCM16 pushed by the app. Playback is the same format
// written by the call stack. ReadPCM never blocks. Empty capture yields silence
// so the encoder keeps its frame clock.
type Host struct {
	mutex     sync.Mutex
	closed    bool
	started   bool
	frameSize int
	maxQueue  int
	capture   [][]int16
	play      [][]int16
	silence   []int16
	onPlay    func([]int16)
	playWake  chan struct{}
	dropCap   uint64
	dropPlay  uint64
}

type HostStats struct {
	CaptureQueued   int
	PlaybackQueued  int
	CaptureDropped  uint64
	PlaybackDropped uint64
	FrameSize       int
	Closed          bool
}

func NewHost() *Host {
	return NewHostSize(DefaultFrameSize)
}

func NewHostSize(frameSize int) *Host {
	if frameSize <= 0 {
		frameSize = DefaultFrameSize
	}
	return &Host{
		frameSize: frameSize,
		maxQueue:  defaultHostQueue,
		playWake:  make(chan struct{}, 1),
	}
}

func (h *Host) FrameSize() int {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	return h.frameSize
}

func (h *Host) SetPlaybackHandler(fn func([]int16)) {
	h.mutex.Lock()
	h.onPlay = fn
	h.mutex.Unlock()
}

func (h *Host) Start() error {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	if h.closed {
		return ErrDeviceClosed
	}
	h.started = true
	return nil
}

func (h *Host) ReadPCM() ([]int16, error) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	if h.closed {
		return nil, ErrDeviceClosed
	}
	if len(h.capture) == 0 {
		if len(h.silence) != h.frameSize {
			h.silence = make([]int16, h.frameSize)
		}
		return h.silence, nil
	}
	frame := h.capture[0]
	h.capture[0] = nil
	h.capture = h.capture[1:]
	return frame, nil
}

func (h *Host) WritePCM(pcm []int16) error {
	if len(pcm) == 0 {
		return nil
	}
	queued := append([]int16(nil), pcm...)
	h.mutex.Lock()
	if h.closed {
		h.mutex.Unlock()
		return ErrDeviceClosed
	}
	if len(h.play) >= h.maxQueue {
		h.play[0] = nil
		h.play = h.play[1:]
		h.dropPlay++
	}
	h.play = append(h.play, queued)
	cb := h.onPlay
	h.mutex.Unlock()
	h.signalPlay()
	if cb != nil {
		cb(append([]int16(nil), queued...))
	}
	return nil
}

func (h *Host) Close() error {
	h.mutex.Lock()
	h.closed = true
	h.capture = nil
	h.play = nil
	h.mutex.Unlock()
	h.signalPlay()
	return nil
}

func (h *Host) Closed() bool {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	return h.closed
}

func (h *Host) Push(pcm []int16) error {
	if len(pcm) == 0 {
		return nil
	}
	cp := append([]int16(nil), pcm...)
	h.mutex.Lock()
	defer h.mutex.Unlock()
	if h.closed {
		return ErrDeviceClosed
	}
	if len(h.capture) >= h.maxQueue {
		h.capture[0] = nil
		h.capture = h.capture[1:]
		h.dropCap++
	}
	h.capture = append(h.capture, cp)
	return nil
}

func (h *Host) PushBytes(raw []byte) error {
	pcm, err := FromPCM16LE(raw)
	if err != nil {
		return err
	}
	return h.Push(pcm)
}

func (h *Host) Pull() ([]int16, bool) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	if len(h.play) == 0 {
		return nil, false
	}
	frame := h.play[0]
	h.play[0] = nil
	h.play = h.play[1:]
	return frame, true
}

func (h *Host) PullBytes() ([]byte, bool) {
	pcm, ok := h.Pull()
	if !ok {
		return nil, false
	}
	return PCM16LE(pcm), true
}

func (h *Host) WaitPlayback(ctx context.Context) ([]int16, error) {
	for {
		if frame, ok := h.Pull(); ok {
			return frame, nil
		}
		h.mutex.Lock()
		closed := h.closed
		h.mutex.Unlock()
		if closed {
			return nil, ErrDeviceClosed
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-h.playWake:
		}
	}
}

func (h *Host) signalPlay() {
	select {
	case h.playWake <- struct{}{}:
	default:
	}
}

func (h *Host) Stats() HostStats {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	return HostStats{
		CaptureQueued:   len(h.capture),
		PlaybackQueued:  len(h.play),
		CaptureDropped:  h.dropCap,
		PlaybackDropped: h.dropPlay,
		FrameSize:       h.frameSize,
		Closed:          h.closed,
	}
}
