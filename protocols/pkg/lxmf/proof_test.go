// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

type stubIface struct {
	sent [][]byte
}

func (s *stubIface) IsEnabled() bool { return true }
func (s *stubIface) IsOnline() bool  { return true }
func (s *stubIface) Send(data []byte, _ string) error {
	s.sent = append(s.sent, append([]byte(nil), data...))
	return nil
}

func TestSendDeliveryProof_ExplicitFormat(t *testing.T) {
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	cfg := common.DefaultConfig()
	tr := transport.NewTransport(cfg)
	dest, err := NewDeliveryDestination(id, tr)
	if err != nil {
		t.Fatalf("dest: %v", err)
	}

	inner := bytes.Repeat([]byte{0xAB}, 32)
	dataPkt := packet.NewPacket(
		packet.DestinationSingle,
		inner,
		packet.PacketTypeData,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		true,
		packet.FlagUnset,
	)
	dataPkt.DestinationHash = dest.GetHash()
	if err := dataPkt.Pack(); err != nil {
		t.Fatalf("pack data: %v", err)
	}

	iface := &stubIface{}
	if err := sendDeliveryProof(dest, dataPkt, iface); err != nil {
		t.Fatalf("sendDeliveryProof: %v", err)
	}
	if len(iface.sent) != 1 {
		t.Fatalf("expected one proof packet, got %d", len(iface.sent))
	}

	proof := &packet.Packet{Raw: iface.sent[0]}
	if err := proof.Unpack(); err != nil {
		t.Fatalf("unpack proof: %v", err)
	}
	if proof.PacketType != packet.PacketTypeProof {
		t.Fatalf("proof type: got 0x%02x", proof.PacketType)
	}
	if len(proof.Data) != packet.ExplicitLength {
		t.Fatalf("proof data len: got %d want %d", len(proof.Data), packet.ExplicitLength)
	}
	if !bytes.Equal(proof.Data[:identity.HashLength/8], dataPkt.GetHash()) {
		t.Fatal("proof hash mismatch")
	}
	if !bytes.Equal(proof.DestinationHash, dataPkt.TruncatedHash()) {
		t.Fatal("proof destination hash mismatch")
	}
}
