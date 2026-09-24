// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

type spamLiveTopo struct {
	relay   *transport.Transport
	flooder *interfaces.UDPInterface
	sink    *interfaces.UDPInterface
	got     *atomic.Int64
}

func setupSpamLiveTopo(t *testing.T, gateway bool) (*spamLiveTopo, func()) {
	t.Helper()
	// freeUDPPort picks are released then rebound below, leaving a window
	// where another process can claim the port. Retry the whole topology on
	// a bind failure rather than failing the test on a lost race.
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		topo, cleanup, err := setupSpamLiveTopoAttempt(t, gateway)
		if err == nil {
			return topo, cleanup
		}
		lastErr = err
		t.Logf("spam topo attempt %d failed: %v", attempt+1, err)
	}
	t.Fatal(lastErr)
	return nil, nil
}

func setupSpamLiveTopoAttempt(t *testing.T, gateway bool) (*spamLiveTopo, func(), error) {
	t.Helper()

	floodListen := freeUDPPort(t)
	relayIn := freeUDPPort(t)
	relayOut := freeUDPPort(t)
	sinkListen := freeUDPPort(t)

	flooder, err := interfaces.NewUDPInterface("spam_flooder",
		"127.0.0.1:"+strconv.Itoa(floodListen), "127.0.0.1:"+strconv.Itoa(relayIn), true)
	if err != nil {
		return nil, nil, fmt.Errorf("flooder: %w", err)
	}
	sink, err := interfaces.NewUDPInterface("spam_sink",
		"127.0.0.1:"+strconv.Itoa(sinkListen), "127.0.0.1:"+strconv.Itoa(relayOut), true)
	if err != nil {
		return nil, nil, fmt.Errorf("sink: %w", err)
	}
	inIface, err := interfaces.NewUDPInterface("spam_in",
		"127.0.0.1:"+strconv.Itoa(relayIn), "127.0.0.1:"+strconv.Itoa(floodListen), true)
	if err != nil {
		return nil, nil, fmt.Errorf("relay in: %w", err)
	}
	outIface, err := interfaces.NewUDPInterface("spam_out",
		"127.0.0.1:"+strconv.Itoa(relayOut), "127.0.0.1:"+strconv.Itoa(sinkListen), true)
	if err != nil {
		return nil, nil, fmt.Errorf("relay out: %w", err)
	}
	if gateway {
		inIface.Mode = common.IFModeGateway
		outIface.Mode = common.IFModeGateway
	}
	// Amplification checks need exactly one forward of the first unique
	// path request. Default ingress PR-burst limiting would trip on a
	// tight duplicate inject and suppress discovery entirely.
	inIface.SetIngressControl(false)
	outIface.SetIngressControl(false)

	cfg := &common.ReticulumConfig{
		EnableTransport: true,
		DoSProtection:   "off",
	}
	tr := transport.NewTransport(cfg)
	if err := tr.RegisterInterface("spam_in", inIface); err != nil {
		return nil, nil, fmt.Errorf("register in: %w", err)
	}
	if err := tr.RegisterInterface("spam_out", outIface); err != nil {
		return nil, nil, fmt.Errorf("register out: %w", err)
	}
	if err := tr.InitializePathRequestHandler(); err != nil {
		return nil, nil, fmt.Errorf("path handler: %w", err)
	}

	var got atomic.Int64
	sink.SetPacketCallback(func([]byte, common.NetworkInterface) {
		got.Add(1)
	})

	if err := flooder.Start(); err != nil {
		_ = tr.Close()
		return nil, nil, fmt.Errorf("start flooder: %w", err)
	}
	if err := sink.Start(); err != nil {
		_ = tr.Close()
		_ = flooder.Stop()
		return nil, nil, fmt.Errorf("start sink: %w", err)
	}
	if err := inIface.Start(); err != nil {
		_ = tr.Close()
		_ = flooder.Stop()
		_ = sink.Stop()
		return nil, nil, fmt.Errorf("start relay in: %w", err)
	}
	if err := outIface.Start(); err != nil {
		_ = tr.Close()
		_ = flooder.Stop()
		_ = sink.Stop()
		return nil, nil, fmt.Errorf("start relay out: %w", err)
	}

	cleanup := func() {
		_ = tr.Close()
		_ = flooder.Stop()
		_ = sink.Stop()
	}
	return &spamLiveTopo{relay: tr, flooder: flooder, sink: sink, got: &got}, cleanup, nil
}

func liveSignedAnnounce(t *testing.T, tr *transport.Transport) []byte {
	t.Helper()
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	dest, err := destination.New(id, destination.In, destination.Single, "reticulum-go", tr, "node")
	if err != nil {
		t.Fatalf("destination: %v", err)
	}
	pkt, err := packet.NewAnnouncePacket(dest.GetHash(), id, []byte("ad"), make([]byte, 16))
	if err != nil {
		t.Fatalf("announce: %v", err)
	}
	raw, err := pkt.Serialize()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	raw[1] = 0
	return raw
}

func livePathRequest(destHash, tag []byte) []byte {
	nameHash := sha256.Sum256([]byte("rnstransport.path.request"))
	final := sha256.Sum256(nameHash[:10])
	data := append(append([]byte(nil), destHash...), tag...)
	pkt := packet.NewPacket(
		packet.DestinationPlain,
		data,
		packet.PacketTypeData,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		false,
		0,
	)
	pkt.DestinationHash = final[:16]
	if err := pkt.Pack(); err != nil {
		return nil
	}
	return pkt.Raw
}

func liveDestHash(seed int) []byte {
	h := sha256.Sum256([]byte{byte(seed), byte(seed >> 8), byte(seed >> 16)})
	return h[:16]
}

func TestLiveSpamFilterDuplicateAnnounceNotAmplified(t *testing.T) {
	liveOrSkip(t)
	topo, cleanup := setupSpamLiveTopo(t, false)
	defer cleanup()

	raw := liveSignedAnnounce(t, topo.relay)
	const inject = 20
	for range inject {
		if err := topo.flooder.ProcessOutgoing(raw); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	waitUntil(t, 2*time.Second, func() bool { return topo.got.Load() >= 1 })
	time.Sleep(400 * time.Millisecond)
	if got := topo.got.Load(); got != 1 {
		t.Fatalf("duplicate announce amplified: sink=%d inject=%d", got, inject)
	}
}

func TestLiveSpamFilterQuietAnnounceStillForwards(t *testing.T) {
	liveOrSkip(t)
	topo, cleanup := setupSpamLiveTopo(t, false)
	defer cleanup()

	raw := liveSignedAnnounce(t, topo.relay)
	if err := topo.flooder.ProcessOutgoing(raw); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitUntil(t, 2*time.Second, func() bool { return topo.got.Load() >= 1 })
}

func TestLiveSpamFilterBadSignatureNotForwarded(t *testing.T) {
	liveOrSkip(t)
	topo, cleanup := setupSpamLiveTopo(t, false)
	defer cleanup()

	raw := liveSignedAnnounce(t, topo.relay)
	raw[len(raw)-1] ^= 0xff
	if err := topo.flooder.ProcessOutgoing(raw); err != nil {
		t.Fatalf("send: %v", err)
	}
	time.Sleep(800 * time.Millisecond)
	if got := topo.got.Load(); got != 0 {
		t.Fatalf("bad signature announce reached sink: %d", got)
	}
}

func TestLiveSpamFilterDuplicatePRNotAmplified(t *testing.T) {
	liveOrSkip(t)
	topo, cleanup := setupSpamLiveTopo(t, true)
	defer cleanup()

	raw := livePathRequest(liveDestHash(7), liveDestHash(8))
	if raw == nil {
		t.Fatal("pack path request")
	}
	const inject = 40
	for range inject {
		if err := topo.flooder.ProcessOutgoing(raw); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	waitUntil(t, 2*time.Second, func() bool { return topo.got.Load() >= 1 })
	time.Sleep(400 * time.Millisecond)
	if got := topo.got.Load(); got != 1 {
		t.Fatalf("duplicate PR amplified: sink=%d inject=%d", got, inject)
	}
}

func TestLiveSpamFilterUniquePRFloodNotAmplified(t *testing.T) {
	liveOrSkip(t)
	topo, cleanup := setupSpamLiveTopo(t, true)
	defer cleanup()

	const inject = 80
	for i := range inject {
		raw := livePathRequest(liveDestHash(i+1), liveDestHash(i+9000))
		if raw == nil {
			t.Fatal("pack path request")
		}
		if err := topo.flooder.ProcessOutgoing(raw); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	time.Sleep(2 * time.Second)
	got := topo.got.Load()
	if got < 1 {
		t.Fatalf("expected at least one unique PR forward, sink=%d", got)
	}
	if got >= int64(inject) {
		t.Fatalf("unique PR flood amplified 1:1 sink=%d inject=%d", got, inject)
	}
	if got > 32 {
		t.Fatalf("unique PR flood exceeded discovery queue cap sink=%d", got)
	}
}

func TestLiveSpamFilterFullModeDoesNotDiscoverUnknownPR(t *testing.T) {
	liveOrSkip(t)
	topo, cleanup := setupSpamLiveTopo(t, false)
	defer cleanup()

	const inject = 16
	for i := range inject {
		raw := livePathRequest(liveDestHash(i+1), liveDestHash(i+4000))
		if raw == nil {
			t.Fatal("pack path request")
		}
		if err := topo.flooder.ProcessOutgoing(raw); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	time.Sleep(1500 * time.Millisecond)
	if got := topo.got.Load(); got != 0 {
		t.Fatalf("full-mode relay rediscovered unknown PRs sink=%d", got)
	}
}
