// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build tinygo

package sharedinstance

import (
	"fmt"
	"io"
	"net"
)

type RPCServer struct{}

func (r *RPCServer) Close() error {
	return nil
}

func AuthenticateClient(conn net.Conn, authkey []byte) error {
	_ = conn
	_ = authkey
	return fmt.Errorf("shared instance RPC is not supported on TinyGo")
}

func SendFramed(w io.Writer, buf []byte) error {
	_ = w
	_ = buf
	return fmt.Errorf("shared instance RPC is not supported on TinyGo")
}

func RecvFramed(r io.Reader, maxSize int) ([]byte, error) {
	_ = r
	_ = maxSize
	return nil, fmt.Errorf("shared instance RPC is not supported on TinyGo")
}
