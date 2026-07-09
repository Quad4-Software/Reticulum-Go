// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"encoding/hex"
	"fmt"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/link"
)

// DestinationCreate registers an inbound destination on nodeHandle.
func DestinationCreate(nodeHandle, identityHandle uint64, appName string, aspects []string, acceptsLinks bool) (uint64, int) {
	nodeRec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return 0, setLastError(err)
	}
	ident := nodeRec.identity
	if identityHandle != 0 {
		identRec, err := identityByHandle(identityHandle)
		if err != nil {
			return 0, setLastError(err)
		}
		ident = identRec.identity
	}
	if ident == nil {
		return 0, setLastError(errState)
	}
	if appName == "" {
		return 0, setLastError(errInvalidArg)
	}

	dest, err := destination.New(ident, destination.In, destination.Single, appName, nodeRec.node.Transport(), aspects...)
	if err != nil {
		return 0, setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	if acceptsLinks {
		wireInboundLinks(nodeRec, dest)
	}

	runtimeMu.Lock()
	destHandle := handles.insert(kindDestination, &destinationRecord{
		destination: dest,
		nodeID:      nodeHandle,
	})
	nodeRec.destinations[destHandle] = dest
	runtimeMu.Unlock()
	return destHandle, OK
}

// DestinationAnnounce sends an announce for destHandle.
func DestinationAnnounce(destHandle uint64, appData []byte) int {
	destRec, err := destinationByHandle(destHandle)
	if err != nil {
		return setLastError(err)
	}
	if len(appData) > 0 {
		destRec.destination.SetDefaultAppData(appData)
	}
	if err := destRec.destination.Announce(false, nil, nil); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	return OK
}

// DestinationHash returns the 16-byte truncated destination hash.
func DestinationHash(destHandle uint64) ([]byte, int) {
	destRec, err := destinationByHandle(destHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	h := destRec.destination.GetHash()
	out := make([]byte, len(h))
	copy(out, h)
	return out, OK
}

// DestinationDestroy releases a destination handle.
func DestinationDestroy(destHandle uint64) int {
	destRec, err := destinationByHandle(destHandle)
	if err != nil {
		return setLastError(err)
	}
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if nodeRec, nerr := nodeByHandle(destRec.nodeID); nerr == nil {
		delete(nodeRec.destinations, destHandle)
	}
	if !handles.delete(destHandle) {
		return setLastError(errInvalidHandle)
	}
	return OK
}

func wireInboundLinks(nodeRec *nodeRecord, dest *destination.Destination) {
	dest.AcceptsLinks(true)
	dest.SetLinkEstablishedCallback(func(v any) {
		lnk, ok := v.(*link.Link)
		if !ok {
			return
		}
		id := lnk.GetLinkID()
		lr := &linkRecord{link: lnk, id: append([]byte(nil), id...), established: true}
		runtimeMu.Lock()
		linkHandle := handles.insert(kindLink, lr)
		nodeRec.links[linkHandle] = lr
		runtimeMu.Unlock()

		wireLinkData(nodeRec, lr)
		lnk.SetLinkClosedCallback(func(*link.Link) {
			nodeRec.enqueue(Event{Kind: EventLinkClosed, LinkID: append([]byte(nil), id...)})
			runtimeMu.Lock()
			for h, rec := range nodeRec.links {
				if rec == lr {
					delete(nodeRec.links, h)
					handles.delete(h)
					break
				}
			}
			runtimeMu.Unlock()
		})

		nodeRec.enqueue(Event{
			Kind:         EventLinkEstablished,
			LinkID:       append([]byte(nil), id...),
			IdentityHash: remoteIdentityBytes(lnk),
		})
	})
}

func remoteIdentityBytes(lnk *link.Link) []byte {
	if remote := lnk.GetRemoteIdentity(); remote != nil {
		h, _ := hex.DecodeString(remote.GetHexHash())
		return h
	}
	return nil
}
