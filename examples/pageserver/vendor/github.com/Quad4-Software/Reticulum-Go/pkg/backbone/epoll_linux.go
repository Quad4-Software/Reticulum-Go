// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package backbone

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

type epollPoller struct {
	fd int
}

func newEpollPoller() (poller, error) {
	fd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	return &epollPoller{fd: fd}, nil
}

func epollEvents(events int) uint32 {
	var e uint32
	if events&evRead != 0 {
		e |= unix.EPOLLIN | unix.EPOLLRDHUP
	}
	if events&evWrite != 0 {
		e |= unix.EPOLLOUT
	}
	return e
}

func (p *epollPoller) Add(fd int, events int) error {
	// #nosec G115 -- socket fds are bounded well below MaxInt32 on Linux
	return unix.EpollCtl(p.fd, unix.EPOLL_CTL_ADD, fd, &unix.EpollEvent{Events: epollEvents(events), Fd: int32(fd)})
}

func (p *epollPoller) Mod(fd int, events int) error {
	// #nosec G115 -- socket fds are bounded well below MaxInt32 on Linux
	return unix.EpollCtl(p.fd, unix.EPOLL_CTL_MOD, fd, &unix.EpollEvent{Events: epollEvents(events), Fd: int32(fd)})
}

func (p *epollPoller) Del(fd int) error {
	return unix.EpollCtl(p.fd, unix.EPOLL_CTL_DEL, fd, nil)
}

func (p *epollPoller) Wait(timeoutMs int) ([]pollEvent, error) {
	events := make([]unix.EpollEvent, 64)
	n, err := unix.EpollWait(p.fd, events, timeoutMs)
	if err != nil {
		if err == syscall.EINTR {
			return nil, nil
		}
		return nil, err
	}
	out := make([]pollEvent, 0, n)
	for i := range n {
		ev := pollEvent{fd: int(events[i].Fd)}
		if events[i].Events&(unix.EPOLLIN|unix.EPOLLRDHUP) != 0 {
			ev.events |= evRead
		}
		if events[i].Events&unix.EPOLLOUT != 0 {
			ev.events |= evWrite
		}
		if events[i].Events&(unix.EPOLLHUP|unix.EPOLLERR) != 0 {
			ev.events |= evHangup
		}
		out = append(out, ev)
	}
	return out, nil
}

func (p *epollPoller) Close() error {
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
