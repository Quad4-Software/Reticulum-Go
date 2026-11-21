// SPDX-License-Identifier: LicenseRef-Reticulum
package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

func TestUnverifiedDelivered(t *testing.T) {
	s := openTestSession(t, Config{NoRateLimit: true})
	unv := make(chan struct{}, 1)
	got := make(chan struct{}, 1)
	s.events.OnUnverified = func(*lxmf.LXMessage) { unv <- struct{}{} }
	s.events.OnMessage = func(*lxmf.LXMessage) { got <- struct{}{} }
	src := make([]byte, lxmf.DestinationLength)
	src[0] = 0x11
	s.onInbound(&lxmf.LXMessage{SourceHash: src, Title: []byte("t")}, nil)
	select {
	case <-unv:
	default:
		t.Fatal("OnUnverified")
	}
	select {
	case <-got:
	default:
		t.Fatal("OnMessage")
	}
	if s.LastError() != nil {
		t.Fatalf("last error %v", s.LastError())
	}
}

func TestDropUnverifiedFails(t *testing.T) {
	s := openTestSession(t, Config{NoRateLimit: true, DropUnverified: true})
	got := make(chan struct{}, 1)
	s.events.OnMessage = func(*lxmf.LXMessage) { got <- struct{}{} }
	src := make([]byte, lxmf.DestinationLength)
	src[0] = 0x22
	s.onInbound(&lxmf.LXMessage{SourceHash: src}, nil)
	if !errors.Is(s.LastError(), ErrUnverified) {
		t.Fatalf("last error %v", s.LastError())
	}
	select {
	case <-got:
		t.Fatal("OnMessage")
	default:
	}
}

func TestWaitRecallHonorsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := waitRecall(ctx, stubPath{}, [][]byte{make([]byte, lxmf.DestinationLength)}, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}

type stubPath struct{}

func (stubPath) HasPath([]byte) bool { return false }

func (stubPath) RequestPath([]byte, string, []byte, bool) error { return nil }

func openTestSession(t *testing.T, cfg Config) *Session {
	t.Helper()
	c := common.DefaultConfig()
	c.ShareInstance = false
	c.InMemoryPathTable = true
	c.InMemoryKnownDestinations = true
	c.ConfigPath = t.TempDir() + "/config"
	tr := transport.NewTransport(c)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Transport = tr
	cfg.Identity = id
	s, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
