// SPDX-License-Identifier: LicenseRef-Reticulum
package call_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/audio/io"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/call"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/proto"
)

type closeCount struct {
	io.Device
	n atomic.Int32
}

func (c *closeCount) Close() error {
	c.n.Add(1)
	return c.Device.Close()
}

func TestBorrowedDeviceNotClosed(t *testing.T) {
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
	host := io.NewHost()
	counted := &closeCount{Device: host}
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	phoneB := call.NewPhone(tB, destB, call.Config{Identity: idB, Device: counted})
	if err := destB.Announce(false, nil, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			c := phoneB.Switchboard().Active()
			if c != nil && c.Incoming() && c.State() == call.StateRinging {
				_ = c.Answer(ctx)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	caller := call.NewCall(tA, call.Config{Identity: idA, Device: io.NewHost()})
	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = caller.Hangup("done")
	time.Sleep(50 * time.Millisecond)
	if counted.n.Load() != 0 {
		t.Fatalf("borrowed device closed %d times", counted.n.Load())
	}
	if host.Closed() {
		t.Fatal("host closed")
	}
}

func TestPhoneSetDevice(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr := transport.NewTransport(isolatedConfig(t))
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(id, destination.In, destination.Single, proto.AppName, tr, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	phone := call.NewPhone(tr, dest, call.Config{Identity: id, UseAudio: false})
	dev := io.NewHost()
	phone.SetDevice(dev)
	if phone.Switchboard() == nil {
		t.Fatal("missing switchboard")
	}
}
