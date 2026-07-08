// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build tinygo

package backbone

import (
	"fmt"
	"net"
	"os"
)

func connFD(conn net.Conn) (int, error) {
	_ = conn
	return -1, fmt.Errorf("fd extraction not supported on TinyGo")
}

func listenerFD(ln net.Listener) (int, *os.File, error) {
	_ = ln
	return -1, nil, fmt.Errorf("fd extraction not supported on TinyGo")
}
