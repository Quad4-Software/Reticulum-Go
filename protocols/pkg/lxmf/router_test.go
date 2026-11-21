// SPDX-License-Identifier: LicenseRef-Reticulum

package lxmf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

func TestMessageStoreAddListPurge(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMessageStore(dir, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest := make([]byte, DestinationLength)
	copy(dest, []byte("desthash12345678"))
	lxm := append(append([]byte(nil), dest...), []byte("encrypted-payload")...)
	stamp := make([]byte, StampSize)
	for i := range stamp {
		stamp[i] = byte(i)
	}
	tid := sha256.Sum256(lxm)
	if _, err := ms.Add(lxm, stamp, 12); err != nil {
		t.Fatal(err)
	}
	if ms.Count() != 1 {
		t.Fatalf("count=%d", ms.Count())
	}
	list := ms.ListForDestination(dest)
	if len(list) != 1 {
		t.Fatalf("list len=%d", len(list))
	}
	if n := ms.Purge([][]byte{tid[:]}, dest); n != 1 {
		t.Fatalf("purge removed=%d", n)
	}
	if ms.Count() != 0 {
		t.Fatal("expected empty store after purge")
	}
}

func TestValidatePNStampsBatch(t *testing.T) {
	dest := make([]byte, DestinationLength)
	copy(dest, []byte("abcd1234abcd1234"))
	lxm := append(append([]byte(nil), dest...), bytesRepeat('x', Overhead+32)...)
	tid := sha256.Sum256(lxm)
	stamp, _, err := GenerateStamp(context.Background(), tid[:], 8, WorkblockExpandRoundsPN)
	if err != nil {
		t.Fatal(err)
	}
	transient := append(append([]byte(nil), lxm...), stamp...)
	got := ValidatePNStamps([][]byte{transient, []byte("bad")}, 8)
	if len(got) != 1 {
		t.Fatalf("validated=%d", len(got))
	}
	if hex.EncodeToString(got[0].TransientID) != hex.EncodeToString(tid[:]) {
		t.Fatal("transient mismatch")
	}
}

func TestEncodePNAnnounceAppDataRoundTrip(t *testing.T) {
	raw, err := EncodePNAnnounceAppData(1700000000, true, 256, 10240, 16, 3, 18, "node-a")
	if err != nil {
		t.Fatal(err)
	}
	if !PNAnnounceDataIsValid(raw) {
		t.Fatal("invalid PN announce")
	}
	name, ok, err := PNNameFromAppData(raw)
	if err != nil || !ok || name != "node-a" {
		t.Fatalf("name=%q ok=%v err=%v", name, ok, err)
	}
}

func TestRouterDeliveryMesh(t *testing.T) {
	if testing.Short() {
		t.Skip("router mesh skipped in -short mode")
	}
	mesh := newRouterMesh(t, 43100)
	defer mesh.close()

	msgDir := filepath.Join(mesh.recvHome, "messages")
	if err := os.MkdirAll(msgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	mesh.recvRouter.messagesDir = msgDir

	title := "router-e2e"
	body := "hello router"
	outMsg, err := NewMessage(mesh.recvDeliveryHash(), mesh.sendDeliveryHash(), []byte(title), []byte(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := outMsg.Pack(mesh.sendID); err != nil {
		t.Fatal(err)
	}
	if err := mesh.sendMessenger.Send(outMsg); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(15 * time.Second)
	var found string
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(msgDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			_, msg, err := ReadFromFile(filepath.Join(msgDir, ent.Name()), RecallSource)
			if err != nil || msg == nil {
				continue
			}
			if msg.TitleString() == title && msg.ContentString() == body {
				found = ent.Name()
				break
			}
		}
		if found != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if found == "" {
		t.Fatal("timeout waiting for inbound message file")
	}
}

func TestRouterPropagationStats(t *testing.T) {
	if testing.Short() {
		t.Skip("router stats skipped in -short mode")
	}
	mesh := newRouterMesh(t, 43150)
	defer mesh.close()

	if err := mesh.recvRouter.EnablePropagation(); err != nil {
		t.Fatal(err)
	}
	stats := mesh.recvRouter.PropagationStats()
	if stats == nil {
		t.Fatal("nil stats")
	}
	if _, ok := stats["messagestore"]; !ok {
		t.Fatal("missing messagestore")
	}
	if _, ok := stats["peers"]; !ok {
		t.Fatal("missing peers")
	}
}

func TestPropagationControlStatsHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("control stats skipped in -short mode")
	}
	mesh := newRouterMesh(t, 43160)
	defer mesh.close()

	if err := mesh.recvRouter.EnablePropagation(); err != nil {
		t.Fatal(err)
	}
	mesh.recvRouter.controlAllow[peerKey(mesh.sendID.Hash())] = struct{}{}
	resp := mesh.recvRouter.statsGetRequestHandler("", nil, nil, nil, mesh.sendID, 0)
	stats, ok := resp.(map[string]any)
	if !ok {
		t.Fatalf("unexpected response type %T", resp)
	}
	if _, ok := stats["messagestore"]; !ok {
		t.Fatal("missing messagestore")
	}
	if _, ok := stats["peers"]; !ok {
		t.Fatal("missing peers")
	}

	denied := mesh.recvRouter.statsGetRequestHandler("", nil, nil, nil, nil, 0)
	// Upstream returns bare integer error codes on the wire.
	if code, ok := asInt64(denied); !ok || code != int64(PeerErrorNoIdentity) {
		t.Fatalf("expected no identity error, got %T %v", denied, denied)
	}
}

type routerMesh struct {
	sendTr, recvTr         *transport.Transport
	sendRouter, recvRouter *Router
	sendMessenger          *Messenger
	sendID, recvID         *identity.Identity
	sendDeliveryHash       func() []byte
	recvDeliveryHash       func() []byte
	recvHome               string
	ifaces                 []interfaces.Interface
}

func (m *routerMesh) close() {
	for _, iface := range m.ifaces {
		_ = iface.Stop()
	}
	if m.sendRouter != nil {
		m.sendRouter.Stop()
	}
	if m.recvRouter != nil {
		m.recvRouter.Stop()
	}
	_ = m.sendTr.Close()
	_ = m.recvTr.Close()
}

func newRouterMesh(t *testing.T, basePort int) *routerMesh {
	t.Helper()
	sendTr := transport.NewTransport(testconfig.New())
	recvTr := transport.NewTransport(testconfig.New())
	if err := sendTr.Start(); err != nil {
		t.Fatal(err)
	}
	if err := recvTr.Start(); err != nil {
		t.Fatal(err)
	}

	addr1 := "127.0.0.1:" + strconv.Itoa(basePort)
	addr2 := "127.0.0.1:" + strconv.Itoa(basePort+1)
	iface1 := startLXMFUDP(t, "R-send", addr1, addr2, sendTr)
	iface2 := startLXMFUDP(t, "R-recv", addr2, addr1, recvTr)

	sendHome := t.TempDir()
	recvHome := t.TempDir()

	sendID := mustNewIdentity(t)
	recvID := mustNewIdentity(t)

	sendCfg := DefaultConfig()
	recvCfg := DefaultConfig()
	recvCfg.LXMF.DisplayName = "recv-peer"

	sendRouter, err := NewRouter(sendID, sendTr, RouterOptions{
		Config:      sendCfg,
		StoragePath: filepath.Join(sendHome, "storage"),
		MessagesDir: filepath.Join(sendHome, "storage", "messages"),
	})
	if err != nil {
		t.Fatal(err)
	}
	recvRouter, err := NewRouter(recvID, recvTr, RouterOptions{
		Config:      recvCfg,
		StoragePath: filepath.Join(recvHome, "storage"),
		MessagesDir: filepath.Join(recvHome, "storage", "messages"),
	})
	if err != nil {
		t.Fatal(err)
	}

	sendDest, err := sendRouter.RegisterDelivery("sender", nil)
	if err != nil {
		t.Fatal(err)
	}
	recvDest, err := recvRouter.RegisterDelivery("receiver", nil)
	if err != nil {
		t.Fatal(err)
	}
	sendRouter.Start()
	recvRouter.Start()

	identity.Remember(nil, recvDest.GetHash(), recvID.GetPublicKey(), nil)
	identity.Remember(nil, sendDest.GetHash(), sendID.GetPublicKey(), nil)

	sendMessenger := NewMessenger(sendTr, sendDest)
	sendHash := append([]byte(nil), sendDest.GetHash()...)
	recvHash := append([]byte(nil), recvDest.GetHash()...)

	_ = sendDest.Announce(false, nil, nil)
	_ = recvDest.Announce(false, nil, nil)
	deadline := time.Now().Add(pathEstablishWait)
	for {
		if sendTr.HasPath(recvHash) && recvTr.HasPath(sendHash) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("router mesh path timeout")
		}
		_ = sendDest.Announce(false, nil, nil)
		_ = recvDest.Announce(false, nil, nil)
		time.Sleep(40 * time.Millisecond)
	}

	mesh := &routerMesh{
		sendTr: sendTr, recvTr: recvTr,
		sendRouter: sendRouter, recvRouter: recvRouter,
		sendMessenger: sendMessenger,
		sendID:        sendID, recvID: recvID,
		sendDeliveryHash: func() []byte { return append([]byte(nil), sendHash...) },
		recvDeliveryHash: func() []byte { return append([]byte(nil), recvHash...) },
		recvHome:         recvHome,
		ifaces:           []interfaces.Interface{iface1, iface2},
	}
	t.Cleanup(mesh.close)
	return mesh
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
