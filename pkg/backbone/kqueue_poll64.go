// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build darwin || (freebsd && (amd64 || arm64 || riscv64)) || (openbsd && (amd64 || arm64 || mips64 || ppc64 || riscv64))

package backbone

import (
	"net"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type kqueuePoller struct {
	fd int
}

func newKqueuePoller() (poller, error) {
	fd, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}
	unix.CloseOnExec(fd)
	return &kqueuePoller{fd: fd}, nil
}

func (p *kqueuePoller) control(fd int, events int, flags int) error {
	var kevs [2]unix.Kevent_t
	n := 0
	if events&evRead != 0 {
		unix.SetKevent(&kevs[n], fd, unix.EVFILT_READ, flags)
		n++
	}
	if events&evWrite != 0 {
		unix.SetKevent(&kevs[n], fd, unix.EVFILT_WRITE, flags)
		n++
	}
	if n == 0 {
		return nil
	}
	_, err := unix.Kevent(p.fd, kevs[:n], nil, nil)
	return err
}

func (p *kqueuePoller) Add(fd int, events int) error {
	return p.control(fd, events, unix.EV_ADD|unix.EV_ENABLE)
}

func (p *kqueuePoller) Mod(fd int, events int) error {
	_ = p.Del(fd)
	return p.Add(fd, events)
}

func (p *kqueuePoller) Del(fd int) error {
	_ = p.control(fd, evRead|evWrite, unix.EV_DELETE)
	return nil
}

func (p *kqueuePoller) Wait(timeoutMs int) ([]pollEvent, error) {
	timespec := unix.NsecToTimespec(int64(timeoutMs) * int64(time.Millisecond))
	events := make([]unix.Kevent_t, 64)
	n, err := unix.Kevent(p.fd, nil, events, &timespec)
	if err != nil {
		if err == syscall.EINTR {
			return nil, nil
		}
		return nil, err
	}
	byFD := make(map[int]*pollEvent)
	for i := 0; i < n; i++ {
		fd := int(events[i].Ident)
		ev, ok := byFD[fd]
		if !ok {
			ev = &pollEvent{fd: fd}
			byFD[fd] = ev
		}
		switch events[i].Filter {
		case unix.EVFILT_READ:
			ev.events |= evRead
		case unix.EVFILT_WRITE:
			ev.events |= evWrite
		}
		if events[i].Flags&unix.EV_EOF != 0 {
			ev.events |= evHangup
		}
	}
	out := make([]pollEvent, 0, len(byFD))
	for _, ev := range byFD {
		out = append(out, *ev)
	}
	return out, nil
}

func (p *kqueuePoller) Close() error {
	return unix.Close(p.fd)
}

func setNonblockConn(conn net.Conn) error {
	fd, err := connFD(conn)
	if err != nil {
		return err
	}
	return setNonblockFD(fd)
}

func setNonblockFD(fd int) error {
	return unix.SetNonblock(fd, true)
}
