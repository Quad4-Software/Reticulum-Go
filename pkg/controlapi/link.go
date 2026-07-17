// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/resource"
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
		wireLinkEvents(sess, l, idHex)
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
		wireLinkEvents(sess, lnk, idHex)

		lnk.SetLinkClosedCallback(func(*link.Link) {
			sess.removeLink(idHex)
			sess.broadcast(linkClosedEvent{Type: "link.closed", LinkID: idHex})
		})

		sess.broadcast(linkEstablishedEvent{Type: "link.established", LinkID: idHex, RemoteHash: remoteHashOf(lnk)})
	})
}

func wireLinkEvents(sess *session, lnk *link.Link, idHex string) {
	lnk.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		sess.broadcast(linkDataEvent{Type: "link.data", LinkID: idHex, Data: base64.StdEncoding.EncodeToString(data)})
	})
	_ = lnk.SetResourceStrategy(link.AcceptAll)
	lnk.SetResourceStartedCallback(func(_ any) {
		sess.broadcast(resourceStartedEvent{Type: "resource.started", LinkID: idHex})
	})
	lnk.SetResourceConcludedCallback(func(v any) {
		evt := resourceConcludedEvent{Type: "resource.concluded", LinkID: idHex, Success: true}
		switch r := v.(type) {
		case link.IncomingResource:
			evt.Data = base64.StdEncoding.EncodeToString(r.Data)
			if name, ok := resourceNameFromMeta(r.Metadata); ok {
				evt.Name = name
			}
			if len(r.Hash) > 0 {
				evt.Hash = hex.EncodeToString(r.Hash)
			}
		case []byte:
			evt.Data = base64.StdEncoding.EncodeToString(r)
		case nil:
			evt.Success = false
			evt.Error = "resource transfer failed"
		default:
			evt.Success = false
			evt.Error = "unknown resource conclude payload"
		}
		sess.broadcast(evt)
	})
	lnk.SetRemoteIdentifiedCallback(func(_ *link.Link, remote *identity.Identity) {
		if remote == nil {
			return
		}
		sess.broadcast(linkRemoteIdentifiedEvent{
			Type:         "link.remote_identified",
			LinkID:       idHex,
			IdentityHash: remote.GetHexHash(),
		})
	})
}

func resourceNameFromMeta(meta map[string]any) (string, bool) {
	if meta == nil {
		return "", false
	}
	raw, ok := meta["name"]
	if !ok {
		return "", false
	}
	switch v := raw.(type) {
	case string:
		return v, true
	case []byte:
		return string(v), true
	default:
		return "", false
	}
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
		c.send(commandErrorEvent{Type: "command.error", Command: "link.open", Error: "invalid command json"})
		return
	}

	destHash, err := hex.DecodeString(cmd.DestinationHash)
	if err != nil || len(destHash) != 16 {
		c.send(linkFailedEvent{Type: "link.failed", DestinationHash: cmd.DestinationHash, Error: "destination_hash must be 16 hex-encoded bytes"})
		return
	}

	remoteIdentity, err := identity.Recall(destHash)
	if err != nil {
		c.send(linkFailedEvent{Type: "link.failed", DestinationHash: cmd.DestinationHash, Error: "unknown destination identity, wait for an announce first"})
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
		c.send(commandErrorEvent{Type: "command.error", Command: "link.send", Error: "invalid command json"})
		return
	}
	ls, ok := c.session.getLink(cmd.LinkID)
	if !ok {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.send", Error: "unknown link_id"})
		return
	}
	data, err := base64.StdEncoding.DecodeString(cmd.Data)
	if err != nil {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.send", Error: "data must be base64"})
		return
	}
	if err := ls.link.SendPacket(data); err != nil {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.send", Error: err.Error()})
	}
}

// handleLinkClose processes a link.close command. The link's own closed
// callback emits the resulting linkClosedEvent.
func (c *wsClient) handleLinkClose(raw []byte) {
	var cmd linkCloseCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.close", Error: "invalid command json"})
		return
	}
	ls, ok := c.session.getLink(cmd.LinkID)
	if !ok {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.close", Error: "unknown link_id"})
		return
	}
	ls.link.Teardown()
}

func (c *wsClient) handleLinkRequest(raw []byte) {
	var cmd linkRequestCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.request", Error: "invalid command json"})
		return
	}
	if cmd.Path == "" {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.request", Error: "path is required"})
		return
	}
	ls, ok := c.session.getLink(cmd.LinkID)
	if !ok {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.request", Error: "unknown link_id"})
		return
	}
	var payload any
	if cmd.Data != "" {
		data, err := base64.StdEncoding.DecodeString(cmd.Data)
		if err != nil {
			c.send(commandErrorEvent{Type: "command.error", Command: "link.request", Error: "data must be base64"})
			return
		}
		payload = data
	}
	timeout := time.Duration(cmd.TimeoutMs) * time.Millisecond
	if cmd.TimeoutMs <= 0 {
		timeout = 0
	}
	receipt, err := ls.link.Request(cmd.Path, payload, timeout)
	if err != nil {
		c.send(requestFailedEvent{
			Type:   "request.failed",
			LinkID: cmd.LinkID,
			Path:   cmd.Path,
			Error:  err.Error(),
		})
		return
	}
	linkID := cmd.LinkID
	path := cmd.Path
	receipt.SetResponseCallback(func(r *link.RequestReceipt) {
		c.session.broadcast(requestResponseEvent{
			Type:      "request.response",
			LinkID:    linkID,
			RequestID: hex.EncodeToString(r.GetRequestID()),
			Path:      path,
			Data:      base64.StdEncoding.EncodeToString(r.GetResponse()),
		})
	})
	receipt.SetFailedCallback(func(r *link.RequestReceipt) {
		c.session.broadcast(requestFailedEvent{
			Type:      "request.failed",
			LinkID:    linkID,
			RequestID: hex.EncodeToString(r.GetRequestID()),
			Path:      path,
			Error:     "request failed or timed out",
		})
	})
}

func (c *wsClient) handleLinkSendResource(raw []byte) {
	var cmd linkSendResourceCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.send_resource", Error: "invalid command json"})
		return
	}
	ls, ok := c.session.getLink(cmd.LinkID)
	if !ok {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.send_resource", Error: "unknown link_id"})
		return
	}
	data, err := base64.StdEncoding.DecodeString(cmd.Data)
	if err != nil {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.send_resource", Error: "data must be base64"})
		return
	}
	res, err := resource.New(append([]byte(nil), data...), false)
	if err != nil {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.send_resource", Error: err.Error()})
		return
	}
	if cmd.Name != "" {
		if err := res.SetMetadata(map[string]any{"name": []byte(cmd.Name)}); err != nil {
			c.send(commandErrorEvent{Type: "command.error", Command: "link.send_resource", Error: err.Error()})
			return
		}
	}
	linkID := cmd.LinkID
	go func() {
		if err := ls.link.SendResource(res); err != nil {
			c.session.broadcast(resourceConcludedEvent{
				Type:    "resource.concluded",
				LinkID:  linkID,
				Name:    cmd.Name,
				Success: false,
				Error:   err.Error(),
			})
		}
	}()
}

func (c *wsClient) handleLinkIdentify(raw []byte) {
	var cmd linkIdentifyCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.identify", Error: "invalid command json"})
		return
	}
	ls, ok := c.session.getLink(cmd.LinkID)
	if !ok {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.identify", Error: "unknown link_id"})
		return
	}
	if err := ls.link.Identify(c.session.identity); err != nil {
		c.send(commandErrorEvent{Type: "command.error", Command: "link.identify", Error: err.Error()})
	}
}
