// SPDX-License-Identifier: LicenseRef-Reticulum
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
