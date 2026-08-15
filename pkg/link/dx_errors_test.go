// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"errors"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

func TestEstablishRequiresPath(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr := transport.NewTransport(nil)
	defer tr.Close()

	dest, err := destination.New(id, destination.Out, destination.Single, "test", tr, "link")
	if err != nil {
		t.Fatal(err)
	}

	l := NewLink(dest, tr, nil, nil, nil)
	err = l.Establish()
	if !errors.Is(err, common.ErrLinkNoPath) {
		t.Fatalf("got %v, want ErrLinkNoPath", err)
	}
	if !strings.Contains(err.Error(), "AwaitPath") {
		t.Fatalf("error should hint AwaitPath: %v", err)
	}
}

func TestEstablishRequiresTransport(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(id, destination.Out, destination.Single, "test", nil, "link")
	if err != nil {
		t.Fatal(err)
	}
	l := NewLink(dest, nil, nil, nil, nil)
	err = l.Establish()
	if !errors.Is(err, common.ErrLinkTransportRequired) {
		t.Fatalf("got %v, want ErrLinkTransportRequired", err)
	}
}

func TestEstablishRequiresDestination(t *testing.T) {
	tr := transport.NewTransport(nil)
	defer tr.Close()
	l := NewLink(nil, tr, nil, nil, nil)
	err := l.Establish()
	if !errors.Is(err, common.ErrLinkDestinationRequired) {
		t.Fatalf("got %v, want ErrLinkDestinationRequired", err)
	}
}

func TestEstablishRejectsInFlightAndSettled(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr := transport.NewTransport(nil)
	defer tr.Close()
	dest, err := destination.New(id, destination.Out, destination.Single, "test", tr, "link")
	if err != nil {
		t.Fatal(err)
	}
	l := NewLink(dest, tr, nil, nil, nil)
	l.requestTime = time.Now()
	err = l.Establish()
	if !errors.Is(err, common.ErrLinkEstablishBusy) {
		t.Fatalf("in-flight Establish = %v, want ErrLinkEstablishBusy", err)
	}
	l.requestTime = time.Time{}
	l.status.Store(int32(StatusActive))
	err = l.Establish()
	if !errors.Is(err, common.ErrLinkAlreadySettled) {
		t.Fatalf("active Establish = %v, want ErrLinkAlreadySettled", err)
	}
}
