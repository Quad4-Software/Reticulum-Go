// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"encoding/hex"
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
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

	hash := append([]byte(nil), dest.GetHash()...)
	runtimeMu.Lock()
	destHandle := handles.insert(kindDestination, &destinationRecord{
		destination: dest,
		nodeID:      nodeHandle,
		hash:        hash,
	})
	nodeRec.destinations[destHandle] = dest
	runtimeMu.Unlock()
	return destHandle, OK
}

// DestinationRegisterRequestHandler bridges path requests to EventRequestIncoming.
// The host must call RequestRespond or RequestRespondFile with the same request id.
func DestinationRegisterRequestHandler(destHandle uint64, path string) int {
	if path == "" {
		return setLastError(errInvalidArg)
	}
	destRec, err := destinationByHandle(destHandle)
	if err != nil {
		return setLastError(err)
	}
	nodeRec, err := nodeByHandle(destRec.nodeID)
	if err != nil {
		return setLastError(err)
	}
	destHash := append([]byte(nil), destRec.hash...)
	if err := destRec.destination.RegisterRequestHandlerAny(path, func(p string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) any {
		_ = requestedAt
		requestIDHex := hex.EncodeToString(requestID)
		ev := Event{
			Kind:            EventRequestIncoming,
			DestinationHash: append([]byte(nil), destHash...),
			LinkID:          append([]byte(nil), linkID...),
			RequestID:       append([]byte(nil), requestID...),
			Path:            p,
			AppData:         append([]byte(nil), data...),
		}
		if remoteIdentity != nil {
			if h, err := hex.DecodeString(remoteIdentity.GetHexHash()); err == nil {
				ev.IdentityHash = h
			}
		}
		ch := nodeRec.awaitResponse(requestIDHex)
		nodeRec.enqueue(ev)
		select {
		case resp := <-ch:
			return resp
		case <-time.After(requestResponseTimeout):
			nodeRec.forgetResponse(requestIDHex)
			return nil
		}
	}, destination.AllowAll, nil); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	return OK
}

// RequestRespond delivers a response for a pending EventRequestIncoming.
func RequestRespond(nodeHandle uint64, requestID, data []byte) int {
	if len(requestID) == 0 {
		return setLastError(errInvalidArg)
	}
	nodeRec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if !nodeRec.deliverResponse(hex.EncodeToString(requestID), append([]byte(nil), data...)) {
		return setLastError(errNotFound)
	}
	return OK
}

// RequestRespondFile delivers a NomadNet-style file response [filename, content].
// Oversized payloads transfer as link resources automatically.
func RequestRespondFile(nodeHandle uint64, requestID []byte, filename string, data []byte) int {
	if len(requestID) == 0 || filename == "" {
		return setLastError(errInvalidArg)
	}
	nodeRec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	payload := []any{filename, append([]byte(nil), data...)}
	if !nodeRec.deliverResponse(hex.EncodeToString(requestID), payload) {
		return setLastError(errNotFound)
	}
	return OK
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
		lr := &linkRecord{link: lnk, id: append([]byte(nil), id...), nodeID: nodeRec.handle, established: true}
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
