// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"errors"
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/packet"
)

// ErrInterfaceReceiveOnly is returned when transmit is blocked by outgoing=no.
var ErrInterfaceReceiveOnly = common.ErrInterfaceReceiveOnly

// AllowsOutgoing reports whether iface may transmit. Interfaces without an
// explicit AllowsOutgoing method default to true (tests and minimal mocks).
func AllowsOutgoing(iface common.NetworkInterface) bool {
	return common.InterfaceAllowsOutgoing(iface)
}

// sendOnInterface transmits data when the interface is enabled and outgoing.
func sendOnInterface(iface common.NetworkInterface, data []byte, address string) error {
	if iface == nil {
		return errors.New("nil interface")
	}
	if !iface.IsEnabled() {
		return fmt.Errorf("interface %q offline or disabled", iface.GetName())
	}
	if !AllowsOutgoing(iface) {
		debug.Log(debug.DebugVerbose, "Skipping transmit on receive-only interface", "name", iface.GetName())
		return ErrInterfaceReceiveOnly
	}
	return iface.Send(data, address)
}

// sendGroupBroadcast transmits a GROUP packet on every writable interface.
// Matches Python Transport.outbound skipping the path table for GROUP dests.
func (t *Transport) sendGroupBroadcast(p *packet.Packet) error {
	if t == nil || p == nil {
		return errors.New("nil transport or packet")
	}

	t.mutex.RLock()
	ifaces := make([]common.NetworkInterface, 0, len(t.interfaces))
	for _, iface := range t.interfaces {
		ifaces = append(ifaces, iface)
	}
	t.mutex.RUnlock()

	data, err := p.Serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize packet: %w", err)
	}

	sent := 0
	var lastErr error
	for _, iface := range ifaces {
		if iface == nil || !iface.IsEnabled() || !iface.IsOnline() {
			continue
		}
		if err := sendOnInterface(iface, data, ""); err != nil {
			if errors.Is(err, ErrInterfaceReceiveOnly) {
				continue
			}
			lastErr = err
			continue
		}
		sent++
	}
	if sent == 0 {
		if lastErr != nil {
			return lastErr
		}
		return errors.New("no interfaces could process the outbound packet")
	}

	p.Sent = true
	p.SentAt = time.Now()
	if p.CreateReceipt {
		receipt := packet.NewPacketReceipt(p)
		t.RegisterReceipt(receipt)
	}
	return nil
}
