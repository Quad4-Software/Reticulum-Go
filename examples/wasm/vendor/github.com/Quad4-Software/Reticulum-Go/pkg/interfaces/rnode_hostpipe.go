// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"io"
	"net"
	"sync"
	"time"
)

// RNodeHostPipe is a SerialPort filled by an Android (or other) host thread.
// The host calls PushRX with bytes from USB/BLE and PullTX to fetch framed
// bytes the RNode stack wants to send. Used with RegisterRNodePortOpener.
type RNodeHostPipe struct {
	mu     sync.Mutex
	cond   *sync.Cond
	rx     []byte
	tx     []byte
	closed bool

	maxRX int
	maxTX int
}

// NewRNodeHostPipe builds a bidirectional host-fed pipe.
func NewRNodeHostPipe() *RNodeHostPipe {
	p := &RNodeHostPipe{
		maxRX: 256 * 1024,
		maxTX: 256 * 1024,
	}
	p.cond = sync.NewCond(&p.mu)
	return p
}

// PushRX appends bytes received from the radio transport (USB or BLE).
func (p *RNodeHostPipe) PushRX(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, io.ErrClosedPipe
	}
	if len(data) == 0 {
		return 0, nil
	}
	if len(p.rx)+len(data) > p.maxRX {
		drop := len(p.rx) + len(data) - p.maxRX
		if drop >= len(p.rx) {
			p.rx = p.rx[:0]
		} else {
			p.rx = append(p.rx[:0], p.rx[drop:]...)
		}
	}
	p.rx = append(p.rx, data...)
	p.cond.Broadcast()
	return len(data), nil
}

// PullTX copies pending host-bound TX bytes into dst and removes them.
func (p *RNodeHostPipe) PullTX(dst []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed && len(p.tx) == 0 {
		return 0, io.EOF
	}
	n := copy(dst, p.tx)
	if n > 0 {
		p.tx = append(p.tx[:0], p.tx[n:]...)
	}
	return n, nil
}

// WaitTX blocks until TX data is available, closed, or timeout elapses.
func (p *RNodeHostPipe) WaitTX(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.tx) == 0 && !p.closed {
		if timeout <= 0 {
			p.cond.Wait()
			continue
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}
		timer := time.AfterFunc(remaining, func() {
			p.mu.Lock()
			p.cond.Broadcast()
			p.mu.Unlock()
		})
		p.cond.Wait()
		timer.Stop()
	}
	return len(p.tx) > 0
}

func (p *RNodeHostPipe) Read(dst []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.rx) == 0 && !p.closed {
		p.cond.Wait()
	}
	if len(p.rx) == 0 && p.closed {
		return 0, io.EOF
	}
	n := copy(dst, p.rx)
	p.rx = append(p.rx[:0], p.rx[n:]...)
	return n, nil
}

func (p *RNodeHostPipe) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, io.ErrClosedPipe
	}
	if len(p.tx)+len(data) > p.maxTX {
		return 0, io.ErrShortBuffer
	}
	p.tx = append(p.tx, data...)
	p.cond.Broadcast()
	return len(data), nil
}

func (p *RNodeHostPipe) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	p.cond.Broadcast()
	return nil
}

// ServeRNodeHostPipeTCP listens on 127.0.0.1, accepts one client, and relays
// bytes between the TCP socket and pipe. Android Java USB/BLE helpers use this
// so Go RNode can dial tcp://127.0.0.1:port without JNI.
func ServeRNodeHostPipeTCP(pipe *RNodeHostPipe) (addr string, stop func(), err error) {
	if pipe == nil {
		return "", nil, io.ErrClosedPipe
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	done := make(chan struct{})
	var once sync.Once
	stop = func() {
		once.Do(func() {
			close(done)
			_ = ln.Close()
			_ = pipe.Close()
		})
	}
	go func() {
		defer stop()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		relayHostPipeConn(pipe, conn, done)
	}()
	return ln.Addr().String(), stop, nil
}

func relayHostPipeConn(pipe *RNodeHostPipe, conn net.Conn, done <-chan struct{}) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			default:
			}
			_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			n, err := conn.Read(buf)
			if n > 0 {
				_, _ = pipe.PushRX(buf[:n])
			}
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			default:
			}
			if !pipe.WaitTX(200 * time.Millisecond) {
				continue
			}
			n, err := pipe.PullTX(buf)
			if n > 0 {
				if _, werr := conn.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	wg.Wait()
}
