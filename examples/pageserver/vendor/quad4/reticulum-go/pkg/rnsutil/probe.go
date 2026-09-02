// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

const (
	// DefaultProbeSize is the default random payload length for probes.
	DefaultProbeSize = 16
	// DefaultProbeTimeout is used when no first-hop timeout is available.
	DefaultProbeTimeout = 12 * time.Second
)

// ProbeResult holds the outcome of a single probe.
type ProbeResult struct {
	Delivered bool
	RTT       time.Duration
	Hops      uint8
	Size      int
}

// WaitPath blocks until HasPath is true or ctx is done.
// When ctx has no deadline the wait is PathResponseWindow.
func WaitPath(ctx context.Context, tr *transport.Transport, destHash []byte) error {
	if tr == nil {
		return fmt.Errorf("nil transport")
	}
	return tr.AwaitPath(ctx, destHash)
}

// SendProbe encrypts a random payload to destHash and waits for a proof.
func SendProbe(ctx context.Context, tr *transport.Transport, destHash []byte, size int) (ProbeResult, error) {
	var out ProbeResult
	if tr == nil {
		return out, fmt.Errorf("nil transport")
	}
	if size <= 0 {
		size = DefaultProbeSize
	}
	out.Size = size

	remote, err := identity.Recall(destHash)
	if err != nil {
		return out, fmt.Errorf("recall identity: %w", err)
	}
	target, err := destination.FromHash(destHash, remote, destination.Single, tr)
	if err != nil {
		return out, err
	}
	payload := make([]byte, size)
	if _, err := rand.Read(payload); err != nil {
		return out, err
	}
	encrypted, err := target.Encrypt(payload)
	if err != nil {
		return out, err
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
	pkt.DestinationHash = destHash
	if err := pkt.Pack(); err != nil {
		return out, err
	}

	receipt := packet.NewPacketReceipt(pkt)
	receipt.SetDestinationIdentity(remote)
	done := make(chan struct{}, 1)
	receipt.SetDeliveryCallback(func(*packet.PacketReceipt) {
		select {
		case done <- struct{}{}:
		default:
		}
	})
	tr.RegisterReceipt(receipt)
	if err := tr.SendPacket(pkt); err != nil {
		return out, err
	}

	select {
	case <-ctx.Done():
		return out, ctx.Err()
	case <-done:
	}

	out.Delivered = receipt.IsDelivered()
	out.RTT = receipt.GetRTT()
	out.Hops = tr.HopsTo(destHash)
	return out, nil
}
