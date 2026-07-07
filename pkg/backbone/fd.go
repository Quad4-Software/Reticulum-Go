// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package backbone

import (
	"fmt"
	"net"
	"os"
)

func connFD(conn net.Conn) (int, error) {
	switch c := conn.(type) {
	case *net.TCPConn:
		raw, err := c.SyscallConn()
		if err != nil {
			return -1, err
		}
		var fd int
		err = raw.Control(func(fdptr uintptr) {
			fd = int(fdptr)
		})
		return fd, err
	case *net.UnixConn:
		raw, err := c.SyscallConn()
		if err != nil {
			return -1, err
		}
		var fd int
		err = raw.Control(func(fdptr uintptr) {
			fd = int(fdptr)
		})
		return fd, err
	default:
		return -1, fmt.Errorf("unsupported connection type %T", conn)
	}
}

func listenerFD(ln net.Listener) (int, *os.File, error) {
	switch l := ln.(type) {
	case *net.TCPListener:
		f, err := l.File()
		if err != nil {
			return -1, nil, err
		}
		return int(f.Fd()), f, nil
	case *net.UnixListener:
		f, err := l.File()
		if err != nil {
			return -1, nil, err
		}
		return int(f.Fd()), f, nil
	default:
		return -1, nil, fmt.Errorf("unsupported listener type %T", ln)
	}
}
