// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"sync"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/node"
)

type nodeRecord struct {
	node         *node.Node
	identity     *identity.Identity
	queue        *eventQueue
	destinations map[uint64]*destination.Destination
	links        map[uint64]*linkRecord
	started      bool
}

type linkRecord struct {
	link        *link.Link
	id          []byte
	established bool
}

type identityRecord struct {
	identity *identity.Identity
}

type destinationRecord struct {
	destination *destination.Destination
	nodeID      uint64
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

func newNodeRecord(n *node.Node) *nodeRecord {
	return &nodeRecord{
		node:         n,
		queue:        newEventQueue(defaultQueueCapacity),
		destinations: make(map[uint64]*destination.Destination),
		links:        make(map[uint64]*linkRecord),
	}
}
