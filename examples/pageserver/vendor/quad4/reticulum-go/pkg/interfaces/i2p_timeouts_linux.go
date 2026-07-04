// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build linux

package interfaces

import (
	"fmt"
	"net"
	"syscall"

	"quad4/reticulum-go/pkg/debug"
)

func setI2PConnTimeouts(conn net.Conn) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return fmt.Errorf("not a TCP connection")
	}
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return err
	}
	var sockoptErr error
	err = rawConn.Control(func(fd uintptr) {
		fdInt, ok := linuxFdToInt(fd)
		if !ok {
			sockoptErr = fmt.Errorf("invalid file descriptor")
			return
		}
		userTimeout := I2PUserTimeoutSec * TCPMilliseconds
		const TCP_USER_TIMEOUT = 18
		const TCP_KEEPIDLE = 4
		const TCP_KEEPINTVL = 5
		const TCP_KEEPCNT = 6
		if err := syscall.SetsockoptInt(fdInt, syscall.IPPROTO_TCP, TCP_USER_TIMEOUT, userTimeout); err != nil {
			debug.Log(debug.DebugVerbose, "Failed to set TCP_USER_TIMEOUT", "error", err)
		}
		if err := syscall.SetsockoptInt(fdInt, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, SOKeepaliveEnable); err != nil {
			sockoptErr = err
			return
		}
		_ = syscall.SetsockoptInt(fdInt, syscall.IPPROTO_TCP, TCP_KEEPIDLE, I2PProbeAfterSec)
		_ = syscall.SetsockoptInt(fdInt, syscall.IPPROTO_TCP, TCP_KEEPINTVL, I2PProbeIntervalSec)
		_ = syscall.SetsockoptInt(fdInt, syscall.IPPROTO_TCP, TCP_KEEPCNT, I2PProbesCount)
	})
	if err != nil {
		return err
	}
	return sockoptErr
}
