// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd

package backbone

import "net"

func setNonblockConn(net.Conn) error { return nil }
func setNonblockFD(int) error        { return nil }
