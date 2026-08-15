// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sharedinstance

import (
	"testing"
)

func TestBughuntRPCServerCloseIdempotent(t *testing.T) {
	s := &RPCServer{done: make(chan struct{})}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
