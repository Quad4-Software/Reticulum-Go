// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build linux

package interfaces

import (
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"

	"quad4/reticulum-go/pkg/debug"
)

func linuxFdToInt(fd uintptr) (int, bool) {
	maxInt := int(^uint(0) >> 1)
	if fd > uintptr(maxInt) {
		return 0, false
	}
	// #nosec G115 -- fd is bounded above by max int, invalid fds rejected above
	return int(fd), true
}

func (tc *TCPClientInterface) setTimeoutsLinux() error {
	tcpConn, ok := tc.conn.(*net.TCPConn)
	if !ok {
		return fmt.Errorf("not a TCP connection")
	}

	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return fmt.Errorf("failed to get raw connection: %v", err)
	}

	var sockoptErr error
	err = rawConn.Control(func(fd uintptr) {
		fdInt, ok := linuxFdToInt(fd)
		if !ok {
			sockoptErr = fmt.Errorf("invalid file descriptor")
			return
		}

		var userTimeout, probeAfter, probeInterval, probeCount int

		if tc.i2pTunneled {
			userTimeout = I2PUserTimeoutSec * TCPMilliseconds
			probeAfter = I2PProbeAfterSec
			probeInterval = I2PProbeIntervalSec
			probeCount = I2PProbesCount
		} else {
			userTimeout = TCPUserTimeoutSec * TCPMilliseconds
			probeAfter = TCPProbeAfterSec
			probeInterval = TCPProbeIntervalSec
			probeCount = TCPProbesCount
		}

		const TCP_USER_TIMEOUT = 18
		const TCP_KEEPIDLE = 4
		const TCP_KEEPINTVL = 5
		const TCP_KEEPCNT = 6

		if err := syscall.SetsockoptInt(fdInt, syscall.IPPROTO_TCP, TCP_USER_TIMEOUT, userTimeout); err != nil {
			debug.Log(debug.DebugVerbose, "Failed to set TCP_USER_TIMEOUT", "error", err)
		}

		if err := syscall.SetsockoptInt(fdInt, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, SOKeepaliveEnable); err != nil {
			sockoptErr = fmt.Errorf("failed to enable SO_KEEPALIVE: %v", err)
			return
		}

		if err := syscall.SetsockoptInt(fdInt, syscall.IPPROTO_TCP, TCP_KEEPIDLE, probeAfter); err != nil {
			debug.Log(debug.DebugVerbose, "Failed to set TCP_KEEPIDLE", "error", err)
		}

		if err := syscall.SetsockoptInt(fdInt, syscall.IPPROTO_TCP, TCP_KEEPINTVL, probeInterval); err != nil {
			debug.Log(debug.DebugVerbose, "Failed to set TCP_KEEPINTVL", "error", err)
		}

		if err := syscall.SetsockoptInt(fdInt, syscall.IPPROTO_TCP, TCP_KEEPCNT, probeCount); err != nil {
			debug.Log(debug.DebugVerbose, "Failed to set TCP_KEEPCNT", "error", err)
		}
	})

	if err != nil {
		return fmt.Errorf("control failed: %v", err)
	}
	if sockoptErr != nil {
		return sockoptErr
	}

	debug.Log(debug.DebugVerbose, "TCP keepalive configured (Linux)", "i2p", tc.i2pTunneled)
	return nil
}

func (tc *TCPClientInterface) setTimeoutsOSX() error {
	return tc.setTimeoutsLinux()
}

func platformGetRTT(fd uintptr) time.Duration {
	var info syscall.TCPInfo
	infoLen := uint32(unsafe.Sizeof(info))

	const TCP_INFO = 11
	// #nosec G103
	_, _, errno := syscall.Syscall6(
		syscall.SYS_GETSOCKOPT,
		fd,
		syscall.IPPROTO_TCP,
		TCP_INFO,
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Pointer(&infoLen)),
		0,
	)

	if errno != 0 {
		return 0
	}

	return time.Duration(info.Rtt) * time.Microsecond
}
