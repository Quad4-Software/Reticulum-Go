// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"fmt"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/packet"
)

// InitializeProbeDestination registers rnstransport.probe when respond_to_probes
// is enabled. Incoming data packets to that destination are proved automatically.
func (t *Transport) InitializeProbeDestination() error {
	if t == nil || t.config == nil || !t.config.RespondToProbes {
		return nil
	}
	if t.transportIdentity == nil {
		return fmt.Errorf("transport identity not initialized")
	}
	if t.probeDestination != nil {
		return nil
	}

	dest, err := destination.New(t.transportIdentity, destination.In, destination.Single, "rnstransport", t, "probe")
	if err != nil {
		return fmt.Errorf("probe destination: %w", err)
	}
	dest.AcceptsLinks(false)
	dest.SetProofStrategy(destination.ProveAll)
	dest.SetPacketCallback(func(data []byte, iface common.NetworkInterface) {
		debug.Log(debug.DebugVerbose, "Probe destination received data", "bytes", len(data))
	})
	t.RegisterDestination(dest.GetHash(), dest)
	t.mutex.Lock()
	t.probeDestination = dest
	t.mutex.Unlock()

	debug.Log(debug.DebugCritical, "Transport will respond to probe requests",
		"destination", fmt.Sprintf("%x", dest.GetHash()))
	return nil
}

// maybeProvePacket sends a proof for a locally delivered data packet when the
// destination proof strategy requires it.
func (t *Transport) maybeProvePacket(pkt *packet.Packet, dest *destination.Destination, iface common.NetworkInterface) {
	if t == nil || pkt == nil || dest == nil {
		return
	}
	id := dest.GetIdentity()
	if id == nil {
		return
	}

	strategy := dest.ProofStrategy()
	switch strategy {
	case destination.ProveAll:
		// continue to prove
	case destination.ProveApp:
		if cb := dest.ProofRequestedCallback(); cb != nil {
			cb(pkt.GetHash(), pkt.Data)
		}
		return
	default:
		return
	}

	if err := t.sendPacketProof(pkt, id, iface); err != nil {
		debug.Log(debug.DebugInfo, "Failed to send packet proof", "error", err)
	}
}

func (t *Transport) sendPacketProof(pkt *packet.Packet, id interface {
	Sign([]byte) ([]byte, error)
}, iface common.NetworkInterface) error {
	hash := pkt.GetHash()
	sig, err := id.Sign(hash)
	if err != nil {
		return err
	}
	proofData := append(append([]byte(nil), hash...), sig...)

	proofDestHash := hash
	if len(proofDestHash) > packet.TruncatedHashLength {
		proofDestHash = proofDestHash[:packet.TruncatedHashLength]
	}

	proofPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeProof,
		TransportType:   packet.PropagationBroadcast,
		Context:         packet.ContextNone,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: packet.DestinationSingle,
		DestinationHash: append([]byte(nil), proofDestHash...),
		Data:            proofData,
		CreateReceipt:   false,
	}
	if err := proofPkt.Pack(); err != nil {
		return err
	}
	raw, err := proofPkt.Serialize()
	if err != nil {
		return err
	}

	if iface != nil {
		return sendOnInterface(iface, raw, "")
	}
	return t.SendPacket(proofPkt)
}
