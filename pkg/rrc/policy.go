// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import "git.quad4.io/reticulum/Reticulum-Go/pkg/link"

// HubPolicy is optional hub-side policy used by gorrcd.
// A nil policy keeps library defaults (open rooms, no slash commands).
type HubPolicy interface {
	// OnLink is called when a new inbound link is accepted.
	OnLink(lnk *link.Link)
	// OnIdentified is called after the peer identity is known. A non-nil error disconnects the peer.
	OnIdentified(peer []byte) error
	// AfterWelcome is called after WELCOME is sent.
	AfterWelcome(peer []byte)
	// AllowJoin decides whether JOIN is accepted. A non-nil error is sent as ERROR.
	AllowJoin(peer []byte, room string, body any) error
	// AfterJoin is called after membership is recorded.
	AfterJoin(peer []byte, room string)
	// AfterPart is called after membership is removed.
	AfterPart(peer []byte, room string)
	// AllowContent decides whether MSG, NOTICE, or ACTION is relayed.
	AllowContent(peer []byte, env *Envelope) error
	// Intercept handles slash commands. True means the envelope must not be relayed.
	Intercept(peer []byte, env *Envelope) bool
	// OnPong is called for a client PONG after welcome.
	OnPong(peer []byte)
	// OnResourceEnvelope is called after type 50 metadata validates.
	OnResourceEnvelope(peer []byte, env *Envelope) error
}

// PeerInfo is a connected peer snapshot for operator lookups.
type PeerInfo struct {
	Hash []byte
	Nick string
}
