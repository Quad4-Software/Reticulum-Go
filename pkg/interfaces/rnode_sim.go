// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"io"
	"sync"
)

// RNodeSim is an in-memory RNode transport for tests and integrations.
type RNodeSim struct {
	mu        sync.Mutex
	rx        chan []byte
	pending   []byte
	closed    chan struct{}
	closeOnce sync.Once
	decoder   *rnodeCmdDecoder
	peer      *RNodeSim
	selected  byte

	FirmwareMajor byte
	FirmwareMinor byte
	Platform      byte
	MCU           byte
	VPorts        int
	AutoReady     bool
}

// NewRNodeSim creates a loopback simulator with the requested virtual ports.
func NewRNodeSim(vports int) *RNodeSim {
	if vports <= 0 {
		vports = 1
	}
	s := &RNodeSim{
		rx:            make(chan []byte, 128),
		closed:        make(chan struct{}),
		FirmwareMajor: rnodeMultiRequiredMaj,
		FirmwareMinor: rnodeMultiRequiredMin,
		Platform:      rnodePlatformESP32,
		MCU:           0x01,
		VPorts:        vports,
		AutoReady:     true,
	}
	s.peer = s
	s.decoder = newRNodeCmdDecoder(rnodeHWMTU, s.handleHostFrame)
	return s
}

// PairRNodeSims relays data frames between two simulators.
func PairRNodeSims(a, b *RNodeSim) {
	if a == nil || b == nil {
		return
	}
	a.mu.Lock()
	a.peer = b
	a.mu.Unlock()
	b.mu.Lock()
	b.peer = a
	b.mu.Unlock()
}

func (s *RNodeSim) Read(dst []byte) (int, error) {
	s.mu.Lock()
	if len(s.pending) > 0 {
		n := copy(dst, s.pending)
		s.pending = s.pending[n:]
		s.mu.Unlock()
		return n, nil
	}
	s.mu.Unlock()
	select {
	case <-s.closed:
		return 0, io.EOF
	case data := <-s.rx:
		n := copy(dst, data)
		if n < len(data) {
			s.mu.Lock()
			s.pending = append(s.pending[:0], data[n:]...)
			s.mu.Unlock()
		}
		return n, nil
	}
}

func (s *RNodeSim) Write(data []byte) (int, error) {
	select {
	case <-s.closed:
		return 0, io.ErrClosedPipe
	default:
	}
	s.mu.Lock()
	s.decoder.feed(data)
	s.mu.Unlock()
	return len(data), nil
}

func (s *RNodeSim) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

func (s *RNodeSim) handleHostFrame(cmd byte, payload []byte) {
	switch cmd {
	case rnodeCmdDetect:
		s.enqueue(appendRNodeFrame(nil, rnodeCmdDetect, []byte{rnodeDetectResp}))
	case rnodeCmdFWVersion:
		s.enqueue(appendRNodeFrame(nil, rnodeCmdFWVersion, []byte{s.FirmwareMajor, s.FirmwareMinor}))
	case rnodeCmdPlatform:
		s.enqueue(appendRNodeFrame(nil, rnodeCmdPlatform, []byte{s.Platform}))
	case rnodeCmdMCU:
		s.enqueue(appendRNodeFrame(nil, rnodeCmdMCU, []byte{s.MCU}))
	case rnodeCmdInterfaces:
		for index := 0; index < s.VPorts; index++ {
			s.enqueue(appendRNodeFrame(nil, rnodeCmdInterfaces, []byte{byte(index), rnodeSX127X}))
		}
	case rnodeCmdSelInt:
		if len(payload) > 0 {
			s.selected = payload[0]
			s.enqueue(appendRNodeSelIntFrame(nil, s.selected))
		}
	case rnodeCmdFrequency, rnodeCmdBandwidth, rnodeCmdTXPower, rnodeCmdSF,
		rnodeCmdCR, rnodeCmdRadioState, rnodeCmdSTAlock, rnodeCmdLTAlock:
		s.enqueue(appendRNodeFrame(nil, cmd, payload))
	case rnodeCmdData:
		s.mu.Unlock()
		s.relayData(payload)
		s.mu.Lock()
		if s.AutoReady {
			s.enqueue(appendRNodeFrame(nil, rnodeCmdReady, []byte{0x01}))
		}
	}
}

func (s *RNodeSim) relayData(payload []byte) {
	s.mu.Lock()
	peer := s.peer
	index := s.selected
	s.mu.Unlock()
	if peer == nil {
		return
	}
	cmd := rnodeCmdData
	if int(index) < len(rnodeIntDataCmds) {
		cmd = rnodeIntDataCmds[index]
	}
	peer.enqueue(appendRNodeFrame(nil, cmd, payload))
}

func (s *RNodeSim) enqueue(frame []byte) {
	copyFrame := append([]byte(nil), frame...)
	select {
	case <-s.closed:
	case s.rx <- copyFrame:
	default:
	}
}

var _ SerialPort = (*RNodeSim)(nil)
