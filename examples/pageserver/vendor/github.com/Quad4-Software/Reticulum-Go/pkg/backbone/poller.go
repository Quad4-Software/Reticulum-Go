// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package backbone

const (
	evRead = 1 << iota
	evWrite
	evHangup
)

type pollEvent struct {
	fd     int
	events int
}

type poller interface {
	Add(fd int, events int) error
	Mod(fd int, events int) error
	Del(fd int) error
	Wait(timeoutMs int) ([]pollEvent, error)
	Close() error
}

func newPoller(backend Backend) (poller, error) {
	switch backend {
	case BackendUring:
		if p, err := newUringPoller(); err == nil {
			return p, nil
		}
		return newEpollPoller()
	case BackendEpoll:
		return newEpollPoller()
	case BackendKqueue:
		return newKqueuePoller()
	default:
		return newGoPoller(), nil
	}
}
