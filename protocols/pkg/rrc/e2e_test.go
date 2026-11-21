// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"testing"
	"time"
)

func TestE2E_ThreeWayChatNoticeAction(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e skipped in -short")
	}

	m := newTestMesh(t, 42640, HubConfig{
		Name:              "e2e-hub",
		Version:           "0.1.0",
		IncludeMemberList: true,
		Limits: HubLimits{
			RateLimitMsgsPerMinute: 120,
			MaxMsgBodyBytes:        350,
		},
	})

	joinedA := make(chan struct{}, 1)
	joinedB := make(chan struct{}, 1)
	gotMsg := make(chan string, 2)
	gotNotice := make(chan string, 1)
	gotAction := make(chan string, 1)
	partedB := make(chan struct{}, 1)

	a := dialMeshClient(t, m, 'A', ClientConfig{
		Nick: "alice",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#e2e" {
					select {
					case joinedA <- struct{}{}:
					default:
					}
				}
			},
			OnMsg: func(env *Envelope) {
				if env.Room != "#e2e" {
					return
				}
				if s, ok := BodyAsString(env.Body); ok {
					gotMsg <- s
				}
			},
			OnNotice: func(env *Envelope) {
				if env.Room != "#e2e" {
					return
				}
				if s, ok := BodyAsString(env.Body); ok {
					gotNotice <- s
				}
			},
		},
	})
	b := dialMeshClient(t, m, 'B', ClientConfig{
		Nick: "bob",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#e2e" {
					select {
					case joinedB <- struct{}{}:
					default:
					}
				}
			},
			OnParted: func(room string, _ *Envelope) {
				if room == "#e2e" {
					select {
					case partedB <- struct{}{}:
					default:
					}
				}
			},
			OnMsg: func(env *Envelope) {
				if env.Room != "#e2e" {
					return
				}
				if s, ok := BodyAsString(env.Body); ok {
					gotMsg <- s
				}
			},
			OnAction: func(env *Envelope) {
				if env.Room != "#e2e" {
					return
				}
				if s, ok := BodyAsString(env.Body); ok {
					gotAction <- s
				}
			},
		},
	})

	if err := a.Join("#e2e"); err != nil {
		t.Fatal(err)
	}
	waitJoined(t, joinedA, "A")
	if err := b.Join("#e2e"); err != nil {
		t.Fatal(err)
	}
	waitJoined(t, joinedB, "B")

	if err := a.SendMsg("#e2e", "hello-e2e"); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-gotMsg:
		if body != "hello-e2e" {
			t.Fatalf("msg=%q", body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout MSG")
	}

	if err := b.SendNotice("#e2e", "notice-e2e"); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-gotNotice:
		if body != "notice-e2e" {
			t.Fatalf("notice=%q", body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout NOTICE")
	}

	if err := a.SendAction("#e2e", "waves"); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-gotAction:
		if body != "waves" {
			t.Fatalf("action=%q", body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout ACTION")
	}

	if err := b.Part("#e2e"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-partedB:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout PARTED")
	}

	w := a.Welcome()
	if w == nil || !w.HasName {
		t.Fatalf("welcome=%+v", w)
	}
	if !w.HasCaps || w.Capabilities[CapAction] != true || w.Capabilities[CapDirectNotice] != true {
		t.Fatalf("welcome caps=%+v", w.Capabilities)
	}
	if a.State() != ClientActive {
		t.Fatalf("state=%v", a.State())
	}
}
