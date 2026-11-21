// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"context"
	"fmt"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
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

func main() {
	tA := transport.NewTransport(isolatedConfig())
	tB := transport.NewTransport(isolatedConfig())
	if err := tA.Start(); err != nil {
		fmt.Println("start A:", err)
		return
	}
	if err := tB.Start(); err != nil {
		fmt.Println("start B:", err)
		return
	}
	ifA := newPairIface("a")
	ifB := newPairIface("b")
	ifA.peer = ifB
	ifB.peer = ifA
	_ = tA.RegisterInterface("a", ifA)
	_ = tB.RegisterInterface("b", ifB)

	idA, err := identity.New()
	if err != nil {
		fmt.Println("identity A:", err)
		return
	}
	idB, err := identity.New()
	if err != nil {
		fmt.Println("identity B:", err)
		return
	}

	alice, err := session.Open(session.Config{Transport: tA, Identity: idA, DisplayName: "alice"})
	if err != nil {
		fmt.Println("open A:", err)
		return
	}
	defer func() { _ = alice.Close() }()

	got := make(chan *lxmf.LXMessage, 1)
	bob, err := session.Open(session.Config{
		Transport:   tB,
		Identity:    idB,
		DisplayName: "bob",
		Events: session.Events{
			OnMessage: func(msg *lxmf.LXMessage) {
				select {
				case got <- msg:
				default:
				}
			},
		},
	})
	if err != nil {
		fmt.Println("open B:", err)
		return
	}
	defer func() { _ = bob.Close() }()
	if err := bob.Announce(); err != nil {
		fmt.Println("announce:", err)
		return
	}
	time.Sleep(150 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := alice.Send(ctx, idB, "hello", "from alice"); err != nil {
		fmt.Println("send:", err)
		return
	}
	select {
	case msg := <-got:
		fmt.Println(bob.Info().String())
		fmt.Println("recv", msg.TitleString(), msg.ContentString())
	case <-ctx.Done():
		fmt.Println("timeout")
	}
}

func isolatedConfig() *common.ReticulumConfig {
	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	cfg.InMemoryPathTable = true
	cfg.InMemoryKnownDestinations = true
	return cfg
}
