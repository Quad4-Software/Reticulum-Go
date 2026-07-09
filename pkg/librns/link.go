// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"fmt"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/packet"
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

	lr := &linkRecord{}
	established := func(l *link.Link) {
		lr.link = l
		lr.id = append([]byte(nil), l.GetLinkID()...)
		lr.established = true
		wireLinkData(nodeRec, lr)
		nodeRec.enqueue(Event{
			Kind:         EventLinkEstablished,
			LinkID:       append([]byte(nil), lr.id...),
			IdentityHash: remoteIdentityBytes(l),
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
		return 0, setLastError(fmt.Errorf("%w: %v", errInternal, err))
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
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
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
}
