// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package backbone provides a process-wide multiplexed I/O hub for high-throughput
// TCP interfaces (BackboneInterface, local shared instance) using epoll, kqueue,
// or optional io_uring on Linux.
package backbone

import (
	"fmt"
	"runtime"
	"sync"
)

// Backend selects the kernel multiplexing implementation.
type Backend string

const (
	BackendAuto   Backend = "auto"
	BackendEpoll  Backend = "epoll"
	BackendKqueue Backend = "kqueue"
	BackendUring  Backend = "io_uring"
	BackendGo     Backend = "go"
)

var (
	globalHub *Hub
	globalMu  sync.Mutex
)

// ParseBackend normalises a configuration value.
func ParseBackend(s string) Backend {
	switch Backend(s) {
	case BackendEpoll, BackendKqueue, BackendUring, BackendGo:
		return Backend(s)
	default:
		return BackendAuto
	}
}

// DefaultBackend picks the native multiplexer for the current platform.
func DefaultBackend() Backend {
	switch runtime.GOOS {
	case "linux", "android":
		return BackendEpoll
	case "darwin", "freebsd", "netbsd", "openbsd":
		return BackendKqueue
	default:
		return BackendGo
	}
}

// Init creates and starts the process-wide I/O hub. Safe to call multiple times.
// subsequent calls return the existing hub if already initialised.
func Init(backend Backend) (*Hub, error) {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalHub != nil {
		return globalHub, nil
	}
	if backend == BackendAuto || backend == "" {
		backend = DefaultBackend()
	}
	h, err := newHub(backend)
	if err != nil {
		if backend == BackendUring {
			h, err = newHub(BackendEpoll)
			if err != nil {
				h, err = newHub(BackendGo)
			}
		} else if backend != BackendGo {
			h, err = newHub(BackendGo)
		}
		if err != nil {
			return nil, fmt.Errorf("backbone init: %w", err)
		}
	}
	globalHub = h
	return h, nil
}

// Get returns the global hub or nil if Init was not called.
func Get() *Hub {
	globalMu.Lock()
	defer globalMu.Unlock()
	return globalHub
}

// Shutdown stops and clears the global hub.
func Shutdown() {
	globalMu.Lock()
	h := globalHub
	globalHub = nil
	globalMu.Unlock()
	if h != nil {
		h.Close()
	}
}
