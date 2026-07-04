// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sharedinstance

import (
	"net"
	"testing"
)

func TestMPConnAuthRoundTrip(t *testing.T) {
	authkey := []byte("test-auth-key-for-shared-instance")
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	errCh := make(chan error, 2)
	go func() {
		errCh <- AuthenticateServer(serverConn, authkey)
	}()
	go func() {
		errCh <- AuthenticateClient(clientConn, authkey)
	}()
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("auth handshake: %v", err)
		}
	}
	payload := []byte("ping")
	recvCh := make(chan []byte, 1)
	errCh2 := make(chan error, 1)
	go func() {
		got, err := recvBytes(clientConn, 64)
		if err != nil {
			errCh2 <- err
			return
		}
		recvCh <- got
	}()
	if err := sendBytes(serverConn, payload); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case err := <-errCh2:
		t.Fatalf("recv: %v", err)
	case got := <-recvCh:
		if string(got) != string(payload) {
			t.Fatalf("payload mismatch: %q", got)
		}
	}
}

func TestMPConnAuthRejectsWrongKey(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	go func() { _ = AuthenticateServer(serverConn, []byte("server-key")) }()
	err := AuthenticateClient(clientConn, []byte("client-key"))
	if err == nil {
		t.Fatal("expected auth failure")
	}
}
