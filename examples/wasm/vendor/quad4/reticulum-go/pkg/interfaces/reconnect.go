// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"
	"net"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/debug"
)

const idleReconnectInterval = 5 * time.Minute

type reconnectDriver struct {
	mu                sync.Mutex
	reconnecting      bool
	maxReconnectTries int
	done              chan struct{}
	dial              func() (net.Conn, error)
	onConnected       func(net.Conn)
	onDown            func()
	onUp              func()
	onExhausted       func()
	label             string
	allowIdleRetry    bool
}

func newReconnectDriver(label string, maxTries int, done chan struct{}, dial func() (net.Conn, error), onConnected func(net.Conn)) *reconnectDriver {
	return &reconnectDriver{
		maxReconnectTries: NormalizeMaxReconnectTries(maxTries),
		done:              done,
		dial:              dial,
		onConnected:       onConnected,
		label:             label,
	}
}

func (rd *reconnectDriver) setHooks(onDown, onUp func()) {
	rd.mu.Lock()
	rd.onDown = onDown
	rd.onUp = onUp
	rd.mu.Unlock()
}

func (rd *reconnectDriver) setOnExhausted(fn func()) {
	rd.mu.Lock()
	rd.onExhausted = fn
	rd.mu.Unlock()
}

func (rd *reconnectDriver) setAllowIdleRetry(allow bool) {
	rd.mu.Lock()
	rd.allowIdleRetry = allow
	rd.mu.Unlock()
}

func (rd *reconnectDriver) fireDown() {
	rd.mu.Lock()
	fn := rd.onDown
	rd.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (rd *reconnectDriver) fireUp() {
	rd.mu.Lock()
	fn := rd.onUp
	rd.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (rd *reconnectDriver) start() {
	rd.mu.Lock()
	if rd.reconnecting {
		rd.mu.Unlock()
		return
	}
	rd.reconnecting = true
	rd.mu.Unlock()
	go rd.run()
}

func (rd *reconnectDriver) run() {
	defer func() {
		rd.mu.Lock()
		rd.reconnecting = false
		rd.mu.Unlock()
	}()

	backoff := InitialBackoff
	retries := 0
	unlimited := rd.maxReconnectTries < 0
	maxAttempts := rd.maxReconnectTries
	if unlimited {
		maxAttempts = 0
	}

	for unlimited || retries < maxAttempts {
		if rd.shouldStop() {
			return
		}

		conn, err := rd.dial()
		if err == nil {
			if rd.shouldStop() {
				_ = conn.Close()
				return
			}
			rd.fireUp()
			rd.onConnected(conn)
			return
		}

		debug.Log(debug.DebugVerbose, "Reconnect attempt failed",
			"target", rd.label,
			"attempt", retries+1,
			"maxTries", rd.maxReconnectTries,
			"error", err)

		if !rd.wait(backoff) {
			return
		}
		backoff *= 2
		if backoff > MaxBackoff {
			backoff = MaxBackoff
		}
		retries++
	}

	debug.Log(debug.DebugError, "Reconnect attempts exhausted",
		"target", rd.label,
		"maxTries", rd.maxReconnectTries)

	rd.mu.Lock()
	exhausted := rd.onExhausted
	allowIdle := rd.allowIdleRetry
	rd.mu.Unlock()
	if exhausted != nil {
		exhausted()
	}
	if !allowIdle {
		return
	}

	for {
		if !rd.wait(idleReconnectInterval) {
			return
		}
		if rd.shouldStop() {
			return
		}
		conn, err := rd.dial()
		if err == nil {
			if rd.shouldStop() {
				_ = conn.Close()
				return
			}
			rd.fireUp()
			rd.onConnected(conn)
			return
		}
		debug.Log(debug.DebugVerbose, "Idle reconnect attempt failed",
			"target", rd.label,
			"error", err)
	}
}

func (rd *reconnectDriver) shouldStop() bool {
	select {
	case <-rd.done:
		return true
	default:
		return false
	}
}

func (rd *reconnectDriver) wait(d time.Duration) bool {
	select {
	case <-rd.done:
		return false
	case <-time.After(d):
		return true
	}
}

func (rd *reconnectDriver) isActive() bool {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	return rd.reconnecting
}

func (rd *reconnectDriver) notifyFailure() {
	rd.fireDown()
	rd.start()
}

func tcpDialTarget(host string, port int) func() (net.Conn, error) {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	return func() (net.Conn, error) {
		return net.DialTimeout("tcp", addr, TCPConnectTimeout)
	}
}
