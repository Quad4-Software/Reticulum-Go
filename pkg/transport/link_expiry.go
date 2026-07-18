// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

// handleUnvalidatedLinkExpiry mirrors Python link-table proof-timeout rediscovery.
func (t *Transport) handleUnvalidatedLinkExpiry(entry *LinkRelayEntry) {
	if t == nil || entry == nil || len(entry.DestinationHash) == 0 {
		return
	}
	dest := append([]byte(nil), entry.DestinationHash...)

	lastPR := time.Time{}
	t.mutex.RLock()
	if ts, ok := t.lastPathRequest[pathMapKey(dest)]; ok {
		lastPR = ts
	}
	t.mutex.RUnlock()
	throttled := !lastPR.IsZero() && time.Since(lastPR) < PathRequestMI

	needPR := false
	markUnresponsive := false
	var blockedIface common.NetworkInterface

	if !t.HasPath(dest) {
		needPR = true
		debug.Log(debug.DebugInfo, "Rediscovering path after failed link (path missing)",
			"dest", fmt.Sprintf("%x", dest))
	} else if !throttled && entry.TakenHops == 0 {
		needPR = true
		debug.Log(debug.DebugInfo, "Rediscovering path after failed local-client link",
			"dest", fmt.Sprintf("%x", dest))
	} else if !throttled && t.HopsTo(dest) == 1 {
		needPR = true
		markUnresponsive = true
		blockedIface = entry.ReceivedIface
		debug.Log(debug.DebugInfo, "Rediscovering path after failed 1-hop link",
			"dest", fmt.Sprintf("%x", dest))
	} else if !throttled && entry.TakenHops == 1 {
		needPR = true
		markUnresponsive = true
		blockedIface = entry.ReceivedIface
		debug.Log(debug.DebugInfo, "Rediscovering path after failed link with local initiator",
			"dest", fmt.Sprintf("%x", dest))
	}

	if markUnresponsive && t.transportEnabled() {
		if blockedIface == nil || blockedIface.GetMode() != common.IFModeBoundary {
			t.MarkPathUnresponsive(dest)
		}
	}

	if needPR {
		if err := t.RequestPath(dest, "", nil, true); err != nil {
			debug.Log(debug.DebugInfo, "Path rediscovery after link expiry failed",
				"dest", fmt.Sprintf("%x", dest), "error", err)
		}
	}

	if !t.transportEnabled() {
		t.ExpirePath(dest)
	}
}
