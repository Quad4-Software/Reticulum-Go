// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"strings"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
)

type stubPolicy struct {
	intercepted bool
}

func (s *stubPolicy) OnLink(*link.Link)                    {}
func (s *stubPolicy) OnIdentified([]byte) error            { return nil }
func (s *stubPolicy) AfterWelcome([]byte)                  {}
func (s *stubPolicy) AllowJoin([]byte, string, any) error  { return nil }
func (s *stubPolicy) AfterJoin([]byte, string)             {}
func (s *stubPolicy) AfterPart([]byte, string)             {}
func (s *stubPolicy) AllowContent([]byte, *Envelope) error { return nil }
func (s *stubPolicy) Intercept(_ []byte, env *Envelope) bool {
	if env != nil {
		if text, ok := BodyAsString(env.Body); ok && strings.HasPrefix(text, "/") {
			s.intercepted = true
			return true
		}
	}
	return false
}
func (s *stubPolicy) OnPong([]byte)                              {}
func (s *stubPolicy) OnResourceEnvelope([]byte, *Envelope) error { return nil }

func TestHubTryInterceptSlash(t *testing.T) {
	pol := &stubPolicy{}
	h := &Hub{cfg: HubConfig{Policy: pol}}
	p := &hubPeer{peerHash: make([]byte, IdentityLength)}
	env, err := NewEnvelope(TypeMsg, p.peerHash)
	if err != nil {
		t.Fatal(err)
	}
	env.Body = "/stats"
	env.HasBody = true
	if !h.tryIntercept(p, env) || !pol.intercepted {
		t.Fatal("slash command should intercept")
	}
	env.Body = "hello"
	pol.intercepted = false
	if h.tryIntercept(p, env) || pol.intercepted {
		t.Fatal("plain text should not intercept")
	}
}

func TestHubTryInterceptNilPolicy(t *testing.T) {
	h := &Hub{}
	p := &hubPeer{peerHash: make([]byte, IdentityLength)}
	env, err := NewEnvelope(TypeMsg, p.peerHash)
	if err != nil {
		t.Fatal(err)
	}
	env.Body = "/stats"
	env.HasBody = true
	if h.tryIntercept(p, env) {
		t.Fatal("nil policy should not intercept")
	}
}
