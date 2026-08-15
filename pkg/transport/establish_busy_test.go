// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"errors"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestOutboundEstablishOccupancy(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	defer tr.Close()
	dest := bytes.Repeat([]byte{0x11}, 16)
	if err := tr.TryBeginOutboundEstablish(dest); err != nil {
		t.Fatalf("first begin: %v", err)
	}
	if err := tr.TryBeginOutboundEstablish(dest); !errors.Is(err, common.ErrLinkEstablishBusy) {
		t.Fatalf("second begin = %v, want ErrLinkEstablishBusy", err)
	}
	tr.EndOutboundEstablish(dest)
	if err := tr.TryBeginOutboundEstablish(dest); err != nil {
		t.Fatalf("begin after end: %v", err)
	}
}

func TestRequestPathRejectsShortHash(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	defer tr.Close()
	err := tr.RequestPath([]byte{0x01}, "", nil, false)
	if !errors.Is(err, common.ErrTransportEmptyDestinationHash) {
		t.Fatalf("short RequestPath = %v, want ErrTransportEmptyDestinationHash", err)
	}
}
