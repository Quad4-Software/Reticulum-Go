// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
//go:build freebsd

package interfaces

import (
	"fmt"
	"net"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
)

func (tc *TCPClientInterface) setTimeoutsLinux() error {
	tcpConn, ok := tc.conn.(*net.TCPConn)
	if !ok {
		return fmt.Errorf("not a TCP connection")
	}

	if err := tcpConn.SetKeepAlive(true); err != nil {
		return fmt.Errorf("failed to enable keepalive: %v", err)
	}

	keepalivePeriod := TCPProbeIntervalSec * time.Second
	if tc.i2pTunneled {
		keepalivePeriod = I2PProbeIntervalSec * time.Second
	}

	if err := tcpConn.SetKeepAlivePeriod(keepalivePeriod); err != nil {
		debug.Log(debug.DebugVerbose, "Failed to set keepalive period", "error", err)
	}

	debug.Log(debug.DebugVerbose, "TCP keepalive configured (FreeBSD)", "i2p", tc.i2pTunneled)
	return nil
}

func (tc *TCPClientInterface) setTimeoutsOSX() error {
	return tc.setTimeoutsLinux()
}
