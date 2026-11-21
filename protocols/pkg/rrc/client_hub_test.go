// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"errors"
	"testing"
)

func TestClientDispatchWelcomeWhileConnected(t *testing.T) {
	sender := make([]byte, IdentityLength)
	env, err := NewEnvelope(TypeWelcome, sender)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{state: ClientConnected, rooms: make(map[string]struct{})}
	welcomeCh := make(chan *Envelope, 1)
	c.dispatch(env, welcomeCh)
	select {
	case got := <-welcomeCh:
		if got.Type != TypeWelcome {
			t.Fatalf("type=%d", got.Type)
		}
	default:
		t.Fatal("welcome dropped before awaiting state")
	}
}

func TestClientDispatchWelcomeIgnoredWhenActive(t *testing.T) {
	sender := make([]byte, IdentityLength)
	env, err := NewEnvelope(TypeWelcome, sender)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{state: ClientActive, rooms: make(map[string]struct{})}
	welcomeCh := make(chan *Envelope, 1)
	c.dispatch(env, welcomeCh)
	select {
	case <-welcomeCh:
		t.Fatal("welcome accepted after session was already active")
	default:
	}
}

func TestClientRefuseBeforeWelcome(t *testing.T) {
	c := &Client{
		state: ClientAwaitingWelcome,
		rooms: make(map[string]struct{}),
	}
	if err := c.Join("#x"); err != ErrNotWelcome {
		t.Fatalf("Join err = %v", err)
	}
	if err := c.SendMsg("#x", "hi"); err != ErrNotWelcome {
		t.Fatalf("SendMsg err = %v", err)
	}
}

func TestClientSendRequiresMembership(t *testing.T) {
	c := &Client{
		state: ClientActive,
		rooms: make(map[string]struct{}),
		sess:  &session{sender: make([]byte, IdentityLength)},
	}
	if err := c.SendMsg("#lobby", "hi"); err != ErrNotMember {
		t.Fatalf("SendMsg err = %v", err)
	}
}

func TestHubConfigDefaults(t *testing.T) {
	cfg := HubConfig{}
	cfg.applyDefaults()
	if cfg.Limits.MaxMsgBodyBytes != DefaultMaxMsgBodyBytes {
		t.Fatalf("limits = %+v", cfg.Limits)
	}
}

func TestHubRateLimit(t *testing.T) {
	h := &Hub{
		cfg: HubConfig{Limits: HubLimits{RateLimitMsgsPerMinute: 2}},
	}
	p := &hubPeer{}
	if !h.allowRate(p) || !h.allowRate(p) {
		t.Fatal("first two should pass")
	}
	if h.allowRate(p) {
		t.Fatal("third should fail")
	}
}

func TestClientLastErrorFromHub(t *testing.T) {
	sender := make([]byte, IdentityLength)
	env, err := NewEnvelope(TypeError, sender)
	if err != nil {
		t.Fatal(err)
	}
	env.Body = "denied"
	env.HasBody = true
	c := &Client{state: ClientActive, rooms: make(map[string]struct{})}
	c.dispatch(env, nil)
	if !errors.Is(c.LastError(), ErrHub) {
		t.Fatalf("last error %v", c.LastError())
	}
}
