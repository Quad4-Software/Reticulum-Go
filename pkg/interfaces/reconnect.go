// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"
	"math/rand/v2"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
)

const idleReconnectInterval = 5 * time.Minute

// kickMinInterval rate-limits network-change kicks so a burst of netlink
// events cannot spin the dial loop faster than this.
const kickMinInterval = 750 * time.Millisecond

// jitterBackoff applies equal jitter (uniform in [base/2, base]) so fleets of
// nodes reconnecting to the same target do not synchronize. Same class of
// protection as tailscaled, nebula and go-libp2p dial backoffs.
func jitterBackoff(base time.Duration) time.Duration {
	if base <= time.Millisecond {
		return base
	}
	half := int64(base / 2)
	return time.Duration(half + rand.Int64N(half))
}

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
	kickCh            chan struct{}
	lastKick          time.Time
	attempted         atomic.Bool
}

func newReconnectDriver(label string, maxTries int, done chan struct{}, dial func() (net.Conn, error), onConnected func(net.Conn)) *reconnectDriver {
	return &reconnectDriver{
		maxReconnectTries: NormalizeMaxReconnectTries(maxTries),
		done:              done,
		dial:              dial,
		onConnected:       onConnected,
		label:             label,
		kickCh:            make(chan struct{}, 1),
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
	unsub := netwatchSubscribe(rd.kick)
	rd.mu.Unlock()
	go rd.run(unsub)
}

func (rd *reconnectDriver) run(unsub func()) {
	defer unsub()
	defer func() {
		rd.mu.Lock()
		rd.reconnecting = false
		rd.mu.Unlock()
	}()

	if rd.maxReconnectTries == ReconnectNever {
		rd.runNever()
		return
	}

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

		if !rd.wait(jitterBackoff(backoff)) {
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
		if !rd.wait(jitterBackoff(idleReconnectInterval)) {
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

// runNever implements max_reconnect_tries = 0 (ReconnectNever): the driver
// owns the initial dial, so exactly one dial is attempted per driver
// lifetime, and no retry runs after failure or a dropped session. Interfaces
// rebuild the driver on Start after Stop, so an operator restart still gets
// one fresh attempt. Previously the -2 sentinel fell into the
// negative-means-unlimited branch and reconnected forever.
func (rd *reconnectDriver) runNever() {
	if !rd.attempted.CompareAndSwap(false, true) {
		return
	}
	conn, err := rd.dial()
	if err != nil {
		debug.Log(debug.DebugError, "Reconnect disabled. Initial dial failed",
			"target", rd.label,
			"error", err)
		rd.mu.Lock()
		exhausted := rd.onExhausted
		rd.mu.Unlock()
		if exhausted != nil {
			exhausted()
		}
		return
	}
	if rd.shouldStop() {
		_ = conn.Close()
		return
	}
	rd.fireUp()
	rd.onConnected(conn)
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
	case <-rd.kickCh:
		// A network-change event woke the sleep early: retry now. Backoff
		// progression is unchanged so repeated kicks cannot spin the loop.
		return true
	case <-time.After(d):
		return true
	}
}

// kick wakes an in-progress backoff sleep so a reconnect attempt runs
// immediately after the underlay network changes (Tailscale netmon / nebula
// rebind pattern). Kicks are non-blocking and rate-limited.
func (rd *reconnectDriver) kick() {
	rd.mu.Lock()
	if now := time.Now(); !rd.lastKick.IsZero() && now.Sub(rd.lastKick) < kickMinInterval {
		rd.mu.Unlock()
		return
	}
	rd.lastKick = time.Now()
	rd.mu.Unlock()
	select {
	case rd.kickCh <- struct{}{}:
	default:
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
