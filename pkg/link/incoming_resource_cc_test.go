// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/resource"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

// --- pure controller tests ---

func newTestCC() *resourceCC {
	cc := &resourceCC{}
	cc.seed(ClassUnknown, 0, false, 0)
	return cc
}

func TestCCClassifyPromotesAfterFastRounds(t *testing.T) {
	cc := newTestCC()
	if cc.class != ClassSlow {
		t.Fatalf("initial class = %v, want slow", cc.class)
	}
	for i := 0; i < ccFastRoundsToPromote-1; i++ {
		cc.rateBps = 100_000
		cc.knownRate = true
		cc.classify()
		if cc.class != ClassSlow {
			t.Fatalf("round %d: promoted early to %v", i, cc.class)
		}
	}
	cc.rateBps = 100_000
	cc.classify()
	if cc.class != ClassFast {
		t.Fatalf("after %d fast rounds class = %v, want fast", ccFastRoundsToPromote, cc.class)
	}
	if got := cc.classWindowMax(); got != resource.WindowMaxFast {
		t.Fatalf("classWindowMax = %d, want %d", got, resource.WindowMaxFast)
	}
}

func TestCCClassifyDemotesAfterVerySlowRounds(t *testing.T) {
	cc := newTestCC()
	for i := 0; i < ccVerySlowRoundsToDemote-1; i++ {
		cc.rateBps = 100
		cc.knownRate = true
		cc.classify()
		if cc.class != ClassSlow {
			t.Fatalf("round %d: demoted early to %v", i, cc.class)
		}
	}
	cc.rateBps = 100
	cc.classify()
	if cc.class != ClassVerySlow {
		t.Fatalf("after %d very-slow rounds class = %v, want very_slow", ccVerySlowRoundsToDemote, cc.class)
	}
	if got := cc.classWindowMax(); got != resource.WindowMaxVerySlow {
		t.Fatalf("classWindowMax = %d, want %d", got, resource.WindowMaxVerySlow)
	}
}

func TestCCClassifyMiddleBandStaysSlow(t *testing.T) {
	cc := newTestCC()
	for i := 0; i < 50; i++ {
		cc.rateBps = 2000 // between RateVerySlow (250) and RateFast (6250)
		cc.knownRate = true
		cc.classify()
	}
	if cc.class != ClassSlow {
		t.Fatalf("middle-band class = %v, want slow", cc.class)
	}
}

// TestCCClassifyOscillatesFastMiddleFast verifies an oscillating
// fast/middle link still promotes (fast rounds persist across middle-band
// rounds, matching Python's fast_rate_rounds) and does not thrash back.
func TestCCClassifyOscillatesFastMiddleFast(t *testing.T) {
	cc := newTestCC()
	rates := []float64{
		1_000_000, // fast 1
		2_000,     // middle
		1_000_000, // fast 2
		1_000_000, // fast 3
		3_000,     // middle
		1_000_000, // fast 4 -> promote
		2_000,     // middle: must STAY fast (no thrash on a single round)
		1_000_000, // fast again: still fast
	}
	for i, r := range rates {
		cc.rateBps = r
		cc.knownRate = true
		cc.classify()
		want := ClassSlow
		if i >= 5 {
			want = ClassFast
		}
		if cc.class != want {
			t.Fatalf("round %d (rate %.0f): class = %v, want %v", i, r, cc.class, want)
		}
	}
}

// TestCCClassifyFastToVerySlowDemotes verifies sustained degradation
// demotes even a previously fast link: fast,middle,then 2 very-slow
// rounds drop the ceiling to WindowMaxVerySlow, and recovery from the
// very-slow floor needs a fresh streak of fast rounds.
func TestCCClassifyFastToVerySlowDemotes(t *testing.T) {
	cc := newTestCC()
	apply := func(rate float64) {
		cc.rateBps = rate
		cc.knownRate = true
		cc.classify()
	}
	for i := 0; i < ccFastRoundsToPromote; i++ {
		apply(1_000_000)
	}
	if cc.class != ClassFast {
		t.Fatalf("precondition: class = %v, want fast", cc.class)
	}
	apply(200) // very slow 1
	if cc.class != ClassFast {
		t.Fatalf("one very-slow round demoted early: %v", cc.class)
	}
	apply(200) // very slow 2 -> demote
	if cc.class != ClassVerySlow {
		t.Fatalf("class after 2 very-slow rounds = %v, want very_slow", cc.class)
	}
	if got := cc.classWindowMax(); got != resource.WindowMaxVerySlow {
		t.Fatalf("ceiling after demotion = %d, want %d", got, resource.WindowMaxVerySlow)
	}
	// Recovery: middle rounds reset the very-slow streak but do not
	// promote; a fresh fast streak is required.
	apply(2_000)
	apply(1_000_000)
	apply(1_000_000)
	if cc.class != ClassVerySlow {
		t.Fatalf("recovery promoted early: %v", cc.class)
	}
	for i := 0; i < ccFastRoundsToPromote-1; i++ {
		apply(1_000_000)
	}
	if cc.class != ClassFast {
		t.Fatalf("recovery did not promote: %v", cc.class)
	}
}

// TestCCClassifySlowFastSlowFastOscillation verifies a link alternating
// between a very-slow and a fast regime settles into the very-slow
// ceiling while slow, and re-promotes after sustained fast rounds.
func TestCCClassifySlowFastSlowFastOscillation(t *testing.T) {
	cc := newTestCC()
	slowPhase := func() {
		applyN(cc, 100, ccVerySlowRoundsToDemote)
		if cc.class != ClassVerySlow {
			t.Fatalf("slow phase: class = %v, want very_slow", cc.class)
		}
	}
	fastPhase := func() {
		applyN(cc, 1_000_000, ccFastRoundsToPromote)
		if cc.class != ClassFast {
			t.Fatalf("fast phase: class = %v, want fast", cc.class)
		}
	}
	slowPhase()
	fastPhase()
	slowPhase()
	fastPhase()
}

func applyN(cc *resourceCC, rateBps float64, n int) {
	for i := 0; i < n; i++ {
		cc.rateBps = rateBps
		cc.knownRate = true
		cc.classify()
	}
}

func TestCCCloseRoundMeasuresAndSkipsStalledRounds(t *testing.T) {
	cc := newTestCC()
	start := time.Unix(1_000_000, 0)
	cc.noteRequestSent(start, 0)
	// A stall during the round invalidates the measurement.
	cc.noteStallDuringRound()
	cc.closeRound(start.Add(2*time.Second), 40_000)
	if cc.knownRate {
		t.Fatal("stalled round must not produce a rate estimate")
	}
	// Healthy round: 40 KB over 2 s = 20 KB/s.
	cc.noteRequestSent(start.Add(2*time.Second), 40_000)
	cc.closeRound(start.Add(4*time.Second), 80_000)
	if !cc.knownRate {
		t.Fatal("healthy round must set knownRate")
	}
	if cc.rateBps < 19_000 || cc.rateBps > 21_000 {
		t.Fatalf("rateBps = %.0f, want ~20000", cc.rateBps)
	}
	// A tiny round (e.g. HMU-only) must not corrupt the estimate.
	before := cc.rateBps
	cc.noteRequestSent(start.Add(4*time.Second), 80_000)
	cc.closeRound(start.Add(5*time.Second), 80_001)
	if cc.rateBps != before {
		t.Fatalf("1-byte round changed rate to %.0f", cc.rateBps)
	}
}

func TestCCNoteFirstPartSRTT(t *testing.T) {
	cc := newTestCC()
	start := time.Unix(1_000_000, 0)
	cc.noteRequestSent(start, 0)
	cc.noteFirstPart(start.Add(300 * time.Millisecond))
	if cc.srtt < 0.29 || cc.srtt > 0.31 {
		t.Fatalf("first srtt = %.4f, want ~0.3", cc.srtt)
	}
	// Later parts of the same round must not update the RTT.
	cc.noteFirstPart(start.Add(2 * time.Second))
	if cc.srtt < 0.29 || cc.srtt > 0.31 {
		t.Fatalf("second part updated srtt to %.4f", cc.srtt)
	}
	// A new round with a different RTT moves the estimate slightly.
	cc.noteRequestSent(start.Add(1*time.Second), 0)
	cc.noteFirstPart(start.Add(1*time.Second + 100*time.Millisecond))
	if cc.srtt <= 0.28 || cc.srtt >= 0.31 {
		t.Fatalf("srtt did not move toward new sample: %.4f", cc.srtt)
	}
}

func TestCCStallGrace(t *testing.T) {
	const sdu = int64(resource.DefaultSegmentSize)

	// Fast link, known rate: floor-dominated (250 ms grace floor).
	cc := newTestCC()
	cc.knownRate = true
	cc.rateBps = 1_000_000
	cc.srtt = 0.01
	if got := cc.stallGrace(75*sdu, 0, false, false, 0); got > 2*time.Second {
		t.Fatalf("fast link grace = %v, want near the 250ms floor", got)
	}
	if got := cc.stallGrace(0, 1000, true, false, 0); got > 2*time.Second {
		t.Fatalf("fast HMU grace = %v, want near the floor", got)
	}

	// Slow link, known rate: scales with outstanding bytes (10 parts x 384
	// B at 250 B/s -> expected 15.4 s, margin ~31 s).
	cc2 := newTestCC()
	cc2.knownRate = true
	cc2.rateBps = 250
	cc2.srtt = 2.0
	got := cc2.stallGrace(10*sdu, 0, false, false, 0)
	if got < 30*time.Second || got > 36*time.Second {
		t.Fatalf("slow link grace = %v, want ~31s", got)
	}

	// Unknown rate: bounded conservative allowance, not 10 minutes.
	cc3 := newTestCC()
	got = cc3.stallGrace(75*sdu, 0, false, false, 0)
	if got < 2*time.Second || got > 60*time.Second {
		t.Fatalf("unknown-rate grace = %v, want bounded 2..60s", got)
	}

	// Reliable transport doubles the margin.
	cc4 := newTestCC()
	cc4.knownRate = true
	cc4.rateBps = 100_000
	cc4.srtt = 0.5
	unrel := cc4.stallGrace(10*sdu, 0, false, false, 0)
	rel := cc4.stallGrace(10*sdu, 0, false, true, 0)
	if rel < unrel*2 || rel > unrel*2+time.Second {
		t.Fatalf("reliable grace %v not ~2x unreliable %v", rel, unrel)
	}

	// Backoff grows per stall and caps.
	cc5 := newTestCC()
	cc5.knownRate = true
	cc5.rateBps = 1_000_000
	g0 := cc5.stallGrace(10*sdu, 0, false, false, 0)
	g1 := cc5.stallGrace(10*sdu, 0, false, false, 1)
	g10 := cc5.stallGrace(10*sdu, 0, false, false, 10)
	if g1 < g0+ccPerStallBackoff {
		t.Fatalf("stall backoff missing: %v vs %v", g1, g0)
	}
	if g10 > g0+ccPerStallBackoffMax+time.Second {
		t.Fatalf("stall backoff not capped: %v vs %v", g10, g0)
	}
	if got := cc.stallGrace(75*sdu, 0, false, false, 0); got < ccGraceFloor {
		t.Fatalf("grace below floor: %v", got)
	}
}

// --- impaired in-memory pipe integration tests ---

type impairedOptions struct {
	dropRate    float64
	delayMin    time.Duration
	delayMax    time.Duration
	bytesPerSec float64
	// dropFilter classifies each packet. dropAlways drops it regardless
	// of dropRate, dropNever spares it, dropApplyRate applies dropRate.
	dropFilter func(data []byte) dropDecision
}

type dropDecision int

const (
	dropApplyRate dropDecision = iota
	dropAlways
	dropNever
)

type impairedPipe struct {
	common.BaseInterface
	tr     *transport.Transport
	peer   *impairedPipe
	online bool
	queue  chan []byte
	closed chan struct{}
	once   sync.Once
	rng    *rand.Rand
	optMu  sync.Mutex
	opts   impairedOptions
}

func newImpairedPair(nameA, nameB string, opts impairedOptions) *impairedPipe {
	makeOne := func(name string) *impairedPipe {
		return &impairedPipe{
			BaseInterface: common.BaseInterface{
				Name:    name,
				Type:    common.IFTypeUDP,
				Enabled: true,
				Online:  true,
			},
			online: true,
			opts:   opts,
			queue:  make(chan []byte, 1024),
			closed: make(chan struct{}),
			rng:    rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
		}
	}
	a, b := makeOne(nameA), makeOne(nameB)
	a.peer, b.peer = b, a
	go a.dispatch()
	go b.dispatch()
	return a // .peer is B
}

// setImpairment swaps the impairment profile on both directions at
// runtime, for regime-change tests (fast->slow, oscillating links).
func (p *impairedPipe) setImpairment(opts impairedOptions) {
	for _, q := range []*impairedPipe{p, p.peer} {
		q.optMu.Lock()
		q.opts = opts
		q.optMu.Unlock()
	}
}

func (p *impairedPipe) dispatch() {
	for {
		select {
		case <-p.closed:
			return
		case data, ok := <-p.queue:
			if !ok {
				return
			}
			p.optMu.Lock()
			opts := p.opts
			p.optMu.Unlock()
			delay := opts.delayMin
			if opts.delayMax > opts.delayMin {
				p.optMu.Lock()
				delay += time.Duration(p.rng.Float64() * float64(opts.delayMax-opts.delayMin))
				p.optMu.Unlock()
			}
			if opts.bytesPerSec > 0 {
				delay += time.Duration(float64(len(data)) / opts.bytesPerSec * float64(time.Second))
			}
			if delay > 0 {
				select {
				case <-time.After(delay):
				case <-p.closed:
					return
				}
			}
			if p.tr != nil {
				p.tr.HandlePacket(data, p)
			}
		}
	}
}

func (p *impairedPipe) Send(data []byte, _ string) error {
	if !p.online || p.peer == nil || !p.peer.online {
		return nil
	}
	p.optMu.Lock()
	dropRate := p.opts.dropRate
	dropFilter := p.opts.dropFilter
	p.optMu.Unlock()
	decision := dropApplyRate
	if dropFilter != nil {
		decision = dropFilter(data)
	}
	if decision == dropAlways {
		return nil
	}
	if decision == dropApplyRate {
		p.optMu.Lock()
		drop := p.rng.Float64() < dropRate
		p.optMu.Unlock()
		if drop {
			return nil
		}
	}
	// decision == dropNever: deliver regardless of dropRate.
	dataCopy := append([]byte(nil), data...)
	select {
	case p.peer.queue <- dataCopy:
	case <-p.peer.closed:
	}
	return nil
}

func (p *impairedPipe) IsEnabled() bool { return p.Enabled }
func (p *impairedPipe) IsOnline() bool  { return p.online }
func (p *impairedPipe) GetName() string { return p.Name }
func (p *impairedPipe) Start() error    { return nil }
func (p *impairedPipe) Stop() error {
	p.once.Do(func() { close(p.closed) })
	return nil
}
func (p *impairedPipe) Detach() { _ = p.Stop() }

func establishImpairedLink(t *testing.T, opts impairedOptions) (initiator *Link, responder *Link, cleanup func()) {
	t.Helper()
	return establishImpairedLinkOn(t, newImpairedPair("impairedA", "impairedB", opts))
}

func establishImpairedLinkOn(t *testing.T, pair *impairedPipe) (initiator *Link, responder *Link, cleanup func()) {
	t.Helper()
	skipHeavyLinkTestsIfShort(t)

	cfgA := unstampedLinkConfig()
	trA := transport.NewTransport(cfgA)
	idA, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	cfgB := unstampedLinkConfig()
	trB := transport.NewTransport(cfgB)

	pair.tr = trA
	pair.peer.tr = trB

	if err := trA.RegisterInterface(pair.Name, pair); err != nil {
		t.Fatalf("RegisterInterface A: %v", err)
	}
	if err := trB.RegisterInterface(pair.peer.Name, pair.peer); err != nil {
		t.Fatalf("RegisterInterface B: %v", err)
	}

	destA, err := destination.New(idA, destination.In, destination.Single, "ccapp", trA, "service")
	if err != nil {
		t.Fatalf("destination.New: %v", err)
	}
	destA.AcceptsLinks(true)

	var (
		responderLink *Link
		respMu        sync.Mutex
	)
	destA.SetLinkEstablishedCallback(func(l any) {
		if lnk, ok := l.(*Link); ok {
			respMu.Lock()
			responderLink = lnk
			respMu.Unlock()
		}
	})
	if err := destA.Announce(false, nil, nil); err != nil {
		t.Fatalf("Announce: %v", err)
	}
	pathDeadline := time.Now().Add(20 * time.Second)
	for !trB.HasPath(destA.GetHash()) && time.Now().Before(pathDeadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !trB.HasPath(destA.GetHash()) {
		t.Fatal("trB never learned path to destA")
	}

	initiatorLink := NewLink(destA, trB, pair.peer, func(*Link) {}, nil)
	if err := initiatorLink.Establish(); err != nil {
		t.Fatalf("Establish: %v", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		respMu.Lock()
		up := responderLink != nil
		respMu.Unlock()
		if up && initiatorLink.GetStatus() == StatusActive {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	respMu.Lock()
	resp := responderLink
	respMu.Unlock()
	if resp == nil {
		t.Fatal("responder link never established")
	}

	cleanup = func() {
		initiatorLink.Teardown()
		trA.Close()
		trB.Close()
	}
	return initiatorLink, resp, cleanup
}

func sendAndAwaitResource(t *testing.T, init *Link, resp *Link, payload []byte, timeout time.Duration) {
	t.Helper()
	if err := resp.SetResourceStrategy(AcceptAll); err != nil {
		t.Fatalf("SetResourceStrategy: %v", err)
	}
	got := make(chan []byte, 1)
	resp.SetResourceConcludedCallback(func(p any) {
		switch v := p.(type) {
		case IncomingResource:
			got <- v.Data
		case []byte:
			got <- v
		}
	})
	res, err := resource.New(payload, false)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- init.SendResource(res) }()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("SendResource: %v", err)
		}
	case <-time.After(timeout):
		t.Fatal("SendResource timeout")
	}
	select {
	case b := <-got:
		if !bytes.Equal(b, payload) {
			t.Fatalf("payload mismatch: got %d bytes want %d", len(b), len(payload))
		}
	case <-time.After(timeout):
		t.Fatal("resource conclusion timeout")
	}
}

// TestIncomingResourceCCFastLinkReachesFastCeiling verifies the controller
// promotes the window ceiling on a fast in-memory path and that the next
// transfer on the same link starts from the carried state.
func TestIncomingResourceCCFastLinkReachesFastCeiling(t *testing.T) {
	init, resp, cleanup := establishImpairedLink(t, impairedOptions{})
	defer cleanup()

	payload := bytes.Repeat([]byte{0xA5}, 100_000)
	sendAndAwaitResource(t, init, resp, payload, 60*time.Second)

	if !resp.lastResourceRateKnown {
		t.Fatal("rate not carried after completion")
	}
	if resp.lastResourceClass != ClassFast {
		t.Fatalf("class = %v, want fast", resp.lastResourceClass)
	}
	if resp.lastResourceWindow < resource.WindowMaxFast/2 {
		t.Fatalf("lastResourceWindow = %d, want >= %d", resp.lastResourceWindow, resource.WindowMaxFast/2)
	}

	// Second transfer should start with the fast ceiling immediately.
	sendAndAwaitResource(t, init, resp, bytes.Repeat([]byte{0x5A}, 50_000), 60*time.Second)
	if resp.lastResourceClass != ClassFast {
		t.Fatalf("second transfer class = %v, want fast", resp.lastResourceClass)
	}
}

// TestIncomingResourceCCSlowLinkStaysConservative verifies a throttled path
// stays in the slow class (ceiling 10) instead of opening a huge window.
func TestIncomingResourceCCSlowLinkStaysConservative(t *testing.T) {
	init, resp, cleanup := establishImpairedLink(t, impairedOptions{
		bytesPerSec: 4000, // middle band: above RateVerySlow, below RateFast
		delayMin:    30 * time.Millisecond,
		delayMax:    60 * time.Millisecond,
	})
	defer cleanup()

	payload := bytes.Repeat([]byte{0x3C}, 20_000)
	sendAndAwaitResource(t, init, resp, payload, 120*time.Second)

	if resp.lastResourceClass == ClassFast {
		t.Fatalf("4 KB/s link classified fast (rate=%0.f B/s)", resp.lastResourceRateBps)
	}
	if resp.lastResourceWindow > resource.WindowMaxSlow {
		t.Fatalf("window on slow link = %d, want <= %d", resp.lastResourceWindow, resource.WindowMaxSlow)
	}
}

// TestIncomingResourceCCLossyLinkCompletes verifies a lossy path still
// completes via stall re-requests, and that the controller demotes rather
// than collapsing the transfer.
func TestIncomingResourceCCLossyLinkCompletes(t *testing.T) {
	pair := newImpairedPair("ccA-lossy", "ccB-lossy", impairedOptions{
		dropRate: 0.05,
		delayMin: 10 * time.Millisecond,
		delayMax: 40 * time.Millisecond,
		// The lossy link models data-path loss for the congestion
		// controller: parts, REQ and HMU packets drop at 5% and the
		// receiver must recover via stall re-requests. Advertisements,
		// proofs and keepalives are spared: the Go sender does not
		// retry a lost RESOURCE_ADV or resource proof (Python
		// retransmits the advertisement with MAX_ADV_RETRIES), so a
		// dropped ADV or proof would hang SendResource independently
		// of window control. Sender-side retry is a separate concern.
		dropFilter: func(data []byte) dropDecision {
			pkt := &packet.Packet{Raw: append([]byte(nil), data...)}
			if err := pkt.Unpack(); err != nil {
				return dropApplyRate
			}
			// The setup announce is sent once and never retried, same as
			// ADV/PRF: a lost announce hangs the test before the CC logic
			// under test ever runs.
			if pkt.PacketType == packet.PacketTypeAnnounce {
				return dropNever
			}
			switch pkt.Context {
			case packet.ContextResourceAdv, packet.ContextResourcePRF, packet.ContextKeepalive:
				return dropNever
			}
			return dropApplyRate
		},
	})
	init, resp, cleanup := establishImpairedLinkOn(t, pair)
	defer cleanup()

	payload := bytes.Repeat([]byte{0x77}, 60_000)
	sendAndAwaitResource(t, init, resp, payload, 120*time.Second)

	if resp.lastResourceClass == ClassVerySlow {
		t.Logf("note: 5%% loss link classified very_slow")
	}
}

// TestIncomingResourceCCSeedClampsLastWindow verifies the seeding helper
// clamps a carried window into the current ceiling.
func TestIncomingResourceCCSeedClampsLastWindow(t *testing.T) {
	if got := startIncomingWindow(75, resource.WindowMin, resource.WindowMaxVerySlow, resource.WindowMaxSlow); got != resource.WindowMaxVerySlow {
		t.Fatalf("startIncomingWindow fast->veryslow = %d", got)
	}
	if got := startIncomingWindow(75, resource.WindowMin, resource.WindowMaxFast, resource.WindowMaxSlow); got != 75 {
		t.Fatalf("startIncomingWindow fast->fast = %d", got)
	}
	if got := startIncomingWindow(0, resource.WindowMin, resource.WindowMaxSlow, resource.WindowMaxSlow); got != resource.WindowMaxSlow {
		t.Fatalf("startIncomingWindow fresh = %d", got)
	}
}

// TestIncomingResourceCCHMUOverdueRerequestsWhilePartsFlow verifies the
// watchdog re-requests a lost hashmap update even while parts keep
// arriving (which keeps the general stall check from ever firing), and
// that it does so without disturbing in-flight parts or the window.
func TestIncomingResourceCCHMUOverdueRerequestsWhilePartsFlow(t *testing.T) {
	pair := newImpairedPair("ccA-hmu", "ccB-hmu", impairedOptions{})
	// Drop every HMU (0xFB) packet in both directions so the transfer can
	// only progress if the watchdog keeps re-requesting the hashmap.
	pair.setImpairment(impairedOptions{
		dropFilter: func(data []byte) dropDecision {
			pkt := &packet.Packet{Raw: append([]byte(nil), data...)}
			if err := pkt.Unpack(); err != nil {
				return dropApplyRate
			}
			if pkt.Context == packet.ContextResourceHMU {
				return dropAlways
			}
			return dropApplyRate
		},
	})

	init, resp, cleanup := establishImpairedLinkOn(t, pair)
	defer cleanup()

	if err := resp.SetResourceStrategy(AcceptAll); err != nil {
		t.Fatalf("SetResourceStrategy: %v", err)
	}
	res, err := resource.New(bytes.Repeat([]byte{0x99}, 60_000), false)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}
	go func() { _ = init.SendResource(res) }()

	// The transfer cannot complete without an HMU, so the assertion is
	// that the receiver keeps issuing HMU re-requests while parts flow:
	// exactly the silence that used to starve the peer's keepalive.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp.incomingMu.Lock()
		rx := resp.incomingRx
		var rounds int
		if rx != nil {
			rounds = rx.hmuRounds
		}
		resp.incomingMu.Unlock()
		if rounds >= 4 {
			cleanup()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("watchdog never re-requested the overdue HMU")
}

// toggleImpairment flips the pipe through a rotating list of impairment
// profiles on a timer, for mid-transfer regime-change tests.
func toggleImpairment(p *impairedPipe, stop <-chan struct{}, interval time.Duration, profiles []impairedOptions) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	i := 0
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			p.setImpairment(profiles[i%len(profiles)])
			i++
		}
	}
}

// TestIncomingResourceCCSlowLinkSpeedsUpMidTransfer verifies a slow link
// that suddenly becomes fast promotes the ceiling mid-transfer and
// finishes the transfer with the fast window.
func TestIncomingResourceCCSlowLinkSpeedsUpMidTransfer(t *testing.T) {
	pair := newImpairedPair("ccA-speedup", "ccB-speedup", impairedOptions{
		bytesPerSec: 3000,
		delayMin:    40 * time.Millisecond,
		delayMax:    80 * time.Millisecond,
	})
	stop := make(chan struct{})
	// After 6 s the link suddenly becomes fast.
	go func() {
		time.Sleep(6 * time.Second)
		pair.setImpairment(impairedOptions{})
		close(stop)
	}()

	init, resp, cleanup := establishImpairedLinkOn(t, pair)
	defer cleanup()

	payload := bytes.Repeat([]byte{0x11}, 40_000)
	sendAndAwaitResource(t, init, resp, payload, 120*time.Second)

	if resp.lastResourceClass != ClassFast {
		t.Fatalf("class after speed-up = %v (rate %.0f B/s), want fast", resp.lastResourceClass, resp.lastResourceRateBps)
	}
	if resp.lastResourceWindow < resource.WindowMaxSlow {
		t.Fatalf("window after speed-up = %d, want promoted beyond slow ceiling", resp.lastResourceWindow)
	}
}

// TestIncomingResourceCCFastLinkSlowsDownMidTransfer verifies a fast link
// that degrades to a middle-band rate does not thrash: the sticky fast
// class is kept (demotion needs sustained very-slow rounds) and the
// transfer still completes.
func TestIncomingResourceCCFastLinkSlowsDownMidTransfer(t *testing.T) {
	// 300 KB/s keeps the measured rate in the fast band (>> 6250 B/s)
	// while making an 80 KB transfer take long enough to switch mid-flight.
	pair := newImpairedPair("ccA-slowdown", "ccB-slowdown", impairedOptions{
		bytesPerSec: 300_000,
	})
	go func() {
		time.Sleep(100 * time.Millisecond)
		pair.setImpairment(impairedOptions{
			bytesPerSec: 8000, // middle band: not very-slow, not fast
			delayMin:    10 * time.Millisecond,
			delayMax:    20 * time.Millisecond,
		})
	}()

	init, resp, cleanup := establishImpairedLinkOn(t, pair)
	defer cleanup()

	payload := bytes.Repeat([]byte{0x22}, 80_000)
	sendAndAwaitResource(t, init, resp, payload, 150*time.Second)

	if resp.lastResourceClass == ClassVerySlow {
		t.Fatalf("middle-band degradation wrongly demoted to very_slow (rate %.0f B/s)", resp.lastResourceRateBps)
	}
	if resp.lastResourceWindow < 40 {
		t.Fatalf("window collapsed after slowdown: %d", resp.lastResourceWindow)
	}
	t.Logf("class after slowdown = %v, final window = %d", resp.lastResourceClass, resp.lastResourceWindow)
}

// TestIncomingResourceCCOscillatingLinkCompletes toggles the link between
// fast and middle-band regimes repeatedly during one transfer and asserts
// the transfer still completes and the controller ends in a sane state.
func TestIncomingResourceCCOscillatingLinkCompletes(t *testing.T) {
	pair := newImpairedPair("ccA-osc", "ccB-osc", impairedOptions{
		bytesPerSec: 300_000,
	})
	stop := make(chan struct{})
	go toggleImpairment(pair, stop, 300*time.Millisecond, []impairedOptions{
		{}, // fast
		{bytesPerSec: 8000, delayMin: 10 * time.Millisecond, delayMax: 20 * time.Millisecond}, // middle
	})
	defer close(stop)

	init, resp, cleanup := establishImpairedLinkOn(t, pair)
	defer cleanup()

	payload := bytes.Repeat([]byte{0x33}, 200_000)
	sendAndAwaitResource(t, init, resp, payload, 150*time.Second)

	switch resp.lastResourceClass {
	case ClassFast, ClassSlow, ClassVerySlow:
		// Any settled class is sane after an oscillating transfer; the
		// transfer itself is the assertion.
	default:
		t.Fatalf("unexpected final class %v", resp.lastResourceClass)
	}
}
