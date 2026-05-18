// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

type noopIface struct {
	*common.BaseInterface
}

func newNoopIface(name string) *noopIface {
	return &noopIface{BaseInterface: common.NewBaseInterfacePtr(name, common.IFTypeUDP, true)}
}

func (n *noopIface) Send([]byte, string) error { return nil }
func (n *noopIface) GetName() string           { return n.Name }
func (n *noopIface) IsEnabled() bool           { return n.Enabled }

type errSendIface struct {
	*common.BaseInterface
}

func newErrSendIface(name string) *errSendIface {
	return &errSendIface{BaseInterface: common.NewBaseInterfacePtr(name, common.IFTypeUDP, true)}
}

func (e *errSendIface) Send([]byte, string) error { return errSendDown }
func (e *errSendIface) GetName() string           { return e.Name }
func (e *errSendIface) IsEnabled() bool           { return e.Enabled }

var errSendDown = errors.New("simulated send failure")

func TestInvalidateTransportPathAfterInitiatorFailure_skipsWhenEstablished(t *testing.T) {
	srvIdent, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	cliIdent, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}

	cfg := &common.ReticulumConfig{EnableTransport: true}
	tr := transport.NewTransport(cfg)
	defer tr.Close()
	tr.SetIdentity(cliIdent)

	iface := newNoopIface("wan")
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatal(err)
	}

	dest, err := destination.New(srvIdent, destination.Out, destination.Single, "app", tr, "peer")
	if err != nil {
		t.Fatal(err)
	}
	destHash := dest.GetHash()
	tr.UpdatePath(destHash, bytes.Repeat([]byte{0x77}, 16), "wan", 1)

	l := NewLink(dest, tr, iface, nil, nil)
	l.initiator = true
	l.establishedAt = time.Now()

	l.invalidateTransportPathAfterInitiatorFailure()
	if !tr.HasPath(destHash) {
		t.Fatal("path must not be dropped after link was established")
	}
}

func TestInvalidateTransportPathAfterInitiatorFailure_clearsPathWhenNeverEstablished(t *testing.T) {
	srvIdent, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	cliIdent, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}

	cfg := &common.ReticulumConfig{EnableTransport: true}
	tr := transport.NewTransport(cfg)
	defer tr.Close()
	tr.SetIdentity(cliIdent)

	iface := newNoopIface("wan")
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatal(err)
	}

	dest, err := destination.New(srvIdent, destination.Out, destination.Single, "app", tr, "peer")
	if err != nil {
		t.Fatal(err)
	}
	destHash := dest.GetHash()
	tr.UpdatePath(destHash, bytes.Repeat([]byte{0x77}, 16), "wan", 1)

	l := NewLink(dest, tr, iface, nil, nil)
	l.initiator = true

	l.invalidateTransportPathAfterInitiatorFailure()
	if tr.HasPath(destHash) {
		t.Fatal("path should be cleared when initiator never reached active")
	}
}

func TestInitiatorBadLinkProofExpiresCachedPathAndClosesLink(t *testing.T) {
	srvIdent, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	cliIdent, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}

	cfg := &common.ReticulumConfig{EnableTransport: true}
	tr := transport.NewTransport(cfg)
	defer tr.Close()
	tr.SetIdentity(cliIdent)

	iface := newNoopIface("wan")
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatal(err)
	}

	dest, err := destination.New(srvIdent, destination.Out, destination.Single, "app", tr, "peer")
	if err != nil {
		t.Fatal(err)
	}
	destHash := dest.GetHash()
	tr.UpdatePath(destHash, bytes.Repeat([]byte{0x77}, 16), "wan", 1)
	if !tr.HasPath(destHash) {
		t.Fatal("fixture needs cached path")
	}

	l := NewLink(dest, tr, iface, nil, nil)
	if err := l.generateEphemeralKeys(); err != nil {
		t.Fatal(err)
	}
	l.linkID = bytes.Repeat([]byte{0x03}, 16)
	l.initiator = true
	l.status.Store(int32(StatusPending))
	tr.RegisterLink(l.linkID, l)

	badProof := &packet.Packet{
		PacketType:      packet.PacketTypeProof,
		Context:         packet.ContextLRProof,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Data:            bytes.Repeat([]byte{0xEE}, identity.SigLength/8+KeySize),
	}
	if err := l.ValidateLinkProof(badProof, iface); err == nil {
		t.Fatal("expected proof validation error")
	}
	if tr.HasPath(destHash) {
		t.Fatal("invalid link proof should expire cached transport path")
	}
	if l.GetStatus() != StatusClosed {
		t.Fatalf("link status want Closed, got %d", l.GetStatus())
	}
}

func TestEstablish_sendLinkRequestFailureClearsPath(t *testing.T) {
	srvIdent, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	cliIdent, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}

	cfg := &common.ReticulumConfig{EnableTransport: true}
	tr := transport.NewTransport(cfg)
	defer tr.Close()
	tr.SetIdentity(cliIdent)

	iface := newErrSendIface("wan")
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatal(err)
	}

	dest, err := destination.New(srvIdent, destination.Out, destination.Single, "app", tr, "peer")
	if err != nil {
		t.Fatal(err)
	}
	destHash := dest.GetHash()
	tr.UpdatePath(destHash, bytes.Repeat([]byte{0x77}, 16), "wan", 1)

	l := NewLink(dest, tr, iface, nil, nil)

	if err := l.Establish(); err == nil {
		t.Fatal("Establish should fail when first send errors")
	}
	if tr.HasPath(destHash) {
		t.Fatal("failed establish should expire cached path")
	}
}
