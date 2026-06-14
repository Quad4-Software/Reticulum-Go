// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

// invalidateTransportPathAfterInitiatorFailure drops the cached transport path to
// l.destination when this node was the initiator and the link never reached
// StatusActive. Normal teardown after establishment leaves establishedAt set, so
// paths are not cleared on intentional disconnect.
func (l *Link) invalidateTransportPathAfterInitiatorFailure() {
	if l == nil || !l.initiator || l.transport == nil || l.destination == nil {
		return
	}
	if !l.establishedAt.IsZero() {
		return
	}
	dh := l.destination.GetHash()
	if len(dh) != 16 {
		return
	}
	l.transport.ExpirePath(dh)
}

// markInitiatorEstablishmentFailedLocked closes the link, unregisters it from the
// transport, expires the cached path to the destination, and notifies
// closedCallback. The link mutex must be held by the caller.
func (l *Link) markInitiatorEstablishmentFailedLocked() {
	if !l.initiator {
		return
	}
	l.status.Store(int32(StatusClosed))
	l.teardownReason = StatusFailed
	if l.transport != nil && len(l.linkID) > 0 {
		l.transport.UnregisterLink(l.linkID)
	}
	l.invalidateTransportPathAfterInitiatorFailure()
	if l.closedCallback != nil {
		l.closedCallback(l)
	}
}
