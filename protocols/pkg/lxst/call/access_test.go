// SPDX-License-Identifier: LicenseRef-Reticulum
package call_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/call"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/phonebook"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/proto"
)

func TestFingerprint(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	fp := call.Fingerprint(id)
	if fp == "" || len(fp) < 19 {
		t.Fatalf("fingerprint %q", fp)
	}
	if call.Fingerprint(nil) != "" {
		t.Fatal("nil identity")
	}
}

func TestRateLimiter(t *testing.T) {
	l := call.NewRateLimiter(time.Hour, 1)
	h := make([]byte, 16)
	if !l.Allow(h) {
		t.Fatal("first")
	}
	if l.Allow(h) {
		t.Fatal("burst exceeded")
	}
}

func TestAllowNoneBusy(t *testing.T) {
	tA := transport.NewTransport(isolatedConfig(t))
	tB := transport.NewTransport(isolatedConfig(t))
	if err := tA.Start(); err != nil {
		t.Fatal(err)
	}
	if err := tB.Start(); err != nil {
		t.Fatal(err)
	}
	ifA := newPairIface("a")
	ifB := newPairIface("b")
	ifA.peer = ifB
	ifB.peer = ifA
	if err := tA.RegisterInterface("a", ifA); err != nil {
		t.Fatal(err)
	}
	if err := tB.RegisterInterface("b", ifB); err != nil {
		t.Fatal(err)
	}
	idA, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	idB, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	busy := make(chan struct{}, 1)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity:    idB,
		UseAudio:    false,
		AllowPolicy: phonebook.AllowNone,
	}, nil)
	sb.Bind(destB)
	_ = destB.Announce(false, nil, nil)
	time.Sleep(100 * time.Millisecond)

	caller := call.NewCall(tA, call.Config{
		Identity: idA,
		UseAudio: false,
		Events: call.Events{
			OnBusy: func(*call.Call) {
				select {
				case busy <- struct{}{}:
				default:
				}
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = caller.Dial(ctx, idB)
	select {
	case <-busy:
	case <-time.After(6 * time.Second):
		if caller.State() != call.StateEnded {
			t.Fatalf("expected busy, state %v", caller.State())
		}
	}
}

func TestAllowNoneEndError(t *testing.T) {
	tA := transport.NewTransport(isolatedConfig(t))
	tB := transport.NewTransport(isolatedConfig(t))
	if err := tA.Start(); err != nil {
		t.Fatal(err)
	}
	if err := tB.Start(); err != nil {
		t.Fatal(err)
	}
	ifA := newPairIface("a")
	ifB := newPairIface("b")
	ifA.peer = ifB
	ifB.peer = ifA
	if err := tA.RegisterInterface("a", ifA); err != nil {
		t.Fatal(err)
	}
	if err := tB.RegisterInterface("b", ifB); err != nil {
		t.Fatal(err)
	}
	idA, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	idB, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity:    idB,
		UseAudio:    false,
		AllowPolicy: phonebook.AllowNone,
		Events: call.Events{
			OnEnded: func(c *call.Call, _ string) {
				select {
				case got <- c.EndError():
				default:
				}
			},
		},
	}, nil)
	sb.Bind(destB)
	_ = destB.Announce(false, nil, nil)
	time.Sleep(100 * time.Millisecond)
	caller := call.NewCall(tA, call.Config{Identity: idA, UseAudio: false})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = caller.Dial(ctx, idB)
	select {
	case err := <-got:
		if !errors.Is(err, call.ErrBlocked) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("no end error")
	}
}

func TestEmptyPhonebookAllowFuncBusy(t *testing.T) {
	tA := transport.NewTransport(isolatedConfig(t))
	tB := transport.NewTransport(isolatedConfig(t))
	if err := tA.Start(); err != nil {
		t.Fatal(err)
	}
	if err := tB.Start(); err != nil {
		t.Fatal(err)
	}
	ifA := newPairIface("a")
	ifB := newPairIface("b")
	ifA.peer = ifB
	ifB.peer = ifA
	if err := tA.RegisterInterface("a", ifA); err != nil {
		t.Fatal(err)
	}
	if err := tB.RegisterInterface("b", ifB); err != nil {
		t.Fatal(err)
	}
	idA, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	idB, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	book := phonebook.New()
	book.AllowPhonebook()
	busy := make(chan struct{}, 1)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity:  idB,
		UseAudio:  false,
		AllowFunc: book.IsAllowed,
	}, nil)
	sb.Bind(destB)
	_ = destB.Announce(false, nil, nil)
	time.Sleep(100 * time.Millisecond)

	caller := call.NewCall(tA, call.Config{
		Identity: idA,
		UseAudio: false,
		Events: call.Events{
			OnBusy: func(*call.Call) {
				select {
				case busy <- struct{}{}:
				default:
				}
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = caller.Dial(ctx, idB)
	select {
	case <-busy:
	case <-time.After(6 * time.Second):
		if caller.State() != call.StateEnded {
			t.Fatalf("expected busy, state %v", caller.State())
		}
	}
}

func TestSwitchProfileAndPTT(t *testing.T) {
	c := call.NewCall(nil, call.Config{UseAudio: false})
	if err := c.SwitchProfile(proto.ProfileQualityHigh); err != nil {
		t.Fatal(err)
	}
	if c.Profile() != proto.ProfileQualityHigh {
		t.Fatalf("profile %d", c.Profile())
	}
	if err := c.SwitchMode(proto.ModeHalfDuplex); err != nil {
		t.Fatal(err)
	}
	c.PTT(true)
	c.Squelch(true)
	c.DisableRemoteModeFollow()
	c.SetTransmitGain(-3)
	c.SetReceiveGain(3)
	c.SetRingtone("/tmp/ringer.opus", 0)
	c.SetSpeaker("HDMI")
	c.SetMicrophone("USB")
	c.SetRinger("PCM")
	c.PauseAGC()
	c.ResumeAGC()
}

func TestSwitchboardSetBusy(t *testing.T) {
	sb := call.NewSwitchboard(nil, call.Config{UseAudio: false}, nil)
	c := call.NewCall(nil, call.Config{UseAudio: false})
	sb.SetBusy(true)
	if sb.Occupy(c) {
		t.Fatal("occupy while externally busy")
	}
	sb.SetBusy(false)
	if !sb.Occupy(c) {
		t.Fatal("occupy after clear")
	}
	sb.Release(c)
}

func TestPhoneAnnounceMin(t *testing.T) {
	tr := transport.NewTransport(isolatedConfig(t))
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(id, destination.In, destination.Single, proto.AppName, tr, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	p := call.NewPhone(tr, dest, call.Config{Identity: id, UseAudio: false})
	_ = p.Announce()
	if err := p.Announce(); err != nil {
		t.Fatal(err)
	}
	if p.Aspect() != "lxst.telephony" {
		t.Fatalf("aspect %s", p.Aspect())
	}
	p.Stop()
}
