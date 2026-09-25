// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/Reticulum-Go/pkg/node"
)

const requestResponseTimeout = 30 * time.Second

type nodeRecord struct {
	handle uint64
	node   *node.Node
	// started and identity are touched from arbitrary embedder threads.
	identity     atomic.Pointer[identity.Identity]
	queue        *eventQueue
	destinations map[uint64]*destination.Destination
	links        map[uint64]*linkRecord
	started      atomic.Bool
	configPath   string

	pendingMu sync.Mutex
	pending   map[string]chan any

	cbMu     sync.Mutex
	callback EventCallback
	cbStop   chan struct{}
	cbDone   chan struct{}
	// inCallback is set while drainEvents is inside the user callback.
	// A reentrant stop (rns_node_destroy or rns_set_event_callback called
	// from the callback on the same goroutine) must not wait for cbDone,
	// or both sides block forever.
	inCallback atomic.Bool
}

type linkRecord struct {
	// link and established are written by transport callbacks on other
	// goroutines. id is written during establishment (the transport assigns
	// linkID inside Establish) and read by FFI threads.
	link        atomic.Pointer[link.Link]
	nodeID      uint64
	established atomic.Bool
	idMu        sync.RWMutex
	id          []byte
}

func (lr *linkRecord) linkIDBytes() []byte {
	lr.idMu.RLock()
	defer lr.idMu.RUnlock()
	return append([]byte(nil), lr.id...)
}

func (lr *linkRecord) setLinkID(id []byte) {
	lr.idMu.Lock()
	lr.id = append([]byte(nil), id...)
	lr.idMu.Unlock()
}

type identityRecord struct {
	identity *identity.Identity
}

type destinationRecord struct {
	destination *destination.Destination
	nodeID      uint64
	hash        []byte
}

var (
	runtimeMu sync.RWMutex
	handles   = newHandleTable()
)

func nodeByHandle(id uint64) (*nodeRecord, error) {
	ref, err := handles.get(id, kindNode)
	if err != nil {
		return nil, err
	}
	return ref.(*nodeRecord), nil
}

func identityByHandle(id uint64) (*identityRecord, error) {
	ref, err := handles.get(id, kindIdentity)
	if err != nil {
		return nil, err
	}
	return ref.(*identityRecord), nil
}

func destinationByHandle(id uint64) (*destinationRecord, error) {
	ref, err := handles.get(id, kindDestination)
	if err != nil {
		return nil, err
	}
	return ref.(*destinationRecord), nil
}

func linkByHandle(id uint64) (*linkRecord, error) {
	ref, err := handles.get(id, kindLink)
	if err != nil {
		return nil, err
	}
	return ref.(*linkRecord), nil
}

func (n *nodeRecord) enqueue(ev Event) {
	if n.queue != nil {
		n.queue.push(ev)
	}
}

func (n *nodeRecord) awaitResponse(requestIDHex string) chan any {
	ch := make(chan any, 1)
	n.pendingMu.Lock()
	if n.pending == nil {
		n.pending = make(map[string]chan any)
	}
	n.pending[requestIDHex] = ch
	n.pendingMu.Unlock()
	return ch
}

func (n *nodeRecord) deliverResponse(requestIDHex string, data any) bool {
	n.pendingMu.Lock()
	ch, ok := n.pending[requestIDHex]
	if ok {
		delete(n.pending, requestIDHex)
	}
	n.pendingMu.Unlock()
	if !ok {
		return false
	}
	ch <- data
	return true
}

func (n *nodeRecord) forgetResponse(requestIDHex string) {
	n.pendingMu.Lock()
	delete(n.pending, requestIDHex)
	n.pendingMu.Unlock()
}

func (n *nodeRecord) stopCallback() {
	n.cbMu.Lock()
	stop := n.cbStop
	done := n.cbDone
	n.callback = nil
	n.cbStop = nil
	n.cbDone = nil
	n.cbMu.Unlock()
	if stop != nil {
		close(stop)
		if !n.inCallback.Load() {
			<-done
		}
	}
}

func newNodeRecord(n *node.Node, configPath string) *nodeRecord {
	return &nodeRecord{
		node:         n,
		configPath:   configPath,
		queue:        newEventQueue(defaultQueueCapacity),
		destinations: make(map[uint64]*destination.Destination),
		links:        make(map[uint64]*linkRecord),
		pending:      make(map[string]chan any),
	}
}
