// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import "errors"

// ErrInterfaceReceiveOnly is returned when transmit is blocked by outgoing=no.
var ErrInterfaceReceiveOnly = errors.New("interface is receive-only")

// OutgoingPermit is the optional capability for interfaces that honor
// outgoing / selected_outgoing. Not part of NetworkInterface so minimal
// mocks keep defaulting to transmit-allowed via InterfaceAllowsOutgoing.
type OutgoingPermit interface {
	AllowsOutgoing() bool
}

// OutgoingController extends OutgoingPermit with config-driven updates.
type OutgoingController interface {
	OutgoingPermit
	SetOutgoingAllowed(allowed bool)
}

// InterfaceAllowsOutgoing reports whether iface may transmit.
// Types that do not implement OutgoingPermit default to true.
func InterfaceAllowsOutgoing(iface NetworkInterface) bool {
	if iface == nil {
		return false
	}
	if o, ok := iface.(OutgoingPermit); ok {
		return o.AllowsOutgoing()
	}
	return true
}

// RejectReceiveOnly returns ErrInterfaceReceiveOnly when transmit is blocked.
func RejectReceiveOnly(iface OutgoingPermit) error {
	if iface != nil && !iface.AllowsOutgoing() {
		return ErrInterfaceReceiveOnly
	}
	return nil
}
