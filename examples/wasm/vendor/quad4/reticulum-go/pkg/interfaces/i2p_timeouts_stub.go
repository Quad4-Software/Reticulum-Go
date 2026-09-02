// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build !linux && !rns_slim

package interfaces

import (
	"net"
	"time"
)

func setI2PConnTimeouts(conn net.Conn) error {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(time.Duration(I2PProbeIntervalSec) * time.Second)
	}
	return nil
}
