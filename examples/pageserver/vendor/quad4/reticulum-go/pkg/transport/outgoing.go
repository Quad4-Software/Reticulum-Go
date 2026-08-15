// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"errors"
	"fmt"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
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
