// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

// Event kinds delivered through EventPoll and optional callbacks.
const (
	EventAnnounce        = 1
	EventLinkEstablished = 2
	EventLinkFailed      = 3
	EventLinkData        = 4
	EventLinkClosed      = 5
	EventRequestIncoming = 6
	EventRequestResponse = 7
	EventRequestFailed   = 8
)

// Event is one pollable librns notification.
type Event struct {
	Kind            int
	LinkID          []byte
	DestinationHash []byte
	IdentityHash    []byte
	RequestID       []byte
	AppData         []byte
	Path            string
	Hops            uint8
	ErrorMessage    string
}

// EventCallback receives a copy of each event when registered on a node.
type EventCallback func(Event)

// PathEntry is one row from the transport path table.
type PathEntry struct {
	Hash      []byte
	Via       []byte
	Hops      uint8
	Interface string
	Timestamp float64
	Expires   float64
}
