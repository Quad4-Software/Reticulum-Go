// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/resource"
)

// LinkOpen starts an outbound link to destHash on nodeHandle.
func LinkOpen(nodeHandle uint64, destHash []byte) (uint64, int) {
	nodeRec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return 0, setLastError(err)
	}
	if !nodeRec.started {
		return 0, setLastError(errState)
	}
	if len(destHash) != identity.TruncatedHashLength/8 {
		return 0, setLastError(errInvalidArg)
	}

	if err := nodeRec.node.Transport().AwaitPath(context.Background(), destHash); err != nil {
		nodeRec.enqueue(Event{
			Kind:            EventLinkFailed,
			DestinationHash: append([]byte(nil), destHash...),
			ErrorMessage:    err.Error(),
		})
		return 0, setLastError(err)
	}

	remoteIdentity, err := identity.Recall(destHash)
	if err != nil {
		nodeRec.enqueue(Event{
			Kind:            EventLinkFailed,
			DestinationHash: append([]byte(nil), destHash...),
			ErrorMessage:    "unknown destination identity",
		})
		return 0, setLastError(fmt.Errorf("%w: %v", errNotFound, err))
	}

	destOut, err := destination.FromHash(destHash, remoteIdentity, destination.Single, nodeRec.node.Transport())
	if err != nil {
		nodeRec.enqueue(Event{
			Kind:            EventLinkFailed,
			DestinationHash: append([]byte(nil), destHash...),
			ErrorMessage:    err.Error(),
		})
		return 0, setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}

	lr := &linkRecord{nodeID: nodeHandle}
	established := func(l *link.Link) {
		lr.link = l
		lr.id = append([]byte(nil), l.GetLinkID()...)
		lr.established = true
		wireLinkData(nodeRec, lr)
		nodeRec.enqueue(Event{
			Kind:            EventLinkEstablished,
			LinkID:          append([]byte(nil), lr.id...),
			DestinationHash: append([]byte(nil), destHash...),
			IdentityHash:    remoteIdentityBytes(l),
		})
	}
	closed := func(l *link.Link) {
		if lr.established {
			nodeRec.enqueue(Event{Kind: EventLinkClosed, LinkID: append([]byte(nil), lr.id...)})
		} else {
			nodeRec.enqueue(Event{
				Kind:            EventLinkFailed,
				DestinationHash: append([]byte(nil), destHash...),
				ErrorMessage:    "link establishment failed or timed out",
			})
		}
		runtimeMu.Lock()
		for h, rec := range nodeRec.links {
			if rec == lr {
				delete(nodeRec.links, h)
				handles.delete(h)
				break
			}
		}
		runtimeMu.Unlock()
	}

	lnk := link.NewLink(destOut, nodeRec.node.Transport(), nil, established, closed)
	if err := lnk.Establish(); err != nil {
		nodeRec.enqueue(Event{
			Kind:            EventLinkFailed,
			DestinationHash: append([]byte(nil), destHash...),
			ErrorMessage:    err.Error(),
		})
		return 0, setLastError(err)
	}
	if lr.id == nil {
		lr.id = append([]byte(nil), lnk.GetLinkID()...)
	}
	lnk.SetLinkClosedCallback(closed)
	lnk.Start()

	runtimeMu.Lock()
	linkHandle := handles.insert(kindLink, lr)
	nodeRec.links[linkHandle] = lr
	runtimeMu.Unlock()
	return linkHandle, OK
}

// LinkSend transmits data on an established link.
func LinkSend(linkHandle uint64, data []byte) int {
	lr, err := linkByHandle(linkHandle)
	if err != nil {
		return setLastError(err)
	}
	if lr.link == nil || !lr.established {
		return setLastError(errState)
	}
	if err := lr.link.SendPacket(data); err != nil {
		return setLastError(err)
	}
	return OK
}

// LinkSendResource transfers data as a link resource.
// When name is non-empty it is attached as rncp-compatible metadata.
func LinkSendResource(linkHandle uint64, data []byte, name string) int {
	lr, err := linkByHandle(linkHandle)
	if err != nil {
		return setLastError(err)
	}
	if lr.link == nil || !lr.established {
		return setLastError(errState)
	}
	res, err := resource.New(append([]byte(nil), data...), false)
	if err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	if name != "" {
		if err := res.SetMetadata(map[string]any{"name": []byte(name)}); err != nil {
			return setLastError(fmt.Errorf("%w: %v", errInternal, err))
		}
	}
	go func() {
		if err := lr.link.SendResource(res); err != nil {
			if nodeRec, nerr := nodeByHandle(lr.nodeID); nerr == nil {
				nodeRec.enqueue(Event{
					Kind:         EventRequestFailed,
					LinkID:       append([]byte(nil), lr.id...),
					ErrorMessage: err.Error(),
					Path:         "resource",
				})
			}
		}
	}()
	return OK
}

// LinkClose tears down a link.
func LinkClose(linkHandle uint64) int {
	lr, err := linkByHandle(linkHandle)
	if err != nil {
		return setLastError(err)
	}
	if lr.link != nil {
		lr.link.Teardown()
	}
	return OK
}

// LinkID returns the 16-byte link identifier.
func LinkID(linkHandle uint64) ([]byte, int) {
	lr, err := linkByHandle(linkHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	if lr.id == nil {
		return nil, setLastError(errState)
	}
	out := make([]byte, len(lr.id))
	copy(out, lr.id)
	return out, OK
}

// LinkFromID returns the handle for an established link on nodeHandle matching linkID.
// Used after inbound RNS_EV_LINK_ESTABLISHED events which only expose the link id.
func LinkFromID(nodeHandle uint64, linkID []byte) (uint64, int) {
	nodeRec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return 0, setLastError(err)
	}
	if len(linkID) == 0 {
		return 0, setLastError(errInvalidArg)
	}
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	for h, lr := range nodeRec.links {
		if lr != nil && bytes.Equal(lr.id, linkID) {
			return h, OK
		}
	}
	return 0, setLastError(errNotFound)
}

// LinkRequest sends a request on an established link.
// Completion arrives as EventRequestResponse or EventRequestFailed on the node queue.
func LinkRequest(nodeHandle, linkHandle uint64, path string, data []byte, timeoutMs int) (requestID []byte, code int) {
	if path == "" {
		return nil, setLastError(errInvalidArg)
	}
	nodeRec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	lr, err := linkByHandle(linkHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	if lr.link == nil || !lr.established {
		return nil, setLastError(errState)
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeoutMs <= 0 {
		timeout = 0
	}
	payload := decodeLinkRequestPayload(data)
	receipt, err := lr.link.Request(path, payload, timeout)
	if err != nil {
		return nil, setLastError(err)
	}
	id := receipt.GetRequestID()
	linkID := append([]byte(nil), lr.id...)
	receipt.SetResponseCallback(func(r *link.RequestReceipt) {
		nodeRec.enqueue(Event{
			Kind:      EventRequestResponse,
			LinkID:    linkID,
			RequestID: append([]byte(nil), r.GetRequestID()...),
			Path:      path,
			AppData:   r.GetResponse(),
		})
	})
	receipt.SetFailedCallback(func(r *link.RequestReceipt) {
		nodeRec.enqueue(Event{
			Kind:         EventRequestFailed,
			LinkID:       linkID,
			RequestID:    append([]byte(nil), r.GetRequestID()...),
			Path:         path,
			ErrorMessage: "request failed or timed out",
		})
	})
	return append([]byte(nil), id...), OK
}

func wireLinkData(nodeRec *nodeRecord, lr *linkRecord) {
	if lr.link == nil {
		return
	}
	id := append([]byte(nil), lr.id...)
	lr.link.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		nodeRec.enqueue(Event{
			Kind:    EventLinkData,
			LinkID:  id,
			AppData: append([]byte(nil), data...),
		})
	})
	_ = lr.link.SetResourceStrategy(link.AcceptAll)
	lr.link.SetResourceStartedCallback(func(_ any) {
		nodeRec.enqueue(Event{
			Kind:   EventResourceStarted,
			LinkID: append([]byte(nil), id...),
		})
	})
	lr.link.SetResourceConcludedCallback(func(v any) {
		ev := Event{
			Kind:   EventResourceConcluded,
			LinkID: append([]byte(nil), id...),
		}
		switch r := v.(type) {
		case link.IncomingResource:
			ev.AppData = append([]byte(nil), r.Data...)
			if name, ok := resourceName(r.Metadata); ok {
				ev.Path = name
			}
			if len(r.Hash) > 0 {
				ev.RequestID = append([]byte(nil), r.Hash...)
			}
		case []byte:
			ev.AppData = append([]byte(nil), r...)
		}
		nodeRec.enqueue(ev)
	})
}

func resourceName(meta map[string]any) (string, bool) {
	if meta == nil {
		return "", false
	}
	raw, ok := meta["name"]
	if !ok {
		return "", false
	}
	switch v := raw.(type) {
	case string:
		return v, v != ""
	case []byte:
		return string(v), len(v) > 0
	default:
		return "", false
	}
}

// decodeLinkRequestPayload turns pre-packed msgpack maps into native maps so
// Link.Request packs [path, timeout, dict] once. Raw bytes that are not a map
// stay as []byte (NomadNet page fetches with empty/no request data).
func decodeLinkRequestPayload(data []byte) any {
	if len(data) == 0 {
		return nil
	}
	var decoded any
	if err := msgpack.Unmarshal(data, &decoded); err != nil {
		return data
	}
	switch decoded.(type) {
	case map[string]any, map[any]any:
		return decoded
	default:
		return data
	}
}
