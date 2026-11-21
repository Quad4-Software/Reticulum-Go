// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"fmt"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
)

type proofReplyInterface interface {
	IsEnabled() bool
	IsOnline() bool
	Send(data []byte, address string) error
}

func sendDeliveryProof(dest *destination.Destination, pkt *packet.Packet, iface proofReplyInterface) error {
	if dest == nil || pkt == nil {
		return fmt.Errorf("lxmf: nil destination or packet")
	}
	signer := dest.GetIdentity()
	if signer == nil {
		return fmt.Errorf("lxmf: destination has no identity")
	}
	if iface == nil {
		return fmt.Errorf("lxmf: no receiving interface for proof")
	}

	packetHash := append([]byte(nil), pkt.GetHash()...)
	signature, err := signer.Sign(packetHash)
	if err != nil {
		return fmt.Errorf("lxmf: sign proof: %w", err)
	}

	proofData := make([]byte, 0, len(packetHash)+len(signature))
	proofData = append(proofData, packetHash...)
	proofData = append(proofData, signature...)

	proofPkt := packet.NewPacket(
		packet.DestinationSingle,
		proofData,
		packet.PacketTypeProof,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		false,
		packet.FlagUnset,
	)
	proofPkt.DestinationHash = append([]byte(nil), pkt.TruncatedHash()...)
	if len(proofPkt.DestinationHash) > identity.TruncatedHashLength/8 {
		proofPkt.DestinationHash = proofPkt.DestinationHash[:identity.TruncatedHashLength/8]
	}
	if err := proofPkt.Pack(); err != nil {
		return fmt.Errorf("lxmf: pack proof: %w", err)
	}
	if !iface.IsEnabled() || !iface.IsOnline() {
		return fmt.Errorf("lxmf: receiving interface offline")
	}
	if err := iface.Send(proofPkt.Raw, ""); err != nil {
		return fmt.Errorf("lxmf: send proof: %w", err)
	}
	return nil
}
