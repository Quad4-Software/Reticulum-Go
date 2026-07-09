// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

// Event kinds delivered through EventPoll.
const (
	EventAnnounce        = 1
	EventLinkEstablished = 2
	EventLinkFailed      = 3
	EventLinkData        = 4
	EventLinkClosed      = 5
)

// Event is one pollable librns notification.
type Event struct {
	Kind            int
	LinkID          []byte
	DestinationHash []byte
	IdentityHash    []byte
	AppData         []byte
	Hops            uint8
	ErrorMessage    string
}
