// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"bytes"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdversarial_ForwardedSenderIsAuthenticatedPeer(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}

	m := newTestMesh(t, 42810, HubConfig{
		Name: "adv-hub",
		Limits: HubLimits{
			MaxNickBytes:           8,
			MaxMsgBodyBytes:        64,
			RateLimitMsgsPerMinute: 120,
		},
	})

	got := make(chan *Envelope, 1)
	joinedA := make(chan struct{}, 1)
	joinedB := make(chan struct{}, 1)

	a := dialMeshClient(t, m, 'A', ClientConfig{
		Nick: "a",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#adv" {
					select {
					case joinedA <- struct{}{}:
					default:
					}
				}
			},
		},
	})
	b := dialMeshClient(t, m, 'B', ClientConfig{
		Nick: "b",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#adv" {
					select {
					case joinedB <- struct{}{}:
					default:
					}
				}
			},
			OnMsg: func(env *Envelope) {
				select {
				case got <- env:
				default:
				}
			},
		},
	})

	if err := a.Join("#adv"); err != nil {
		t.Fatal(err)
	}
	if err := b.Join("#adv"); err != nil {
		t.Fatal(err)
	}
	waitJoined(t, joinedA, "A")
	waitJoined(t, joinedB, "B")

	fakeSender := bytes.Repeat([]byte{0xee}, IdentityLength)
	env, err := NewEnvelope(TypeMsg, fakeSender)
	if err != nil {
		t.Fatal(err)
	}
	env.Room = "#adv"
	env.HasRoom = true
	env.Body = "spoofed"
	env.HasBody = true
	if err := a.sess.sendEnvelope(env); err != nil {
		t.Fatal(err)
	}

	select {
	case fwd := <-got:
		if bytes.Equal(fwd.Sender, fakeSender) {
			t.Fatal("hub forwarded client-claimed Sender (spoof succeeded)")
		}
		if !bytes.Equal(fwd.Sender, a.sender) {
			t.Fatalf("forwarded sender=%x want peer %x", fwd.Sender, a.sender)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for forwarded MSG")
	}
}

func TestAdversarial_PostWelcomeOversizedNickRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}

	m := newTestMesh(t, 42820, HubConfig{
		Limits: HubLimits{MaxNickBytes: 4, MaxMsgBodyBytes: 64, RateLimitMsgsPerMinute: 60},
	})
	errCh := make(chan string, 1)
	joined := make(chan struct{}, 1)
	c := dialMeshClient(t, m, 'A', ClientConfig{
		Nick: "ok",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#n" {
					select {
					case joined <- struct{}{}:
					default:
					}
				}
			},
			OnError: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case errCh <- s:
					default:
					}
				}
			},
		},
	})
	if err := c.Join("#n"); err != nil {
		t.Fatal(err)
	}
	waitJoined(t, joined, "A")

	c.SetNick(strings.Repeat("Z", 32))
	if err := c.SendMsg("#n", "hi"); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-errCh:
		if !strings.Contains(strings.ToLower(msg), "nick") {
			t.Fatalf("error body=%q", msg)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("expected nickname too long ERROR")
	}
}

func TestAdversarial_ControlOnlyNickNotForwardedRaw(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}

	m := newTestMesh(t, 42830, HubConfig{
		Limits: HubLimits{MaxNickBytes: 16, MaxMsgBodyBytes: 64, RateLimitMsgsPerMinute: 60},
	})
	got := make(chan *Envelope, 1)
	joinedA := make(chan struct{}, 1)
	joinedB := make(chan struct{}, 1)
	a := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#c" {
					select {
					case joinedA <- struct{}{}:
					default:
					}
				}
			},
		},
	})
	b := dialMeshClient(t, m, 'B', ClientConfig{
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#c" {
					select {
					case joinedB <- struct{}{}:
					default:
					}
				}
			},
			OnMsg: func(env *Envelope) { got <- env },
		},
	})
	if err := a.Join("#c"); err != nil {
		t.Fatal(err)
	}
	if err := b.Join("#c"); err != nil {
		t.Fatal(err)
	}
	waitJoined(t, joinedA, "A")
	waitJoined(t, joinedB, "B")

	env := mustEnvelope(t, TypeMsg, a.sender)
	env.Room = "#c"
	env.HasRoom = true
	env.Body = "x"
	env.HasBody = true
	env.Nick = "\n\r\x00"
	env.HasNick = true
	if err := a.sess.sendEnvelope(env); err != nil {
		t.Fatal(err)
	}

	select {
	case fwd := <-got:
		if fwd.HasNick && strings.ContainsAny(fwd.Nick, "\n\r\x00") {
			t.Fatalf("raw control nick forwarded: %q", fwd.Nick)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for forwarded MSG")
	}
}

func TestAdversarial_StructuredBodyForwarded(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}

	m := newTestMesh(t, 42840, HubConfig{
		Limits: HubLimits{MaxMsgBodyBytes: 128, RateLimitMsgsPerMinute: 60},
	})
	got := make(chan any, 1)
	joinedA := make(chan struct{}, 1)
	joinedB := make(chan struct{}, 1)
	a := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#b" {
					select {
					case joinedA <- struct{}{}:
					default:
					}
				}
			},
		},
	})
	b := dialMeshClient(t, m, 'B', ClientConfig{
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#b" {
					select {
					case joinedB <- struct{}{}:
					default:
					}
				}
			},
			OnMsg: func(env *Envelope) {
				select {
				case got <- env.Body:
				default:
				}
			},
		},
	})
	if err := a.Join("#b"); err != nil {
		t.Fatal(err)
	}
	if err := b.Join("#b"); err != nil {
		t.Fatal(err)
	}
	waitJoined(t, joinedA, "A")
	waitJoined(t, joinedB, "B")

	env := mustEnvelope(t, TypeMsg, a.sender)
	env.Room = "#b"
	env.HasRoom = true
	env.Body = map[uint64]any{0: true, 1: "x"}
	env.HasBody = true
	if err := a.sess.sendEnvelope(env); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-got:
		raw, err := coerceUintMap(body)
		if err != nil {
			t.Fatalf("structured body not forwarded as map: %#v", body)
		}
		if v, ok := raw[0].(bool); !ok || !v {
			t.Fatalf("body=%#v", body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for structured MSG")
	}
}

func TestAdversarial_ApplyInboundNickUnit(t *testing.T) {
	h := &Hub{cfg: HubConfig{Limits: HubLimits{MaxNickBytes: 4}}}
	h.cfg.applyDefaults()
	p := &hubPeer{sess: &session{}}
	if err := h.applyInboundNick(p, "abcd"); err != nil {
		t.Fatal(err)
	}
	if err := h.applyInboundNick(p, "abcde"); err != ErrNickTooLong {
		t.Fatalf("err=%v", err)
	}
	if err := h.applyInboundNick(p, "\n\n"); err != nil {
		t.Fatal(err)
	}
	if p.sess.getNick() != "abcd" {
		t.Fatalf("nick mutated to %q", p.sess.getNick())
	}
}

func TestAdversarial_DropPeerIfIgnoresStale(t *testing.T) {
	h := &Hub{
		peers: map[peerID]*hubPeer{},
		rooms: map[string]map[peerID]struct{}{},
	}
	hash := bytes.Repeat([]byte{0x01}, IdentityLength)
	old := &hubPeer{peerHash: hash, rooms: map[string]struct{}{}}
	neu := &hubPeer{peerHash: hash, rooms: map[string]struct{}{}}
	key := peerKey(hash)
	h.peers[key] = neu
	h.dropPeerIf(old)
	if h.peers[key] != neu {
		t.Fatal("stale close removed replacement peer")
	}
	h.dropPeerIf(neu)
	if _, ok := h.peers[key]; ok {
		t.Fatal("current peer not dropped")
	}
}

func TestAdversarial_DirectNoticeSenderIsAuthenticatedPeer(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}

	m := newTestMesh(t, 42850, HubConfig{
		Limits: HubLimits{MaxMsgBodyBytes: 64, RateLimitMsgsPerMinute: 60},
	})
	got := make(chan *Envelope, 1)
	joinedA := make(chan struct{}, 1)
	a := dialMeshClient(t, m, 'A', ClientConfig{
		Nick: "a",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#d" {
					select {
					case joinedA <- struct{}{}:
					default:
					}
				}
			},
		},
	})
	b := dialMeshClient(t, m, 'B', ClientConfig{
		Nick: "b",
		Handlers: ClientHandlers{
			OnNotice: func(env *Envelope) {
				select {
				case got <- env:
				default:
				}
			},
		},
	})
	if err := a.Join("#d"); err != nil {
		t.Fatal(err)
	}
	waitJoined(t, joinedA, "A")

	fake := bytes.Repeat([]byte{0xee}, IdentityLength)
	env := mustEnvelope(t, TypeNotice, fake)
	env.Destination = append([]byte(nil), b.sender...)
	env.HasDestination = true
	env.Body = "secret"
	env.HasBody = true
	if err := a.sess.sendEnvelope(env); err != nil {
		t.Fatal(err)
	}

	select {
	case n := <-got:
		if bytes.Equal(n.Sender, fake) {
			t.Fatal("direct notice forwarded claimed sender")
		}
		if !bytes.Equal(n.Sender, a.sender) {
			t.Fatalf("sender=%x want %x", n.Sender, a.sender)
		}
		if !n.HasDestination || !bytes.Equal(n.Destination, b.sender) {
			t.Fatalf("dest=%x", n.Destination)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for direct NOTICE")
	}
}

func TestAdversarial_OversizedStructuredBodyRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 42860, HubConfig{
		Limits: HubLimits{MaxMsgBodyBytes: 8, RateLimitMsgsPerMinute: 60},
	})
	errCh := make(chan string, 1)
	joined := make(chan struct{}, 1)
	c := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#z" {
					select {
					case joined <- struct{}{}:
					default:
					}
				}
			},
			OnError: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case errCh <- s:
					default:
					}
				}
			},
		},
	})
	if err := c.Join("#z"); err != nil {
		t.Fatal(err)
	}
	waitJoined(t, joined, "A")
	env := mustEnvelope(t, TypeMsg, c.sender)
	env.Room = "#z"
	env.HasRoom = true
	env.Body = map[uint64]any{0: strings.Repeat("x", 64)}
	env.HasBody = true
	if err := c.sess.sendEnvelope(env); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-errCh:
		if !strings.Contains(strings.ToLower(msg), "large") {
			t.Fatalf("error=%q", msg)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("expected oversized body ERROR")
	}
}

func TestAdversarial_ResourceEnvelopeDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 42870, HubConfig{
		EnableResourceTransfer: false,
		Limits:                 HubLimits{RateLimitMsgsPerMinute: 60},
	})
	errCh := make(chan string, 1)
	c := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnError: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case errCh <- s:
					default:
					}
				}
			},
		},
	})
	body := &ResourceEnvelopeBody{ID: []byte{1, 2, 3}, HasID: true, Kind: ResourceKindBlob, HasKind: true}
	if err := c.SendResourceEnvelope("", body); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-errCh:
		if !strings.Contains(strings.ToLower(msg), "resource") {
			t.Fatalf("error=%q", msg)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("expected resource transfer disabled ERROR")
	}
}

func TestAdversarial_PingEchoesPongAfterWelcome(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 42880, HubConfig{
		Limits: HubLimits{RateLimitMsgsPerMinute: 60},
	})
	pong := make(chan any, 1)
	c := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnPong: func(env *Envelope) {
				select {
				case pong <- env.Body:
				default:
				}
			},
		},
	})
	if err := c.Ping("latency"); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-pong:
		s, ok := BodyAsString(body)
		if !ok || s != "latency" {
			t.Fatalf("pong body=%#v", body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("expected PONG")
	}
}

func TestAdversarial_SenderReceivesOwnRoomMsg(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 42890, HubConfig{
		Limits: HubLimits{MaxMsgBodyBytes: 64, RateLimitMsgsPerMinute: 60},
	})
	got := make(chan string, 1)
	joined := make(chan struct{}, 1)
	c := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#echo" {
					select {
					case joined <- struct{}{}:
					default:
					}
				}
			},
			OnMsg: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case got <- s:
					default:
					}
				}
			},
		},
	})
	if err := c.Join("#echo"); err != nil {
		t.Fatal(err)
	}
	waitJoined(t, joined, "A")
	if err := c.SendMsg("#echo", "own-msg"); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-got:
		if body != "own-msg" {
			t.Fatalf("body=%q", body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("sender did not receive own MSG")
	}
}

func TestAdversarial_ResourceEnvelopeEnabledAccepted(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	got := make(chan string, 1)
	m := newTestMesh(t, 42900, HubConfig{
		EnableResourceTransfer: true,
		MaxResourceBytes:       1024,
		Limits:                 HubLimits{RateLimitMsgsPerMinute: 60},
		Handlers: HubHandlers{
			OnResource: func(_ []byte, env *Envelope) {
				body, reason := ValidateResourceEnvelopeBody(env.Body)
				if reason != "" {
					return
				}
				select {
				case got <- body.Kind:
				default:
				}
			},
		},
	})
	c := dialMeshClient(t, m, 'A', ClientConfig{})
	body := &ResourceEnvelopeBody{
		ID: []byte{9, 8, 7, 6, 5, 4, 3, 2}, HasID: true,
		Kind: ResourceKindBlob, HasKind: true,
		Size: 16, HasSize: true,
	}
	if err := c.SendResourceEnvelope("", body); err != nil {
		t.Fatal(err)
	}
	select {
	case kind := <-got:
		if kind != ResourceKindBlob {
			t.Fatalf("kind=%q", kind)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("expected resource envelope at hub")
	}
}

func TestAdversarial_InvalidResourceEnvelopeRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	errCh := make(chan string, 1)
	m := newTestMesh(t, 42910, HubConfig{
		EnableResourceTransfer: true,
		Limits:                 HubLimits{RateLimitMsgsPerMinute: 60},
	})
	c := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnError: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case errCh <- s:
					default:
					}
				}
			},
		},
	})
	if err := c.SendResourceEnvelope("", &ResourceEnvelopeBody{ID: []byte{1}, HasID: true, Kind: ResourceKindBlob, HasKind: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-errCh:
		if !strings.Contains(strings.ToLower(msg), "size") {
			t.Fatalf("error=%q", msg)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("expected invalid resource size ERROR")
	}
}

func TestAdversarial_HelloRequiredBeforeJoin(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 42912, HubConfig{
		Limits: HubLimits{RateLimitMsgsPerMinute: 60},
	})
	errCh := make(chan string, 1)
	sess := dialMeshPreHello(t, m, 'A', func(env *Envelope) {
		if env.Type == TypeError {
			if s, ok := BodyAsString(env.Body); ok {
				select {
				case errCh <- s:
				default:
				}
			}
		}
	})
	if err := sess.sendType(TypeJoin, "#lobby", nil, "early"); err != nil {
		t.Fatal(err)
	}
	waitError(t, errCh, "hello")
}

func TestAdversarial_MsgWithoutJoinRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 42922, HubConfig{
		Limits: HubLimits{MaxMsgBodyBytes: 64, RateLimitMsgsPerMinute: 60},
	})
	errCh := make(chan string, 1)
	c := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnError: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case errCh <- s:
					default:
					}
				}
			},
		},
	})
	if err := c.sess.sendType(TypeMsg, "#ghost", "nope", c.sess.getNick()); err != nil {
		t.Fatal(err)
	}
	waitError(t, errCh, "member")
}

func TestAdversarial_RateLimitTriggersError(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 42932, HubConfig{
		Limits: HubLimits{RateLimitMsgsPerMinute: 2, MaxMsgBodyBytes: 64},
	})
	errCh := make(chan string, 1)
	joined := make(chan struct{}, 1)
	c := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#rate" {
					select {
					case joined <- struct{}{}:
					default:
					}
				}
			},
			OnError: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case errCh <- s:
					default:
					}
				}
			},
		},
	})
	if err := c.Join("#rate"); err != nil {
		t.Fatal(err)
	}
	waitJoined(t, joined, "A")
	for range 2 {
		if err := c.SendMsg("#rate", "x"); err != nil {
			t.Fatal(err)
		}
	}
	waitError(t, errCh, "rate")
}

func TestAdversarial_RoomNameTooLongRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 42942, HubConfig{
		Limits: HubLimits{MaxRoomNameBytes: 8, RateLimitMsgsPerMinute: 60},
	})
	errCh := make(chan string, 1)
	c := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnError: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case errCh <- s:
					default:
					}
				}
			},
		},
	})
	longRoom := "#" + strings.Repeat("x", 64)
	if err := c.Join(longRoom); err != nil {
		t.Fatal(err)
	}
	waitError(t, errCh, "room")
}

func TestAdversarial_RoomLimitPerSession(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 42952, HubConfig{
		Limits: HubLimits{MaxRoomsPerSession: 2, RateLimitMsgsPerMinute: 60},
	})
	errCh := make(chan string, 1)
	joined := make(chan string, 2)
	c := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				select {
				case joined <- room:
				default:
				}
			},
			OnError: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case errCh <- s:
					default:
					}
				}
			},
		},
	})
	if err := c.Join("#one"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-joined:
	case <-time.After(5 * time.Second):
		t.Fatal("join #one timeout")
	}
	if err := c.Join("#two"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-joined:
	case <-time.After(5 * time.Second):
		t.Fatal("join #two timeout")
	}
	if err := c.Join("#three"); err != nil {
		t.Fatal(err)
	}
	waitError(t, errCh, "room limit")
}

func TestAdversarial_DirectNoticeWithRoomRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 42962, HubConfig{
		Limits: HubLimits{MaxMsgBodyBytes: 64, RateLimitMsgsPerMinute: 60},
	})
	errCh := make(chan string, 1)
	a := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnError: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case errCh <- s:
					default:
					}
				}
			},
		},
	})
	b := dialMeshClient(t, m, 'B', ClientConfig{})
	env := mustEnvelope(t, TypeNotice, a.sender)
	env.Room = "#d"
	env.HasRoom = true
	env.Destination = append([]byte(nil), b.sender...)
	env.HasDestination = true
	env.Body = "bad"
	env.HasBody = true
	if err := a.sess.sendEnvelope(env); err != nil {
		t.Fatal(err)
	}
	waitError(t, errCh, "direct notice")
}

func TestAdversarial_DirectNoticeUnknownDestRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 42972, HubConfig{
		Limits: HubLimits{MaxMsgBodyBytes: 64, RateLimitMsgsPerMinute: 60},
	})
	errCh := make(chan string, 1)
	a := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnError: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case errCh <- s:
					default:
					}
				}
			},
		},
	})
	ghost := bytes.Repeat([]byte{0xde}, IdentityLength)
	env := mustEnvelope(t, TypeNotice, a.sender)
	env.Destination = ghost
	env.HasDestination = true
	env.Body = "dm"
	env.HasBody = true
	if err := a.sess.sendEnvelope(env); err != nil {
		t.Fatal(err)
	}
	waitError(t, errCh, "destination")
}

func TestAdversarial_ActionSenderIsAuthenticatedPeer(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 42982, HubConfig{
		Limits: HubLimits{MaxMsgBodyBytes: 64, RateLimitMsgsPerMinute: 60},
	})
	got := make(chan *Envelope, 1)
	joinedA := make(chan struct{}, 1)
	joinedB := make(chan struct{}, 1)
	a := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#act" {
					select {
					case joinedA <- struct{}{}:
					default:
					}
				}
			},
		},
	})
	b := dialMeshClient(t, m, 'B', ClientConfig{
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#act" {
					select {
					case joinedB <- struct{}{}:
					default:
					}
				}
			},
			OnAction: func(env *Envelope) {
				select {
				case got <- env:
				default:
				}
			},
		},
	})
	if err := a.Join("#act"); err != nil {
		t.Fatal(err)
	}
	if err := b.Join("#act"); err != nil {
		t.Fatal(err)
	}
	waitJoined(t, joinedA, "A")
	waitJoined(t, joinedB, "B")
	fake := bytes.Repeat([]byte{0xfa}, IdentityLength)
	env := mustEnvelope(t, TypeAction, fake)
	env.Room = "#act"
	env.HasRoom = true
	env.Body = "waves"
	env.HasBody = true
	if err := a.sess.sendEnvelope(env); err != nil {
		t.Fatal(err)
	}
	select {
	case fwd := <-got:
		if bytes.Equal(fwd.Sender, fake) {
			t.Fatal("action sender spoofed")
		}
		if !bytes.Equal(fwd.Sender, a.sender) {
			t.Fatalf("sender=%x", fwd.Sender)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for ACTION")
	}
}

func TestAdversarial_PolicyDenyJoin(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 42992, HubConfig{
		Policy: &denyPolicy{joinErr: errors.New("access denied")},
		Limits: HubLimits{RateLimitMsgsPerMinute: 60},
	})
	errCh := make(chan string, 1)
	c := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnError: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case errCh <- s:
					default:
					}
				}
			},
		},
	})
	if err := c.Join("#locked"); err != nil {
		t.Fatal(err)
	}
	waitError(t, errCh, "denied")
}

func TestAdversarial_PolicyDenyContent(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 43002, HubConfig{
		Policy: &denyPolicy{contentErr: errors.New("content blocked")},
		Limits: HubLimits{MaxMsgBodyBytes: 64, RateLimitMsgsPerMinute: 60},
	})
	errCh := make(chan string, 1)
	joined := make(chan struct{}, 1)
	c := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#pol" {
					select {
					case joined <- struct{}{}:
					default:
					}
				}
			},
			OnError: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case errCh <- s:
					default:
					}
				}
			},
		},
	})
	if err := c.Join("#pol"); err != nil {
		t.Fatal(err)
	}
	waitJoined(t, joined, "A")
	if err := c.SendMsg("#pol", "blocked"); err != nil {
		t.Fatal(err)
	}
	waitError(t, errCh, "blocked")
}

func TestAdversarial_ResourceOversizeRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	errCh := make(chan string, 1)
	m := newTestMesh(t, 43012, HubConfig{
		EnableResourceTransfer: true,
		MaxResourceBytes:       64,
		Limits:                 HubLimits{RateLimitMsgsPerMinute: 60},
	})
	c := dialMeshClient(t, m, 'A', ClientConfig{
		Handlers: ClientHandlers{
			OnError: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case errCh <- s:
					default:
					}
				}
			},
		},
	})
	body := &ResourceEnvelopeBody{
		ID: []byte{1, 2, 3, 4}, HasID: true,
		Kind: ResourceKindBlob, HasKind: true,
		Size: 128, HasSize: true,
	}
	if err := c.SendResourceEnvelope("", body); err != nil {
		t.Fatal(err)
	}
	waitError(t, errCh, "large")
}

type identRejectPolicy struct {
	denyPolicy
}

func (identRejectPolicy) OnIdentified([]byte) error {
	return errors.New("banned identity")
}

func TestAdversarial_IdentRejectBlocksSession(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	m := newTestMesh(t, 43032, HubConfig{
		Policy: &identRejectPolicy{},
		Limits: HubLimits{RateLimitMsgsPerMinute: 60},
	})
	_, err := Dial(m.trA, m.idA, m.hubHash, ClientConfig{DialTimeout: 5 * time.Second, WelcomeTimeout: 2 * time.Second})
	if err == nil {
		t.Fatal("banned identity must not complete dial")
	}
}

func TestAdversarial_BadPacketRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("adversarial mesh skipped in -short")
	}
	var badCount atomic.Int32
	m := newTestMesh(t, 43022, HubConfig{
		Limits: HubLimits{RateLimitMsgsPerMinute: 60},
		OnBadPacket: func(err error) {
			if err == nil {
				t.Error("nil bad packet error")
			}
			badCount.Add(1)
		},
	})
	c := dialMeshClient(t, m, 'A', ClientConfig{})
	if err := c.sess.lnk.SendPacket([]byte{0xff, 0xfe, 0xfd}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for badCount.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("hub did not report bad packet")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
