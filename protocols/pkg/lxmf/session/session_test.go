// SPDX-License-Identifier: LicenseRef-Reticulum
package session_test

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf/session"
)

type pairIface struct {
	common.BaseInterface
	peer *pairIface
}

func newPairIface(name string) *pairIface {
	p := &pairIface{BaseInterface: common.NewBaseInterface(name, common.IFTypeAuto, true)}
	p.MTU = common.DefaultMTU
	p.Bitrate = 1_000_000
	p.In = true
	p.Out = true
	p.Enable()
	return p
}

func (p *pairIface) Send(data []byte, _ string) error {
	if p.peer == nil {
		return nil
	}
	cp := append([]byte(nil), data...)
	go p.peer.ProcessIncoming(cp)
	return nil
}

func (p *pairIface) ProcessOutgoing(data []byte) error { return p.Send(data, "") }

func isolatedConfig(t *testing.T) *common.ReticulumConfig {
	t.Helper()
	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	cfg.InMemoryPathTable = true
	cfg.InMemoryKnownDestinations = true
	cfg.ConfigPath = t.TempDir() + "/config"
	return cfg
}

func pairedSessions(t *testing.T) (*session.Session, *session.Session, *identity.Identity) {
	t.Helper()
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
	gotA, err := session.Open(session.Config{Transport: tA, Identity: idA})
	if err != nil {
		t.Fatal(err)
	}
	gotB, err := session.Open(session.Config{Transport: tB, Identity: idB})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = gotA.Close()
		_ = gotB.Close()
	})
	if err := gotA.Announce(); err != nil {
		t.Fatal(err)
	}
	if err := gotB.Announce(); err != nil {
		t.Fatal(err)
	}
	waitPath(t, tA, gotB.DestHash())
	waitPath(t, tB, gotA.DestHash())
	return gotA, gotB, idB
}

func waitPath(t *testing.T, tr *transport.Transport, hash []byte) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if tr.HasPath(hash) {
			if _, err := identity.Recall(hash); err == nil {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no path or recall")
}

func TestParseHash(t *testing.T) {
	raw := make([]byte, lxmf.DestinationLength)
	raw[0] = 0xab
	raw[1] = 0xcd
	hexs := hex.EncodeToString(raw)
	got, err := session.ParseHash(" <" + hexs[:4] + " " + hexs[4:] + "> ")
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != hexs {
		t.Fatalf("got %x", got)
	}
	if session.FormatHash(got) == "" {
		t.Fatal("empty format")
	}
}

func TestParseHashRejectsEmpty(t *testing.T) {
	_, err := session.ParseHash(" <> ")
	if !errors.Is(err, session.ErrInvalidHash) {
		t.Fatalf("got %v", err)
	}
}

func TestParseHashRejectsShort(t *testing.T) {
	_, err := session.ParseHash("abcd")
	if !errors.Is(err, session.ErrInvalidHash) {
		t.Fatalf("got %v", err)
	}
}

func TestOpenRequiresTransport(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Open(session.Config{Identity: id}); err != session.ErrNoTransport {
		t.Fatalf("got %v", err)
	}
}

func TestOpenRequireStampNeedsCost(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr := transport.NewTransport(isolatedConfig(t))
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	_, err = session.Open(session.Config{Transport: tr, Identity: id, RequireStamp: true})
	if !errors.Is(err, session.ErrRequireStampCost) {
		t.Fatalf("got %v", err)
	}
}

func TestSendReceive(t *testing.T) {
	a, b, idB := pairedSessions(t)
	got := make(chan *lxmf.LXMessage, 1)
	b.SetOnMessage(func(msg *lxmf.LXMessage) {
		select {
		case got <- msg:
		default:
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := a.Send(ctx, idB, "hi", "there"); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case msg := <-got:
		if msg.TitleString() != "hi" || msg.ContentString() != "there" {
			t.Fatalf("got %q %q", msg.TitleString(), msg.ContentString())
		}
		if !msg.SignatureValidated {
			t.Fatal("unsigned")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	info := b.Info()
	if info.Recv < 1 || info.Aspect != "lxmf.delivery" || !info.Announced {
		t.Fatalf("info %+v", info)
	}
	if info.AllowPolicy != "all" {
		t.Fatalf("allow %q", info.AllowPolicy)
	}
	if info.String() == "" {
		t.Fatal("empty info string")
	}
}

func TestSendHashGrouped(t *testing.T) {
	a, b, _ := pairedSessions(t)
	got := make(chan struct{}, 1)
	b.SetOnMessage(func(*lxmf.LXMessage) {
		select {
		case got <- struct{}{}:
		default:
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := a.SendHash(ctx, session.FormatHash(b.DestHash()), "t", "body"); err != nil {
		t.Fatalf("send hash: %v", err)
	}
	select {
	case <-got:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestAllowNoneDrops(t *testing.T) {
	a, b, idB := pairedSessions(t)
	dropped := make(chan error, 1)
	b.SetAllowNone(true)
	b.SetOnMessage(func(*lxmf.LXMessage) {
		t.Error("delivered while allow none")
	})
	b.SetOnDropped(func(_ *lxmf.LXMessage, err error) {
		select {
		case dropped <- err:
		default:
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := a.Send(ctx, idB, "no", "thanks"); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case err := <-dropped:
		if !errors.Is(err, session.ErrNotAllowed) {
			t.Fatalf("drop %v", err)
		}
	case <-ctx.Done():
		t.Fatal("expected drop")
	}
	if b.Info().AllowPolicy != "none" {
		t.Fatalf("allow %q", b.Info().AllowPolicy)
	}
}

func TestAllowNoneLastError(t *testing.T) {
	a, b, idB := pairedSessions(t)
	b.SetAllowNone(true)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := a.Send(ctx, idB, "no", "thanks"); err != nil {
		t.Fatalf("send: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(b.LastError(), session.ErrNotAllowed) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("last error %v", b.LastError())
}

func TestReceiveErrorDecrypt(t *testing.T) {
	a, _, _ := pairedSessions(t)
	pkt := packet.NewPacket(
		packet.DestinationSingle,
		[]byte{1, 2, 3, 4, 5, 6, 7, 8},
		packet.PacketTypeData,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		true,
		packet.FlagUnset,
	)
	a.Messenger().Receive(pkt, nil)
	if a.LastError() == nil {
		t.Fatal("expected decrypt error")
	}
}

func TestSendUnknownHash(t *testing.T) {
	a, _, _ := pairedSessions(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	raw := make([]byte, lxmf.DestinationLength)
	raw[0] = 0xff
	_, err := a.SendHash(ctx, hex.EncodeToString(raw), "t", "x")
	if !errors.Is(err, session.ErrRecall) {
		t.Fatalf("got %v", err)
	}
	if a.LastError() == nil {
		t.Fatal("last error not stored")
	}
}

func TestGoSendHash(t *testing.T) {
	a, b, _ := pairedSessions(t)
	got := make(chan struct{}, 1)
	b.SetOnMessage(func(*lxmf.LXMessage) {
		select {
		case got <- struct{}{}:
		default:
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	a.GoSendHash(ctx, hex.EncodeToString(b.DestHash()), "go", "async")
	select {
	case <-got:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestSendPropagatedWithoutNode(t *testing.T) {
	a, b, _ := pairedSessions(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := a.SendPropagated(ctx, hex.EncodeToString(b.DestHash()), "t", "x")
	if !errors.Is(err, session.ErrNoPropagation) {
		t.Fatalf("got %v", err)
	}
}

func TestInfoIdle(t *testing.T) {
	a, _, _ := pairedSessions(t)
	info := a.Info()
	if info.DestHash == "" || info.LocalHash == "" {
		t.Fatal("missing hashes")
	}
	if info.Aspect != "lxmf.delivery" {
		t.Fatalf("aspect %q", info.Aspect)
	}
	if info.AllowPolicy != "all" {
		t.Fatalf("allow %q", info.AllowPolicy)
	}
	if info.Propagation != "none" {
		t.Fatalf("prop %q", info.Propagation)
	}
	if !info.Announced {
		t.Fatal("expected announced")
	}
}

func TestRaceSessionSend(t *testing.T) {
	a, b, idB := pairedSessions(t)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for range 20 {
			_, _ = a.Send(ctx, idB, "r", "x")
		}
	}()
	go func() {
		defer wg.Done()
		for range 20 {
			_ = b.Info()
			b.SetAllowNone(false)
		}
	}()
	wg.Wait()
}
