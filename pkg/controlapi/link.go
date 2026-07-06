// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/packet"
)

// newOutboundLinkCallbacks builds the established/closed callbacks passed
// to link.NewLink for a link.open command, plus the linkSession they
// populate. destHashHex is only used to label a linkFailedEvent if the
// link never becomes active.
//
// link.Teardown and the establishment watchdog both invoke the closed
// callback while still holding the Link's own internal mutex, so closed
// must never call back into locked Link accessors like GetLinkID: doing so
// would deadlock. idHex is therefore cached on linkSession by the caller
// (see handleLinkOpen) and by established, which the link package always
// invokes from a fresh goroutine after releasing its lock.
func newOutboundLinkCallbacks(sess *session, destHashHex string) (ls *linkSession, established func(*link.Link), closed func(*link.Link)) {
	ls = &linkSession{}

	established = func(l *link.Link) {
		ls.link = l
		idHex := hex.EncodeToString(l.GetLinkID())

		ls.establishedMu.Lock()
		ls.idHex = idHex
		ls.established = true
		ls.establishedMu.Unlock()

		sess.addLink(idHex, ls)
		wireLinkDataEvents(sess, l, idHex)
		sess.broadcast(linkEstablishedEvent{Type: "link.established", LinkID: idHex, RemoteHash: remoteHashOf(l)})
	}

	closed = func(l *link.Link) {
		ls.establishedMu.Lock()
		idHex := ls.idHex
		wasEstablished := ls.established
		ls.establishedMu.Unlock()

		sess.removeLink(idHex)

		if wasEstablished {
			sess.broadcast(linkClosedEvent{Type: "link.closed", LinkID: idHex})
		} else {
			sess.broadcast(linkFailedEvent{
				Type:            "link.failed",
				LinkID:          idHex,
				DestinationHash: destHashHex,
				Error:           "link establishment failed or timed out",
			})
		}
	}

	return ls, established, closed
}

// wireInboundLinks makes dest accept links and reports every one that
// becomes active as a linkEstablishedEvent, forwarding its data and
// eventual teardown to the session's WebSocket clients.
func wireInboundLinks(sess *session, dest *destination.Destination) {
	dest.AcceptsLinks(true)
	dest.SetLinkEstablishedCallback(func(v any) {
		lnk, ok := v.(*link.Link)
		if !ok {
			return
		}

		idHex := hex.EncodeToString(lnk.GetLinkID())
		sess.addLink(idHex, &linkSession{link: lnk, idHex: idHex, established: true})
		wireLinkDataEvents(sess, lnk, idHex)

		lnk.SetLinkClosedCallback(func(*link.Link) {
			sess.removeLink(idHex)
			sess.broadcast(linkClosedEvent{Type: "link.closed", LinkID: idHex})
		})

		sess.broadcast(linkEstablishedEvent{Type: "link.established", LinkID: idHex, RemoteHash: remoteHashOf(lnk)})
	})
}

func wireLinkDataEvents(sess *session, lnk *link.Link, idHex string) {
	lnk.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		sess.broadcast(linkDataEvent{Type: "link.data", LinkID: idHex, Data: base64.StdEncoding.EncodeToString(data)})
	})
}

func remoteHashOf(lnk *link.Link) string {
	if remote := lnk.GetRemoteIdentity(); remote != nil {
		return remote.GetHexHash()
	}
	return ""
}

// handleLinkOpen processes a link.open command: it resolves the peer's
// identity from a previously-seen announce, opens an outbound link, and
// reports the outcome as linkEstablishedEvent or linkFailedEvent.
func (c *wsClient) handleLinkOpen(raw []byte) {
	var cmd linkOpenCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		debug.Log(debug.DebugError, "controlapi: invalid link.open command", "error", err)
		return
	}

	destHash, err := hex.DecodeString(cmd.DestinationHash)
	if err != nil || len(destHash) != 16 {
		c.send(linkFailedEvent{Type: "link.failed", DestinationHash: cmd.DestinationHash, Error: "destination_hash must be 16 hex-encoded bytes"})
		return
	}

	remoteIdentity, err := identity.Recall(destHash)
	if err != nil {
		c.send(linkFailedEvent{Type: "link.failed", DestinationHash: cmd.DestinationHash, Error: "unknown destination identity; wait for an announce first"})
		return
	}

	destOut, err := destination.FromHash(destHash, remoteIdentity, destination.Single, c.server.transport)
	if err != nil {
		c.send(linkFailedEvent{Type: "link.failed", DestinationHash: cmd.DestinationHash, Error: err.Error()})
		return
	}

	ls, established, closed := newOutboundLinkCallbacks(c.session, cmd.DestinationHash)
	lnk := link.NewLink(destOut, c.server.transport, nil, established, closed)
	if err := lnk.Establish(); err != nil {
		c.send(linkFailedEvent{Type: "link.failed", DestinationHash: cmd.DestinationHash, Error: err.Error()})
		return
	}

	// Establish always assigns the link ID before returning, so this is
	// safe to read here even though it is unsafe from inside closed: it
	// guarantees an ID is cached before any establishment-timeout failure
	// can reach closed, without ever calling back into the Link itself.
	ls.establishedMu.Lock()
	if ls.idHex == "" {
		ls.idHex = hex.EncodeToString(lnk.GetLinkID())
	}
	ls.establishedMu.Unlock()

	lnk.Start()
}

// handleLinkSend processes a link.send command, forwarding data over an
// already-established link owned by the session.
func (c *wsClient) handleLinkSend(raw []byte) {
	var cmd linkSendCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		debug.Log(debug.DebugError, "controlapi: invalid link.send command", "error", err)
		return
	}
	ls, ok := c.session.getLink(cmd.LinkID)
	if !ok {
		debug.Log(debug.DebugError, "controlapi: link.send for unknown link", "link_id", cmd.LinkID)
		return
	}
	data, err := base64.StdEncoding.DecodeString(cmd.Data)
	if err != nil {
		debug.Log(debug.DebugError, "controlapi: link.send data is not base64", "link_id", cmd.LinkID)
		return
	}
	if err := ls.link.SendPacket(data); err != nil {
		debug.Log(debug.DebugError, "controlapi: link.send failed", "link_id", cmd.LinkID, "error", err)
	}
}

// handleLinkClose processes a link.close command; the link's own closed
// callback emits the resulting linkClosedEvent.
func (c *wsClient) handleLinkClose(raw []byte) {
	var cmd linkCloseCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		debug.Log(debug.DebugError, "controlapi: invalid link.close command", "error", err)
		return
	}
	if ls, ok := c.session.getLink(cmd.LinkID); ok {
		ls.link.Teardown()
	}
}
