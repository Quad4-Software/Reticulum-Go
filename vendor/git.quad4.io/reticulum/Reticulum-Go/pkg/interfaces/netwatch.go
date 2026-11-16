// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

// netwatchSubscribe registers fn for debounced underlay network-change
// notifications (link/address events), the same trigger Tailscale's netmon
// and nebula's rebind use to shorten roam recovery. Returns an unsubscribe
// function. Package var so tests can substitute a manual trigger.
var netwatchSubscribe = platformNetwatchSubscribe

// WatchNetworkChanges registers fn for debounced underlay network-change
// notifications. Platforms without a watcher return an inert unsubscribe.
func WatchNetworkChanges(fn func()) func() {
	return netwatchSubscribe(fn)
}
