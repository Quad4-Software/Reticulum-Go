// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package backbone

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"quad4/reticulum-go/pkg/debug"
)

type listenerSlot struct {
	ln     net.Listener
	accept func(net.Conn)
}

// Stream is a non-blocking TCP/unix connection managed by the hub.
type Stream struct {
	hub    *Hub
	conn   net.Conn
	fd     int
	mtu    int
	closed atomic.Bool

	mu      sync.Mutex
	txBuf   []byte
	wantOut bool
	decoder *HDLCDecoder

	onFrame func([]byte)
	onClose func()
}

// Hub multiplexes listeners and streams on a single kernel event loop.
type Hub struct {
	backend Backend
	poller  poller
	goMode  bool

	mu              sync.Mutex
	listeners       map[int]*listenerSlot
	streams         map[int]*Stream
	acceptListeners []net.Listener

	stop         chan struct{}
	stopOnce     sync.Once
	shutdownOnce sync.Once
	wg           sync.WaitGroup
}

func newHub(backend Backend) (*Hub, error) {
	p, err := newPoller(backend)
	if err != nil {
		return nil, err
	}
	h := &Hub{
		backend:   backend,
		poller:    p,
		goMode:    backend == BackendGo,
		listeners: make(map[int]*listenerSlot),
		streams:   make(map[int]*Stream),
		stop:      make(chan struct{}),
	}
	if !h.goMode {
		h.wg.Add(1)
		go h.loop()
	}
	return h, nil
}

// Backend reports the active multiplexer implementation.
func (h *Hub) Backend() Backend {
	return h.backend
}

// RegisterListener adds a TCP/unix listener. Accept runs in a dedicated goroutine.
// established connections are multiplexed on the hub event loop.
func (h *Hub) RegisterListener(ln net.Listener, accept func(net.Conn)) error {
	if err := h.errIfClosed(); err != nil {
		return err
	}
	h.mu.Lock()
	h.acceptListeners = append(h.acceptListeners, ln)
	h.mu.Unlock()
	if !h.beginWorker() {
		h.mu.Lock()
		h.removeAcceptListener(ln)
		h.mu.Unlock()
		return h.errIfClosed()
	}
	go h.acceptLoop(ln, accept)
	return nil
}

func (h *Hub) removeAcceptListener(ln net.Listener) {
	for i, l := range h.acceptListeners {
		if l == ln {
			h.acceptListeners = append(h.acceptListeners[:i], h.acceptListeners[i+1:]...)
			return
		}
	}
}

func (h *Hub) errIfClosed() error {
	select {
	case <-h.stop:
		return fmt.Errorf("backbone hub closed")
	default:
		return nil
	}
}

func (h *Hub) beginWorker() bool {
	if err := h.errIfClosed(); err != nil {
		return false
	}
	h.wg.Add(1)
	select {
	case <-h.stop:
		h.wg.Done()
		return false
	default:
		return true
	}
}

func (h *Hub) acceptLoop(ln net.Listener, accept func(net.Conn)) {
	defer h.wg.Done()
	defer func() {
		h.mu.Lock()
		h.removeAcceptListener(ln)
		h.mu.Unlock()
	}()
	for {
		select {
		case <-h.stop:
			return
		default:
		}
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-h.stop:
				return
			default:
				return
			}
		}
		if accept != nil {
			accept(conn)
		}
	}
}

// RegisterStream attaches a connected socket to the hub for HDLC I/O.
func (h *Hub) RegisterStream(conn net.Conn, mtu int, onFrame func([]byte), onClose func()) (*Stream, error) {
	if err := h.errIfClosed(); err != nil {
		return nil, err
	}
	fd, err := connFD(conn)
	if err != nil {
		fd = -1
	}
	s := &Stream{
		hub:     h,
		conn:    conn,
		fd:      fd,
		mtu:     mtu,
		onFrame: onFrame,
		onClose: onClose,
	}
	s.decoder = NewHDLCDecoder(mtu, onFrame)
	if h.goMode {
		h.mu.Lock()
		if fd >= 0 {
			h.streams[fd] = s
		}
		h.mu.Unlock()
		if !h.beginWorker() {
			h.mu.Lock()
			if fd >= 0 {
				delete(h.streams, fd)
			}
			h.mu.Unlock()
			return nil, h.errIfClosed()
		}
		go h.goReadLoop(s)
		return s, nil
	}
	if fd < 0 {
		return nil, err
	}
	h.mu.Lock()
	h.streams[fd] = s
	h.mu.Unlock()
	if err := h.poller.Add(fd, evRead); err != nil {
		h.mu.Lock()
		delete(h.streams, fd)
		h.mu.Unlock()
		return nil, fmt.Errorf("register stream fd %d: %w", fd, err)
	}
	return s, nil
}

func (h *Hub) goReadLoop(s *Stream) {
	defer h.wg.Done()
	buf := make([]byte, s.mtu)
	if len(buf) == 0 {
		buf = make([]byte, 1<<20)
	}
	for !s.closed.Load() {
		select {
		case <-h.stop:
			return
		default:
		}
		n, err := s.conn.Read(buf)
		if err != nil || n == 0 {
			s.Close()
			return
		}
		s.decoder.Feed(buf[:n])
	}
}

// QueueSend appends an HDLC frame to the transmit buffer and arms write interest.
func (s *Stream) QueueSend(payload []byte) {
	if s == nil || s.closed.Load() {
		return
	}
	frame := frameHDLC(payload)
	if s.hub.goMode {
		s.mu.Lock()
		s.txBuf = append(s.txBuf, frame...)
		buf := append([]byte(nil), s.txBuf...)
		s.txBuf = s.txBuf[:0]
		s.mu.Unlock()
		_, _ = s.conn.Write(buf)
		return
	}
	s.mu.Lock()
	s.txBuf = append(s.txBuf, frame...)
	needOut := !s.wantOut && len(s.txBuf) > 0
	if needOut {
		s.wantOut = true
	}
	s.mu.Unlock()
	if needOut {
		s.hub.pollerMod(s.fd, evRead|evWrite)
	}
}

func (h *Hub) pollerMod(fd int, events int) {
	if h.poller == nil {
		return
	}
	_ = h.poller.Mod(fd, events)
}

func (h *Hub) pollerDel(fd int) {
	if h.poller == nil {
		return
	}
	_ = h.poller.Del(fd)
}

// Close removes the stream from the hub and closes the socket.
func (s *Stream) Close() {
	if s == nil || !s.closed.CompareAndSwap(false, true) {
		return
	}
	if s.fd >= 0 {
		s.hub.removeStream(s.fd)
	}
	_ = s.conn.Close()
	if s.onClose != nil {
		s.onClose()
	}
}

func (h *Hub) removeStream(fd int) {
	if fd < 0 {
		return
	}
	h.mu.Lock()
	delete(h.streams, fd)
	h.mu.Unlock()
	if !h.goMode {
		h.pollerDel(fd)
	}
}

func (h *Hub) removeListener(fd int) {
	h.mu.Lock()
	delete(h.listeners, fd)
	h.mu.Unlock()
	h.pollerDel(fd)
}

// Close shuts down the hub event loop.
func (h *Hub) Close() {
	h.shutdownOnce.Do(func() {
		h.stopOnce.Do(func() { close(h.stop) })
		h.closeAllAcceptListeners()
		h.closeAllStreams()
		h.wg.Wait()
		if h.poller != nil {
			_ = h.poller.Close()
			h.poller = nil
		}
	})
}

func (h *Hub) closeAllAcceptListeners() {
	h.mu.Lock()
	lns := h.acceptListeners
	h.acceptListeners = nil
	h.mu.Unlock()
	for _, ln := range lns {
		_ = ln.Close()
	}
}

func (h *Hub) closeAllStreams() {
	h.mu.Lock()
	streams := make([]*Stream, 0, len(h.streams))
	for _, s := range h.streams {
		streams = append(streams, s)
	}
	h.streams = make(map[int]*Stream)
	h.mu.Unlock()
	for _, s := range streams {
		s.Close()
	}
}

func (h *Hub) loop() {
	defer h.wg.Done()
	readBuf := make([]byte, 1<<20)
	for {
		select {
		case <-h.stop:
			return
		default:
		}
		if h.poller == nil {
			return
		}
		events, err := h.poller.Wait(1)
		if err != nil {
			select {
			case <-h.stop:
				return
			default:
				debug.Log(debug.DebugError, "backbone poll", "error", err)
				continue
			}
		}
		for _, ev := range events {
			if ev.events&evHangup != 0 {
				h.handleHangup(ev.fd)
				continue
			}
			h.mu.Lock()
			stream := h.streams[ev.fd]
			h.mu.Unlock()
			if stream != nil {
				if ev.events&evRead != 0 {
					h.readStream(stream, readBuf)
				}
				if ev.events&evWrite != 0 {
					h.writeStream(stream)
				}
			}
		}
	}
}

func (h *Hub) readStream(s *Stream, buf []byte) {
	if s.closed.Load() {
		return
	}
	n := len(buf)
	if s.mtu > 0 && s.mtu < n {
		n = s.mtu
	}
	readN, err := s.conn.Read(buf[:n])
	if err != nil {
		if err == io.EOF {
			s.Close()
		}
		return
	}
	if readN > 0 {
		s.decoder.Feed(buf[:readN])
	}
}

func (h *Hub) writeStream(s *Stream) {
	s.mu.Lock()
	if len(s.txBuf) == 0 {
		s.wantOut = false
		s.mu.Unlock()
		h.pollerMod(s.fd, evRead)
		return
	}
	buf := append([]byte(nil), s.txBuf...)
	s.mu.Unlock()

	written, err := s.conn.Write(buf)
	if err != nil && written == 0 {
		return
	}

	s.mu.Lock()
	if written >= len(s.txBuf) {
		s.txBuf = s.txBuf[:0]
	} else if written > 0 {
		s.txBuf = append(s.txBuf[:0], s.txBuf[written:]...)
	}
	if len(s.txBuf) == 0 {
		s.wantOut = false
		s.mu.Unlock()
		h.pollerMod(s.fd, evRead)
		return
	}
	s.mu.Unlock()
}

func (h *Hub) handleHangup(fd int) {
	h.mu.Lock()
	stream := h.streams[fd]
	h.mu.Unlock()
	if stream != nil {
		stream.Close()
	}
}
