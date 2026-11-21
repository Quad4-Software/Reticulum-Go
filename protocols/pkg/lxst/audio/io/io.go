// SPDX-License-Identifier: LicenseRef-Reticulum
//
//revive:disable:var-naming
package io

import (
	"errors"
	"sync"
)

var (
	ErrDeviceClosed = errors.New("audio device closed")
	ErrDeviceInit   = errors.New("audio device init failed")
)

const (
	DefaultSampleRate = 48000
	DefaultChannels   = 1
	DefaultFrameSize  = 960
)

// Device captures and plays PCM16 mono audio.
type Device interface {
	Start() error
	ReadPCM() ([]int16, error)
	WritePCM([]int16) error
	Close() error
}

// NullDevice discards capture and produces silence for playback.
type NullDevice struct {
	mutex     sync.Mutex
	closed    bool
	frameSize int
	silence   []int16
}

func NewNullDevice() *NullDevice {
	return &NullDevice{frameSize: DefaultFrameSize}
}

func NewNullDeviceSize(frameSize int) *NullDevice {
	if frameSize <= 0 {
		frameSize = DefaultFrameSize
	}
	return &NullDevice{frameSize: frameSize}
}

func (n *NullDevice) Start() error {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if n.closed {
		return ErrDeviceClosed
	}
	return nil
}

func (n *NullDevice) ReadPCM() ([]int16, error) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if n.closed {
		return nil, ErrDeviceClosed
	}
	if len(n.silence) != n.frameSize {
		n.silence = make([]int16, n.frameSize)
	}
	return n.silence, nil
}

func (n *NullDevice) WritePCM(_ []int16) error {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if n.closed {
		return ErrDeviceClosed
	}
	return nil
}

func (n *NullDevice) Close() error {
	n.mutex.Lock()
	n.closed = true
	n.mutex.Unlock()
	return nil
}
