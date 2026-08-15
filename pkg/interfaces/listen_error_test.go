// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"errors"
	"net"
	"strconv"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestUDPStartPortConflict(t *testing.T) {
	holder, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	defer holder.Close()

	addr := holder.LocalAddr().String()
	ui, err := NewUDPInterface("udp-conflict", addr, "", true)
	if err != nil {
		t.Fatalf("NewUDPInterface: %v", err)
	}
	err = ui.Start()
	if !errors.Is(err, common.ErrPortConflict) {
		t.Fatalf("Start on busy port: got %v, want ErrPortConflict", err)
	}
}

func TestTCPServerStartPortConflict(t *testing.T) {
	holder, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer holder.Close()

	_, portStr, err := net.SplitHostPort(holder.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	srv, err := NewTCPServerInterface("tcp-conflict", "127.0.0.1", port, false, false, false)
	if err != nil {
		t.Fatalf("NewTCPServerInterface: %v", err)
	}
	err = srv.Start()
	if !errors.Is(err, common.ErrPortConflict) {
		t.Fatalf("Start on busy port: got %v, want ErrPortConflict", err)
	}
}
