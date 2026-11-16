// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

// contestIfaces builds a transport with the given named tracking ifaces.
func contestIfaces(t *testing.T, names ...string) (*Transport, map[string]*trackingIface) {
	t.Helper()
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	ifaces := make(map[string]*trackingIface, len(names))
	for _, n := range names {
		ti := newTrackingIface(n)
		if err := tr.RegisterInterface(n, ti); err != nil {
			t.Fatalf("register %s: %v", n, err)
		}
		ifaces[n] = ti
	}
	t.Cleanup(func() { _ = tr.Close() })
	return tr, ifaces
}

func contestPathIface(t *testing.T, tr *Transport, destHash []byte) string {
	t.Helper()
	tr.mutex.RLock()
	p := tr.paths[pathMapKey(destHash)]
	tr.mutex.RUnlock()
	if p == nil || p.Interface == nil {
		return ""
	}
	return p.Interface.GetName()
}

func contestPendingJobs(t *testing.T, tr *Transport) int {
	t.Helper()
	tr.pendingAnnounceMu.Lock()
	defer tr.pendingAnnounceMu.Unlock()
	return len(tr.pendingAnnounceJobs)
}

func contestAnnounceCopy(raw []byte, wireHops byte) []byte {
	cp := append([]byte(nil), raw...)
	cp[1] = wireHops
	return cp
}

// A duplicate announce copy arriving via a worse route must be dropped
// before signature verification, but the copy is buffered as a failover
// candidate for its interface.
func TestDedupDuplicateWorseRouteDropped(t *testing.T) {
	tr, ifaces := contestIfaces(t, "ifA", "ifB")

	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	raw, destHash := signedAnnounce(t, tr, id)

	if err := tr.handleAnnouncePacket(contestAnnounceCopy(raw, 2), ifaces["ifA"]); err != nil {
		t.Fatalf("first announce: %v", err)
	}
	if got := contestPathIface(t, tr, destHash); got != "ifA" {
		t.Fatalf("path iface = %q, want ifA", got)
	}
	jobsBefore := contestPendingJobs(t, tr)

	// Same emission via ifB with more hops: loses the contest, must not
	// verify, update the path, or schedule a forward.
	if err := tr.handleAnnouncePacket(contestAnnounceCopy(raw, 6), ifaces["ifB"]); err != nil {
		t.Fatalf("duplicate announce: %v", err)
	}
	if got := contestPathIface(t, tr, destHash); got != "ifA" {
		t.Fatalf("path iface moved to %q on worse dupe", got)
	}
	if got := contestPendingJobs(t, tr); got != jobsBefore {
		t.Fatalf("losing dupe scheduled a forward: jobs %d -> %d", jobsBefore, got)
	}

	key := pathMapKey(destHash)
	tr.mutex.RLock()
	alts := tr.pendingAltAnnounces[key]
	tr.mutex.RUnlock()
	if len(alts) != 1 || alts[0].iface != "ifB" {
		t.Fatalf("expected one buffered alternate via ifB, got %+v", alts)
	}
}

// A duplicate announce copy via a higher-gravity interface wins the path
// contest even with equal hops, matching the reference gravity rule. The
// displaced path is demoted into the backup list.
func TestDedupDuplicateGravityWins(t *testing.T) {
	tr, ifaces := contestIfaces(t, "ifA", "ifB")
	ifaces["ifA"].SetGravity(1)
	ifaces["ifB"].SetGravity(10)

	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	raw, destHash := signedAnnounce(t, tr, id)

	if err := tr.handleAnnouncePacket(contestAnnounceCopy(raw, 2), ifaces["ifA"]); err != nil {
		t.Fatalf("first announce: %v", err)
	}
	if got := contestPathIface(t, tr, destHash); got != "ifA" {
		t.Fatalf("path iface = %q, want ifA", got)
	}

	// Equal hops, higher gravity: the dupe must verify and replace the path.
	if err := tr.handleAnnouncePacket(contestAnnounceCopy(raw, 2), ifaces["ifB"]); err != nil {
		t.Fatalf("gravity dupe: %v", err)
	}
	if got := contestPathIface(t, tr, destHash); got != "ifB" {
		t.Fatalf("path iface = %q, want ifB after gravity win", got)
	}

	key := pathMapKey(destHash)
	tr.mutex.RLock()
	backups := tr.backupPaths[key]
	tr.mutex.RUnlock()
	if len(backups) != 1 || backups[0].ifaceName != "ifA" {
		t.Fatalf("expected displaced ifA path in backups, got %+v", backups)
	}
}

// When the active path is marked unresponsive, a buffered same-emission
// alternate is re-injected through the normal pipeline and wins the
// contest, restoring a route without waiting for a path request.
func TestPendingAltFailoverOnUnresponsive(t *testing.T) {
	tr, ifaces := contestIfaces(t, "ifA", "ifB")

	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	raw, destHash := signedAnnounce(t, tr, id)

	if err := tr.handleAnnouncePacket(contestAnnounceCopy(raw, 2), ifaces["ifA"]); err != nil {
		t.Fatalf("first announce: %v", err)
	}
	if err := tr.handleAnnouncePacket(contestAnnounceCopy(raw, 6), ifaces["ifB"]); err != nil {
		t.Fatalf("worse dupe: %v", err)
	}
	key := pathMapKey(destHash)
	tr.mutex.RLock()
	if n := len(tr.pendingAltAnnounces[key]); n != 1 {
		tr.mutex.RUnlock()
		t.Fatalf("expected 1 pending alternate, got %d", n)
	}
	tr.mutex.RUnlock()

	tr.MarkPathUnresponsive(destHash)

	if got := contestPathIface(t, tr, destHash); got != "ifB" {
		t.Fatalf("path iface = %q after failover, want ifB", got)
	}
	tr.mutex.RLock()
	state := tr.pathStates[key]
	tr.mutex.RUnlock()
	if state == StateUnresponsive {
		t.Fatal("path still marked unresponsive after failover")
	}
}

// A verified backup path promotes instantly when the active path fails,
// with no announce traffic required.
func TestBackupPathPromotedOnUnresponsive(t *testing.T) {
	tr, _ := contestIfaces(t, "ifA", "ifB")

	sum := sha256.Sum256([]byte("backup-dest"))
	destHash := sum[:16]
	now := time.Now()
	blobA := bytes.Repeat([]byte{0xA1}, 10)
	blobB := bytes.Repeat([]byte{0xB2}, 10)

	tr.updatePathUnlocked(destHash, []byte{1}, "ifA", 3, blobA, nil, now)
	tr.updatePathUnlocked(destHash, []byte{2}, "ifB", 2, blobB, nil, now)
	if got := contestPathIface(t, tr, destHash); got != "ifB" {
		t.Fatalf("path iface = %q, want ifB", got)
	}

	tr.MarkPathUnresponsive(destHash)

	if got := contestPathIface(t, tr, destHash); got != "ifA" {
		t.Fatalf("path iface = %q after backup promotion, want ifA", got)
	}
	key := pathMapKey(destHash)
	tr.mutex.RLock()
	n := len(tr.backupPaths[key])
	tr.mutex.RUnlock()
	if n != 0 {
		t.Fatalf("expected backup consumed, %d remain", n)
	}
}

// A promoted backup path answers a pending local path request immediately
// instead of leaving the waiter for the request timeout.
func TestBackupPromotionAnswersPendingPathRequest(t *testing.T) {
	tr, _ := contestIfaces(t, "ifA", "ifB")

	sum := sha256.Sum256([]byte("pr-answer-dest"))
	destHash := sum[:16]
	now := time.Now()
	blobA := bytes.Repeat([]byte{0xA1}, 10)
	blobB := bytes.Repeat([]byte{0xB2}, 10)

	tr.updatePathUnlocked(destHash, []byte{1}, "ifA", 3, blobA, nil, now)
	tr.updatePathUnlocked(destHash, []byte{2}, "ifB", 2, blobB, nil, now)
	if got := contestPathIface(t, tr, destHash); got != "ifB" {
		t.Fatalf("path iface = %q, want ifB", got)
	}

	// Seed the announce cache so a path response can be built for the
	// waiting local client.
	tr.cacheAnnouncePacket(destHash, &packet.Packet{
		DestinationHash: append([]byte(nil), destHash...),
		Data:            []byte("announce-payload"),
	})

	waiter := newTrackingIface("waiter")
	if err := tr.RegisterInterface("waiter", waiter); err != nil {
		t.Fatalf("register waiter: %v", err)
	}
	tr.notePendingLocalPathRequest(destHash, waiter)

	tr.MarkPathUnresponsive(destHash)

	if got := contestPathIface(t, tr, destHash); got != "ifA" {
		t.Fatalf("path iface = %q after promotion, want ifA", got)
	}
	tr.mutex.RLock()
	_, stillPending := tr.pendingLocalPathReqs[destKey(destHash)]
	tr.mutex.RUnlock()
	if stillPending {
		t.Fatal("pending local path request not consumed by promotion")
	}
	if _, sends, _ := waiter.counts(); sends == 0 {
		t.Fatal("waiter iface received no path response after promotion")
	}
}

// The pending alternate table is bounded per destination.
func TestPendingAltPerDestCap(t *testing.T) {
	tr, ifaces := contestIfaces(t, "ifA", "ifB", "ifC", "ifD", "ifE")

	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	raw, destHash := signedAnnounce(t, tr, id)
	if err := tr.handleAnnouncePacket(contestAnnounceCopy(raw, 2), ifaces["ifA"]); err != nil {
		t.Fatalf("first announce: %v", err)
	}
	for _, n := range []string{"ifB", "ifC", "ifD", "ifE"} {
		if err := tr.handleAnnouncePacket(contestAnnounceCopy(raw, 9), ifaces[n]); err != nil {
			t.Fatalf("dupe via %s: %v", n, err)
		}
	}
	key := pathMapKey(destHash)
	tr.mutex.RLock()
	n := len(tr.pendingAltAnnounces[key])
	tr.mutex.RUnlock()
	if n != maxPendingAltPerDest {
		t.Fatalf("pending alternates = %d, want cap %d", n, maxPendingAltPerDest)
	}
}

func meshPathIface(t *testing.T, tr *Transport, destHash []byte) string {
	t.Helper()
	tr.mutex.RLock()
	p := tr.paths[pathMapKey(destHash)]
	tr.mutex.RUnlock()
	if p == nil || p.Interface == nil {
		return ""
	}
	return p.Interface.GetName()
}

func waitForMeshPathIface(t *testing.T, tr *Transport, destHash []byte, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := meshPathIface(t, tr, destHash); got == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("path iface = %q, want %q", meshPathIface(t, tr, destHash), want)
}

// TestMeshDedupContestFailover runs the real receive pipeline on a
// three-node triangle. Node 2 first hears node 0's announce over the
// direct link, then the same emission arrives relayed through node 1.
// The relayed copy loses the dedup contest and is buffered as a pending
// alternate; when the direct path fails, the alternate is re-injected
// and wins through the unresponsive-path rule.
func TestMeshDedupContestFailover(t *testing.T) {
	enableSimFastPath(t)
	net := newSimNetwork(t, 3)
	net.link(t, 0, 1)
	net.link(t, 1, 2)
	net.link(t, 0, 2)
	defer net.close()

	n0, n1, n2 := net.nodes[0], net.nodes[1], net.nodes[2]
	pkt := n0.announcePacket(t)

	// node2.ifaces[0] faces node1, ifaces[1] faces node0 (link order).
	directSide := n0.ifaces[1]
	relaySide := n1.ifaces[1]

	// Direct copy first: wire hops 1 -> announceHops 2.
	direct := append([]byte(nil), pkt...)
	direct[1] = 1
	if err := directSide.Send(direct, ""); err != nil {
		t.Fatalf("send direct copy: %v", err)
	}
	waitForMeshPathIface(t, n2.tr, n0.destHash, "node0002_to_node0000", 5*time.Second)

	// Same emission relayed via node 1: wire hops 2 -> announceHops 3.
	// Loses the contest, gets buffered for failover.
	relayed := append([]byte(nil), pkt...)
	relayed[1] = 2
	if err := relaySide.Send(relayed, ""); err != nil {
		t.Fatalf("send relayed copy: %v", err)
	}
	time.Sleep(400 * time.Millisecond)

	if got := meshPathIface(t, n2.tr, n0.destHash); got != "node0002_to_node0000" {
		t.Fatalf("path iface = %q after losing dupe, want direct", got)
	}
	key := pathMapKey(n0.destHash)
	n2.tr.mutex.RLock()
	alts := n2.tr.pendingAltAnnounces[key]
	n2.tr.mutex.RUnlock()
	if len(alts) != 1 || alts[0].iface != "node0002_to_node0001" {
		t.Fatalf("expected buffered alternate via node1 iface, got %+v", alts)
	}

	// Direct path fails: the buffered alternate must be re-injected and
	// win through the unresponsive rule (more hops allowed when the
	// current path is dead).
	n2.tr.MarkPathUnresponsive(n0.destHash)
	waitForMeshPathIface(t, n2.tr, n0.destHash, "node0002_to_node0001", 5*time.Second)
}
