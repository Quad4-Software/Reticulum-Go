// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

func TestE2E_JoinListWhoGreeting(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e skipped in -short")
	}
	cfg := testConfig()
	cfg.Greeting = "welcome-e2e"
	cfg.HubName = "e2e-hub"
	m := newDaemonMesh(t, 43100, cfg)
	_ = m.svc.rooms.Register("lobby", mustID(1))

	joinedA := make(chan struct{}, 1)
	notices := make(chan string, 64)
	a := dialDaemon(t, m, 'A', rrc.ClientConfig{
		Nick: "alice",
		Handlers: rrc.ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *rrc.Envelope) {
				if room == "#e2e" {
					select {
					case joinedA <- struct{}{}:
					default:
					}
				}
			},
			OnNotice: func(env *rrc.Envelope) {
				if s, ok := rrc.BodyAsString(env.Body); ok {
					select {
					case notices <- s:
					default:
					}
				}
			},
		},
	})
	if a.State() != rrc.ClientActive {
		t.Fatalf("state=%v", a.State())
	}
	if a.Welcome() == nil || a.Welcome().HubName != "e2e-hub" {
		t.Fatalf("welcome=%+v", a.Welcome())
	}
	deadline := time.Now().Add(8 * time.Second)
	sawGreeting := false
	for time.Now().Before(deadline) && !sawGreeting {
		select {
		case s := <-notices:
			if strings.Contains(s, "welcome-e2e") {
				sawGreeting = true
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	if !sawGreeting {
		t.Fatal("missing greeting")
	}
	if err := a.Join("#e2e"); err != nil {
		t.Fatal(err)
	}
	waitCh(t, joinedA, "join")
	time.Sleep(150 * time.Millisecond)
	if err := a.SendSlash("/list"); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(8 * time.Second)
	sawList, sawWho := false, false
	var got []string
	for time.Now().Before(deadline) && !sawList {
		select {
		case s := <-notices:
			got = append(got, s)
			if strings.Contains(s, "Registered public rooms") || s == "No public rooms registered" {
				sawList = true
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	if !sawList {
		t.Fatalf("list=%v notices=%q", sawList, got)
	}
	whoDeadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(whoDeadline) && !sawWho {
		if err := a.SendSlash("/who #e2e"); err != nil {
			t.Fatal(err)
		}
		waitUntil := time.Now().Add(time.Second)
		for time.Now().Before(waitUntil) && !sawWho {
			select {
			case s := <-notices:
				got = append(got, s)
				low := strings.ToLower(s)
				if strings.Contains(low, "members in") || strings.Contains(low, "alice") {
					sawWho = true
				}
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
	if !sawWho {
		t.Fatalf("who=false notices=%q", got)
	}
}

func TestE2E_KeyedAndModerated(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e skipped in -short")
	}
	cfg := testConfig()
	m := newDaemonMesh(t, 43110, cfg)
	founder := mustID(1)
	m.svc.rooms.Ensure("#lock", founder, true)
	if err := m.svc.rooms.SetFlag("#lock", "k", true, "s3cret"); err != nil {
		t.Fatal(err)
	}
	if err := m.svc.rooms.SetFlag("#lock", "m", true, ""); err != nil {
		t.Fatal(err)
	}

	errA := make(chan string, 4)
	errB := make(chan string, 4)
	joinedB := make(chan struct{}, 1)
	gotMsg := make(chan string, 2)

	a := dialDaemon(t, m, 'A', rrc.ClientConfig{
		Nick: "alice",
		Handlers: rrc.ClientHandlers{
			OnError: func(env *rrc.Envelope) {
				if s, ok := rrc.BodyAsString(env.Body); ok {
					select {
					case errA <- s:
					default:
					}
				}
			},
		},
	})
	if err := a.Join("#lock"); err != nil {
		t.Fatal(err)
	}
	bad := waitStr(t, errA, "bad key")
	if bad != "bad key (+k)" {
		t.Fatalf("join without key: %q", bad)
	}

	b := dialDaemon(t, m, 'B', rrc.ClientConfig{
		Nick: "bob",
		Handlers: rrc.ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *rrc.Envelope) {
				if room == "#lock" {
					select {
					case joinedB <- struct{}{}:
					default:
					}
				}
			},
			OnError: func(env *rrc.Envelope) {
				if s, ok := rrc.BodyAsString(env.Body); ok {
					select {
					case errB <- s:
					default:
					}
				}
			},
			OnMsg: func(env *rrc.Envelope) {
				if s, ok := rrc.BodyAsString(env.Body); ok {
					select {
					case gotMsg <- s:
					default:
					}
				}
			},
		},
	})
	if err := b.JoinKeyed("#lock", "s3cret"); err != nil {
		t.Fatal(err)
	}
	waitCh(t, joinedB, "keyed join")
	if err := b.SendMsg("#lock", "hello"); err != nil {
		t.Fatal(err)
	}
	mod := waitStr(t, errB, "moderated")
	if mod != "room is moderated (+m)" {
		t.Fatalf("unvoiced msg: %q", mod)
	}
}

func TestE2E_UnrecognizedAndActionNotCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e skipped in -short")
	}
	cfg := testConfig()
	m := newDaemonMesh(t, 43120, cfg)
	joined := make(chan struct{}, 1)
	errs := make(chan string, 4)
	actions := make(chan string, 2)
	a := dialDaemon(t, m, 'A', rrc.ClientConfig{
		Nick: "alice",
		Handlers: rrc.ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *rrc.Envelope) {
				if room == "#e2e" {
					select {
					case joined <- struct{}{}:
					default:
					}
				}
			},
			OnError: func(env *rrc.Envelope) {
				if s, ok := rrc.BodyAsString(env.Body); ok {
					select {
					case errs <- s:
					default:
					}
				}
			},
			OnAction: func(env *rrc.Envelope) {
				if s, ok := rrc.BodyAsString(env.Body); ok {
					select {
					case actions <- s:
					default:
					}
				}
			},
		},
	})
	b := dialDaemon(t, m, 'B', rrc.ClientConfig{
		Nick: "bob",
		Handlers: rrc.ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *rrc.Envelope) {
				if room == "#e2e" {
					select {
					case joined <- struct{}{}:
					default:
					}
				}
			},
			OnAction: func(env *rrc.Envelope) {
				if s, ok := rrc.BodyAsString(env.Body); ok {
					select {
					case actions <- s:
					default:
					}
				}
			},
		},
	})
	if err := a.Join("#e2e"); err != nil {
		t.Fatal(err)
	}
	if err := b.Join("#e2e"); err != nil {
		t.Fatal(err)
	}
	waitCh(t, joined, "A")
	waitCh(t, joined, "B")
	if err := a.SendSlash("/nope"); err != nil {
		t.Fatal(err)
	}
	got := waitStr(t, errs, "unrecognized")
	if got != "unrecognized command" {
		t.Fatalf("slash: %q", got)
	}
	if err := a.SendAction("#e2e", "/stats"); err != nil {
		t.Fatal(err)
	}
	act := waitStr(t, actions, "action")
	if act != "/stats" {
		t.Fatalf("action body=%q", act)
	}
	select {
	case s := <-errs:
		t.Fatalf("ACTION slash produced ERROR %q", s)
	case <-time.After(400 * time.Millisecond):
	}
}
