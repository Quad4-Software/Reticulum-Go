// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

// shouldForwardAnnounceOn reports whether an announce for destHash may leave
// on outIface given the receive/next-hop interface fromIface. When fromIface
// is the inbound interface (relay path), missing next hop is allowed. When
// fromIface is nil (local SendAnnounce), non-local destinations still require
// a path next hop.
func (t *Transport) shouldForwardAnnounceOn(destHash []byte, outIface, fromIface common.NetworkInterface) bool {
	if outIface == nil {
		return false
	}
	local := t.isLocalDestination(destHash)
	mode := outIface.GetMode()

	if !local && fromIface == nil {
		nh := t.nextHopNetworkInterface(destHash)
		if nh == nil {
			debug.Log(debug.DebugAll, "Blocking announce broadcast: next hop interface missing",
				"iface", outIface.GetName())
			return false
		}
		fromIface = nh
	}

	if !local && fromIface != nil && !announcesFromInternal(outIface) && fromIface.GetMode() == common.IFModeInternal {
		debug.Log(debug.DebugAll, "Blocking announce broadcast: internal-mode next hop",
			"iface", outIface.GetName())
		return false
	}

	switch mode {
	case common.IFModeAccessPoint:
		debug.Log(debug.DebugAll, "Blocking announce broadcast: AP mode",
			"iface", outIface.GetName())
		return false
	case common.IFModeInternal:
		if !local {
			if fromIface == nil {
				return false
			}
			if fromIface.GetMode() == common.IFModeBoundary {
				debug.Log(debug.DebugAll, "Blocking announce broadcast: boundary next hop to internal",
					"iface", outIface.GetName())
				return false
			}
		}
	case common.IFModeRoaming:
		if !local && fromIface != nil {
			fm := fromIface.GetMode()
			if fm == common.IFModeRoaming || fm == common.IFModeBoundary {
				debug.Log(debug.DebugAll, "Blocking announce broadcast: roaming/boundary next hop",
					"iface", outIface.GetName())
				return false
			}
		}
	case common.IFModeBoundary:
		if !local && fromIface != nil && fromIface.GetMode() == common.IFModeRoaming {
			debug.Log(debug.DebugAll, "Blocking announce broadcast: roaming next hop to boundary",
				"iface", outIface.GetName())
			return false
		}
	}
	return true
}

func announcesFromInternal(iface common.NetworkInterface) bool {
	type afi interface{ AnnouncesFromInternalFlag() bool }
	if v, ok := iface.(afi); ok {
		return v.AnnouncesFromInternalFlag()
	}
	// Default true when the concrete type does not expose the flag.
	if bi, ok := iface.(interface{ GetAnnouncesFromInternal() bool }); ok {
		return bi.GetAnnouncesFromInternal()
	}
	return true
}

func (t *Transport) isLocalDestination(destHash []byte) bool {
	if len(destHash) == 0 {
		return false
	}
	key := hash16FromSlice(destHash)
	t.mutex.RLock()
	_, ok := t.destinations[key]
	t.mutex.RUnlock()
	return ok
}

func (t *Transport) nextHopNetworkInterface(destHash []byte) common.NetworkInterface {
	name := t.NextHopInterface(destHash)
	if name == "" {
		return nil
	}
	t.mutex.RLock()
	iface := t.interfaces[name]
	t.mutex.RUnlock()
	return iface
}

// ifaceDiscoversUnknownPaths reports whether iface should search for unknown
// paths (discover modes or recursive_prs).
func ifaceDiscoversUnknownPaths(iface common.NetworkInterface) bool {
	if iface == nil {
		return false
	}
	type rp interface{ RecursivePRsEnabled() bool }
	if v, ok := iface.(rp); ok && v.RecursivePRsEnabled() {
		return true
	}
	return common.ModeDiscoversPaths(iface.GetMode())
}
