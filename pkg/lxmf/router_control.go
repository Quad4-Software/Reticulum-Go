// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

func (r *Router) statsGetRequestHandler(path string, data []byte, requestID, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) any {
	_ = path
	_ = data
	_ = requestID
	_ = requestedAt

	remoteIdentity = r.requestRemoteIdentity(linkID, remoteIdentity)
	if remoteIdentity == nil {
		return int(PeerErrorNoIdentity)
	}
	if !r.controlAllowed(remoteIdentity.Hash()) {
		return int(PeerErrorNoAccess)
	}
	return r.compileStats()
}

func (r *Router) peerSyncRequestHandler(path string, data []byte, requestID, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) any {
	_ = path
	_ = requestID
	_ = requestedAt

	remoteIdentity = r.requestRemoteIdentity(linkID, remoteIdentity)
	if remoteIdentity == nil {
		return int(PeerErrorNoIdentity)
	}
	if !r.controlAllowed(remoteIdentity.Hash()) {
		return int(PeerErrorNoAccess)
	}
	if len(data) != DestinationLength {
		return int(PeerErrorInvalidData)
	}
	r.peersMu.RLock()
	peer, ok := r.peers[peerKey(data)]
	r.peersMu.RUnlock()
	if !ok {
		return int(PeerErrorNotFound)
	}
	go peer.sync()
	return true
}

func (r *Router) peerUnpeerRequestHandler(path string, data []byte, requestID, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) any {
	_ = path
	_ = requestID
	_ = requestedAt

	remoteIdentity = r.requestRemoteIdentity(linkID, remoteIdentity)
	if remoteIdentity == nil {
		return int(PeerErrorNoIdentity)
	}
	if !r.controlAllowed(remoteIdentity.Hash()) {
		return int(PeerErrorNoAccess)
	}
	if len(data) != DestinationLength {
		return int(PeerErrorInvalidData)
	}
	r.peersMu.RLock()
	_, ok := r.peers[peerKey(data)]
	r.peersMu.RUnlock()
	if !ok {
		return int(PeerErrorNotFound)
	}
	r.unpeer(data, 0)
	return true
}

func (r *Router) controlAllowed(hash []byte) bool {
	_, ok := r.controlAllow[peerKey(hash)]
	return ok
}
