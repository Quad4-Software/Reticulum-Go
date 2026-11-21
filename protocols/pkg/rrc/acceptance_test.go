// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// Acceptance criteria mapped to RRC 0.1.3 behavioral requirements.

func TestAcceptance_1_DestinationIsRrcHub(t *testing.T) {
	if AppName != "rrc" || HubAspect != "hub" {
		t.Fatalf("hub destination must be rrc.hub, got %s.%s", AppName, HubAspect)
	}
	if KeyDestination != 8 || TypeResourceEnvelope != 50 {
		t.Fatal("extension assignments")
	}
}

func TestAcceptance_2_EnvelopeCBORUintKeys(t *testing.T) {
	sender := bytes.Repeat([]byte{0x10}, IdentityLength)
	env := mustEnvelope(t, TypeMsg, sender)
	env.Room = "#acc"
	env.HasRoom = true
	env.Body = "ok"
	env.HasBody = true
	raw, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != ProtocolVersion || got.Type != TypeMsg {
		t.Fatal("version/type")
	}
	if len(got.MsgID) != MessageIDLength || len(got.Sender) != IdentityLength {
		t.Fatal("fixed field lengths")
	}
}

func TestAcceptance_3_UnknownKeysIgnored(t *testing.T) {
	sender := bytes.Repeat([]byte{0x11}, IdentityLength)
	env := mustEnvelope(t, TypePing, sender)
	raw, err := mustMarshalWithExtra(*env, 200, "extension")
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != TypePing {
		t.Fatal("unknown keys must not break decode")
	}
}

func TestAcceptance_4_HelloWelcomeGate(t *testing.T) {
	if testing.Short() {
		t.Skip("acceptance live gate skipped in -short")
	}
	m := newTestMesh(t, 42660, HubConfig{Name: "accept-hub"})
	c := dialMeshClient(t, m, 'A', ClientConfig{Nick: "acc"})
	if c.State() != ClientActive {
		t.Fatalf("after Dial state=%v want Active", c.State())
	}
	if c.Welcome() == nil {
		t.Fatal("WELCOME required before Active")
	}
	joined := make(chan struct{}, 1)
	c2 := dialMeshClient(t, m, 'B', ClientConfig{
		Nick: "acc2",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#acc" {
					select {
					case joined <- struct{}{}:
					default:
					}
				}
			},
		},
	})
	if err := c2.Join("#acc"); err != nil {
		t.Fatal(err)
	}
	waitJoined(t, joined, "B")
	if err := c2.SendMsg("#acc", "gated"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m.hub.PeerCount() >= 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("peer count=%d", m.hub.PeerCount())
}

func TestAcceptance_RefuseOpsBeforeWelcome(t *testing.T) {
	c := &Client{state: ClientAwaitingWelcome, rooms: map[string]struct{}{}}
	if err := c.Join("#x"); err != ErrNotWelcome {
		t.Fatalf("err=%v", err)
	}
	if err := c.Ping(nil); err != ErrNotWelcome {
		t.Fatalf("err=%v", err)
	}
}

func TestAcceptance_RoomNormalizationCaseInsensitive(t *testing.T) {
	if NormalizeRoom("#Lobby") != NormalizeRoom("#lobby") {
		t.Fatal("rooms must be case-insensitive")
	}
}

func TestAcceptance_ErrorBodyIsText(t *testing.T) {
	env := &Envelope{Body: "rate limited", HasBody: true}
	if !strings.Contains(FormatError(env), "rate") {
		t.Fatal(FormatError(env))
	}
}
