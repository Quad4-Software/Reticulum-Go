// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

// MessageHandler is called for inbound RRC envelopes on a session.
type MessageHandler func(env *Envelope)

// ClientHandlers holds optional client-side event callbacks.
type ClientHandlers struct {
	OnWelcome  func(body *WelcomeBody, env *Envelope)
	OnJoined   func(room string, members [][]byte, env *Envelope)
	OnParted   func(room string, env *Envelope)
	OnMsg      func(env *Envelope)
	OnNotice   func(env *Envelope)
	OnAction   func(env *Envelope)
	OnError    func(env *Envelope)
	OnPong     func(env *Envelope)
	OnResource func(env *Envelope)
	OnClose    func()
}

// HubHandlers holds optional hub-side session callbacks.
type HubHandlers struct {
	OnHello    func(peer []byte, body *HelloBody, env *Envelope)
	OnJoin     func(peer []byte, room string, env *Envelope)
	OnPart     func(peer []byte, room string, env *Envelope)
	OnMsg      func(peer []byte, env *Envelope)
	OnResource func(peer []byte, env *Envelope)
	OnClose    func(peer []byte)
}
