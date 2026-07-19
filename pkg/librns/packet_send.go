// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"fmt"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

// DestinationEncrypt encrypts plaintext for a recalled destination hash.
// Used by LXMF opportunistic strip and propagation packing.
func DestinationEncrypt(destHash, plaintext []byte) ([]byte, int) {
	if len(destHash) != identity.TruncatedHashLength/8 {
		return nil, setLastError(errInvalidArg)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		return nil, setLastError(fmt.Errorf("%w: %v", errNotFound, err))
	}
	dest, err := destination.FromHash(destHash, remote, destination.Single, nil)
	if err != nil {
		return nil, setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	ct, err := dest.Encrypt(plaintext)
	if err != nil {
		return nil, setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	return ct, OK
}

// PacketSend encrypts plaintext for destHash and sends a DATA packet.
// Matches Python RNS.Packet(out_destination, plaintext).send() for opportunistic LXMF.
func PacketSend(nodeHandle uint64, destHash, plaintext []byte) int {
	nodeRec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if !nodeRec.started {
		return setLastError(errState)
	}
	if len(destHash) != identity.TruncatedHashLength/8 {
		return setLastError(errInvalidArg)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		return setLastError(fmt.Errorf("%w: %v", errNotFound, err))
	}
	dest, err := destination.FromHash(destHash, remote, destination.Single, nodeRec.node.Transport())
	if err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	encrypted, err := dest.Encrypt(plaintext)
	if err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	pkt := packet.NewPacket(
		packet.DestinationSingle,
		encrypted,
		packet.PacketTypeData,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		false,
		packet.FlagUnset,
	)
	pkt.DestinationHash = append([]byte(nil), destHash...)
	if err := pkt.Pack(); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	if err := nodeRec.node.Transport().SendPacket(pkt); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	return OK
}
