// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package interfaces

import (
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
)

// netwatchDebounce collapses the burst of rtnetlink messages a roam or
// address change produces into one notification.
const netwatchDebounce = 600 * time.Millisecond

type netlinkWatcher struct {
	mu      sync.Mutex
	subs    map[int]func()
	nextID  int
	started bool
	pending bool
}

var nlWatcher = &netlinkWatcher{subs: make(map[int]func())}

func platformNetwatchSubscribe(fn func()) func() {
	return nlWatcher.subscribe(fn)
}

func (w *netlinkWatcher) subscribe(fn func()) func() {
	w.mu.Lock()
	id := w.nextID
	w.nextID++
	w.subs[id] = fn
	if !w.started {
		w.started = true
		go w.run()
	}
	w.mu.Unlock()
	return func() {
		w.mu.Lock()
		delete(w.subs, id)
		w.mu.Unlock()
	}
}

// run opens one process-wide rtnetlink socket lazily on first subscribe. The
// socket lives for the process lifetime. A read failure marks the watcher
// stopped so a later subscribe retries the open.
func (w *netlinkWatcher) run() {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		debug.Log(debug.DebugVerbose, "netwatch: netlink socket failed", "error", err)
		w.stop()
		return
	}
	addr := &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Groups: unix.RTMGRP_LINK | unix.RTMGRP_IPV4_IFADDR | unix.RTMGRP_IPV6_IFADDR,
	}
	if err := unix.Bind(fd, addr); err != nil {
		debug.Log(debug.DebugVerbose, "netwatch: netlink bind failed", "error", err)
		_ = unix.Close(fd)
		w.stop()
		return
	}

	buf := make([]byte, 8192)
	for {
		n, err := unix.Read(fd, buf)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			debug.Log(debug.DebugVerbose, "netwatch: netlink read failed", "error", err)
			_ = unix.Close(fd)
			w.stop()
			return
		}
		if n > 0 {
			w.noteEvent()
		}
	}
}

func (w *netlinkWatcher) stop() {
	w.mu.Lock()
	w.started = false
	w.mu.Unlock()
}

// noteEvent marks a netlink event. Messages inside a debounce window
// coalesce into a single fan-out.
func (w *netlinkWatcher) noteEvent() {
	w.mu.Lock()
	if w.pending {
		w.mu.Unlock()
		return
	}
	w.pending = true
	w.mu.Unlock()
	time.AfterFunc(netwatchDebounce, w.fire)
}

func (w *netlinkWatcher) fire() {
	w.mu.Lock()
	w.pending = false
	subs := make([]func(), 0, len(w.subs))
	for _, fn := range w.subs {
		subs = append(subs, fn)
	}
	w.mu.Unlock()
	for _, fn := range subs {
		if fn == nil {
			continue
		}
		func() {
			// A subscriber panic must not kill the watcher goroutine or
			// starve the remaining subscribers.
			defer func() {
				if r := recover(); r != nil {
					debug.Log(debug.DebugError, "netwatch: subscriber panicked", "error", r)
				}
			}()
			fn()
		}()
	}
}
