// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"encoding/hex"
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/announce"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/reticulumconfig"
)

type announceHandler struct {
	rec *nodeRecord
}

func (h *announceHandler) AspectFilter() []string { return []string{"*"} }

func (h *announceHandler) ReceivePathResponses() bool { return true }

func (h *announceHandler) ReceivedAnnounce(destHash []byte, announcedIdentity any, appData []byte, hops uint8) error {
	ev := Event{
		Kind:            EventAnnounce,
		DestinationHash: append([]byte(nil), destHash...),
		AppData:         append([]byte(nil), appData...),
		Hops:            hops,
	}
	if id, ok := announcedIdentity.(*identity.Identity); ok {
		if h, err := hex.DecodeString(id.GetHexHash()); err == nil {
			ev.IdentityHash = h
		}
	}
	h.rec.enqueue(ev)
	return nil
}

// NodeCreate loads configuration and constructs a node handle.
// An empty configPath uses in-memory defaults.
func NodeCreate(configPath string) (uint64, int) {
	var cfg *common.ReticulumConfig
	var err error
	if configPath == "" {
		cfg = common.DefaultConfig()
		// In-memory defaults for embedders: no shared-instance bind.
		cfg.ShareInstance = false
	} else {
		if err = validatePath(configPath); err != nil {
			return 0, setLastError(err)
		}
		cfg, err = reticulumconfig.LoadConfig(configPath)
		if err != nil {
			return 0, setLastError(fmt.Errorf("%w: %v", errIO, err))
		}
	}

	n, err := node.New(cfg)
	if err != nil {
		return 0, setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}

	rec := newNodeRecord(n)
	n.Transport().RegisterAnnounceHandler(&announceHandler{rec: rec})

	runtimeMu.Lock()
	id := handles.insert(kindNode, rec)
	runtimeMu.Unlock()
	return id, OK
}

// NodeStart starts transport and configured interfaces.
func NodeStart(nodeHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if rec.started {
		return OK
	}
	if err := rec.node.Start(); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	rec.started = true
	return OK
}

// NodeStop stops transport and interfaces without destroying the handle.
func NodeStop(nodeHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if !rec.started {
		return OK
	}
	if err := rec.node.Stop(); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	rec.started = false
	return OK
}

// NodeDestroy tears down a node and invalidates its handle.
func NodeDestroy(nodeHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if rec.started {
		_ = rec.node.Stop()
		rec.started = false
	}
	rec.queue.close()

	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if !handles.delete(nodeHandle) {
		return setLastError(errInvalidHandle)
	}
	return OK
}

// NodeSetIdentity attaches an identity to the node transport.
func NodeSetIdentity(nodeHandle, identityHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	identRec, err := identityByHandle(identityHandle)
	if err != nil {
		return setLastError(err)
	}
	rec.identity = identRec.identity
	rec.node.Transport().SetIdentity(identRec.identity)
	return OK
}

// PathRequest asks the transport to resolve a path to destHash.
func PathRequest(nodeHandle uint64, destHash []byte) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if len(destHash) != identity.TruncatedHashLength/8 {
		return setLastError(errInvalidArg)
	}
	if !rec.started {
		return setLastError(errState)
	}
	_ = rec.node.Transport().RequestPath(destHash, "", nil, false)
	return OK
}

// EventPoll waits up to timeout for the next event on nodeHandle.
func EventPoll(nodeHandle uint64, timeout time.Duration) (Event, int) {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return Event{}, setLastError(err)
	}
	ev, err := rec.queue.poll(timeout)
	if err != nil {
		return Event{}, setLastError(err)
	}
	return ev, OK
}

// CopyEventField copies one event byte field into dst and reports truncation.
func CopyEventField(dst, src []byte) (written int, truncated bool) {
	if len(src) == 0 {
		return 0, false
	}
	n := copy(dst, src)
	return n, n < len(src)
}

// DecodeHexHash decodes a 32-character hex destination or identity hash.
func DecodeHexHash(hexHash string) ([]byte, error) {
	b, err := hex.DecodeString(hexHash)
	if err != nil || len(b) != identity.TruncatedHashLength/8 {
		return nil, errInvalidArg
	}
	return b, nil
}

var _ announce.Handler = (*announceHandler)(nil)
