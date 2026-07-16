// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/blackhole"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

// TestRegression_EstablishDoesNotDeadlockWithWatchdog guards against spawning
// the link watchdog while Establish still holds the link mutex (watchdog then
// blocks forever trying to acquire the same lock).
func TestRegression_EstablishDoesNotDeadlockWithWatchdog(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	initLink, _, cleanup := establishInteropLinkAsync(t)
	defer cleanup()

	if initLink.GetStatus() != StatusActive {
		t.Fatalf("initiator status = %d, want Active", initLink.GetStatus())
	}
	if !initLink.watchdogActive.Load() {
		t.Fatal("watchdog should be running on established initiator link")
	}
}

// TestRegression_RequestOnActiveLinkDoesNotSelfDeadlock guards against Request
// calling encrypt() (RLock) while already holding the link write lock.
func TestRegression_RequestOnActiveLinkDoesNotSelfDeadlock(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	respLink.destination.RegisterRequestHandler("echo", func(_ string, data []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) []byte {
		return append([]byte("echo:"), data...)
	}, destination.AllowAll, nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		receipt, err := initLink.Request("echo", []byte("ping"), 3*time.Second)
		if err != nil {
			t.Errorf("Request: %v", err)
			return
		}
		respCh := make(chan []byte, 1)
		receipt.SetResponseCallback(func(r *RequestReceipt) {
			respCh <- append([]byte(nil), r.GetResponse()...)
		})
		select {
		case got := <-respCh:
			if !bytes.Equal(got, []byte("echo:ping")) {
				t.Errorf("response = %q, want echo:ping", got)
			}
		case <-time.After(3 * time.Second):
			t.Error("response timeout")
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Request path deadlocked")
	}
}

// TestRegression_TeardownWhileWatchdogActive guards against Teardown and the
// watchdog both calling encrypt under the link write lock.
func TestRegression_TeardownWhileWatchdogActive(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	initLink, _, cleanup := establishInteropLink(t)
	defer cleanup()

	done := make(chan struct{})
	go func() {
		defer close(done)
		initLink.Teardown()
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Teardown deadlocked with watchdog active")
	}
	if initLink.GetStatus() != StatusClosed {
		t.Fatalf("status after teardown = %d, want Closed", initLink.GetStatus())
	}
}

// TestRegression_WatchdogStaleTeardownDoesNotDeadlock guards against the
// watchdog calling sendTeardownPacket -> encrypt while holding the write lock.
func TestRegression_WatchdogStaleTeardownDoesNotDeadlock(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	initLink, _, cleanup := establishInteropLink(t)
	defer cleanup()

	done := make(chan struct{})
	go func() {
		defer close(done)
		initLink.mutex.Lock()
		initLink.status.Store(int32(StatusStale))
		_ = initLink.sendTeardownPacket()
		initLink.status.Store(int32(StatusClosed))
		initLink.mutex.Unlock()
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stale teardown path deadlocked")
	}
	if initLink.GetStatus() != StatusClosed {
		t.Fatalf("status after stale teardown = %d, want Closed", initLink.GetStatus())
	}
}

// TestRegression_ValidateLinkProofSafeUnderConcurrentInbound guards against
// transport.handleProofPacket calling ValidateLinkProof without the link mutex
// while HandleInbound mutates the same link state.
func TestRegression_ValidateLinkProofSafeUnderConcurrentInbound(t *testing.T) {
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
	tr.UpdatePath(dest.GetHash(), bytes.Repeat([]byte{0x77}, 16), "wan", 1)

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

	var wg sync.WaitGroup
	const workers = 16
	deadline := time.Now().Add(2 * time.Second)
	for range workers {
		wg.Go(func() {
			for time.Now().Before(deadline) {
				_ = l.ValidateLinkProof(badProof, iface)
				_ = l.HandleInbound(&packet.Packet{
					PacketType:      packet.PacketTypeData,
					Context:         packet.ContextKeepalive,
					DestinationType: DestTypeLink,
					DestinationHash: l.linkID,
					Data:            []byte{KeepaliveRequestByte},
				})
			}
		})
	}
	wg.Wait()

	if l.GetStatus() != StatusClosed {
		t.Fatalf("link status want Closed after bad proof storm, got %d", l.GetStatus())
	}
}

// TestRegression_CryptoSafeDuringHandshakeKeyInstall guards against torn reads
// of session/hmac key material while performHandshake writes them.
func TestRegression_CryptoSafeDuringHandshakeKeyInstall(t *testing.T) {
	l := &Link{linkID: bytes.Repeat([]byte{0x01}, 16)}
	if err := l.generateEphemeralKeys(); err != nil {
		t.Fatal(err)
	}
	l.peerPub = bytes.Repeat([]byte{0x02}, KeySize)
	l.mode = ModeAES256CBC
	l.linkID = bytes.Repeat([]byte{0x03}, 16)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = l.performHandshake()
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = l.encrypt([]byte("probe"))
				_, _ = l.decrypt(bytes.Repeat([]byte{0x00}, 64))
			}
		}
	}()
	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// TestRegression_SessionKeysNotAliasedToDerivedKeyBuffer guards against
// encrypt/decrypt observing mutated HKDF output after handshake.
func TestRegression_SessionKeysNotAliasedToDerivedKeyBuffer(t *testing.T) {
	l := &Link{linkID: bytes.Repeat([]byte{0x0A}, 16)}
	if err := l.generateEphemeralKeys(); err != nil {
		t.Fatal(err)
	}
	l.peerPub = l.pub
	l.mode = ModeAES256CBC
	if err := l.performHandshake(); err != nil {
		t.Fatal(err)
	}

	plain := []byte("session-key-isolation-check")
	ct, err := l.encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt before mutation: %v", err)
	}

	for i := range l.derivedKey.Bytes() {
		l.derivedKey.Bytes()[i] ^= 0xFF
	}

	pt, err := l.decrypt(ct)
	if err != nil {
		t.Fatalf("decrypt after derivedKey mutation: %v", err)
	}
	if !bytes.Equal(pt, plain) {
		t.Fatalf("plaintext mismatch after derivedKey mutation: got %q", pt)
	}

	ct2, err := l.encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt after derivedKey mutation: %v", err)
	}
	pt2, err := l.decrypt(ct2)
	if err != nil {
		t.Fatalf("second decrypt after derivedKey mutation: %v", err)
	}
	if !bytes.Equal(pt2, plain) {
		t.Fatalf("second plaintext mismatch: got %q", pt2)
	}
}

// TestRegression_EncryptLockedRoundTrip verifies encryptLocked works when the
// caller already holds the link mutex (used by Request, SendPacketWithContext,
// watchdog keepalive/teardown paths).
func TestRegression_EncryptLockedRoundTrip(t *testing.T) {
	l := &Link{linkID: bytes.Repeat([]byte{0x0B}, 16)}
	if err := l.generateEphemeralKeys(); err != nil {
		t.Fatal(err)
	}
	l.peerPub = l.pub
	l.mode = ModeAES256CBC
	if err := l.performHandshake(); err != nil {
		t.Fatal(err)
	}

	plain := []byte("encrypt-locked-roundtrip")
	l.mutex.Lock()
	ct, err := l.encryptLocked(plain)
	l.mutex.Unlock()
	if err != nil {
		t.Fatalf("encryptLocked: %v", err)
	}
	pt, err := l.decrypt(ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(pt, plain) {
		t.Fatalf("roundtrip mismatch: got %q want %q", pt, plain)
	}
}

// TestRegression_SendPacketWithContextUnderLock guards the inbound resource
// retry path, which sends encrypted resource requests while holding locks.
func TestRegression_SendPacketWithContextUnderLock(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	initLink, _, cleanup := establishInteropLink(t)
	defer cleanup()

	payload := bytes.Repeat([]byte{0xCD}, min(initLink.mdu, 128))
	done := make(chan error, 1)
	go func() { done <- initLink.SendPacketWithContext(payload, packet.ContextResourceReq) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SendPacketWithContext: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SendPacketWithContext deadlocked")
	}
}

// TestRegression_BlackholeIdentifyTearsDownLink pins HandleIdentification:
// when the remote peer Identifys as a blackholed identity the link must close.
// Python only sends LINKIDENTIFY as initiator so the initiator Identifys here.
func TestRegression_BlackholeIdentifyTearsDownLink(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	peerID, err := identity.New()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	localID, err := identity.New()
	if err != nil {
		t.Fatalf("local identity: %v", err)
	}
	blackhole.SetLocalIdentityHash(localID.Hash())
	tab := blackhole.New("")
	if _, err := tab.Add(peerID.Hash(), 0, "regression"); err != nil {
		t.Fatalf("blackhole add: %v", err)
	}
	respLink.transport.SetBlackholeTable(tab)

	closed := make(chan struct{}, 1)
	respLink.SetLinkClosedCallback(func(_ *Link) {
		select {
		case closed <- struct{}{}:
		default:
		}
	})

	if err := initLink.Identify(peerID); err != nil {
		t.Fatalf("Identify: %v", err)
	}

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("expected responder teardown after blackholed LINKIDENTIFY")
	}
	if respLink.GetStatus() != StatusClosed {
		t.Fatalf("responder status=%d want Closed", respLink.GetStatus())
	}
}
