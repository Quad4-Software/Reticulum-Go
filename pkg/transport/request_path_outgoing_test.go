// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
)

func TestRequestPathNoOutgoingInterface(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	dest := bytes.Repeat([]byte{0x22}, 16)
	if err := tr.RequestPath(dest, "", nil, false); !errors.Is(err, common.ErrTransportNoOutgoingForPR) {
		t.Fatalf("RequestPath = %v, want ErrTransportNoOutgoingForPR", err)
	}
}

func TestRequestPathIfaceNotReady(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	out := newRelayIface("out")
	out.Disable()
	_ = tr.RegisterInterface("out", out)

	dest := bytes.Repeat([]byte{0x33}, 16)
	if err := tr.RequestPath(dest, "out", nil, false); !errors.Is(err, common.ErrTransportIfaceNotReadyForPR) {
		t.Fatalf("RequestPath = %v, want ErrTransportIfaceNotReadyForPR", err)
	}
}
