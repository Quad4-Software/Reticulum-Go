// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package backbone

import (
	"fmt"
	"net"
)

// socketFD converts a kernel file descriptor to int for epoll and similar syscalls.
func socketFD(ptr uintptr) int {
	// #nosec G115 -- kernel fds are small integers well below MaxInt32 on all supported OSes
	return int(ptr)
}

func connFD(conn net.Conn) (int, error) {
	switch c := conn.(type) {
	case *net.TCPConn:
		raw, err := c.SyscallConn()
		if err != nil {
			return -1, err
		}
		var fd int
		err = raw.Control(func(fdptr uintptr) {
			fd = socketFD(fdptr)
		})
		return fd, err
	case *net.UnixConn:
		raw, err := c.SyscallConn()
		if err != nil {
			return -1, err
		}
		var fd int
		err = raw.Control(func(fdptr uintptr) {
			fd = socketFD(fdptr)
		})
		return fd, err
	default:
		return -1, fmt.Errorf("unsupported connection type %T", conn)
	}
}
