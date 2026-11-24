// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"context"
	"fmt"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/audio/io"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/session"
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
	cfgA := isolatedConfig()
	cfgB := isolatedConfig()
	tA := transport.NewTransport(cfgA)
	tB := transport.NewTransport(cfgB)
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

	alice, err := session.Open(session.Config{Transport: tA, Identity: idA})
	if err != nil {
		fmt.Println("open A:", err)
		return
	}
	defer func() { _ = alice.Close() }()
	bob, err := session.Open(session.Config{Transport: tB, Identity: idB})
	if err != nil {
		fmt.Println("open B:", err)
		return
	}
	defer func() { _ = bob.Close() }()
	if err := bob.Announce(); err != nil {
		fmt.Println("announce:", err)
		return
	}
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			c := bob.Active()
			if c != nil && c.Incoming() {
				_ = bob.Answer(ctx)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	if _, err := alice.Dial(ctx, idB); err != nil {
		fmt.Println("dial:", err)
		return
	}
	tone := make([]int16, io.DefaultFrameSize)
	for i := range tone {
		tone[i] = 8000
	}
	_ = alice.PushPCM(tone)
	fmt.Println(alice.Info().String())
	_ = alice.Hangup()
	fmt.Println("ended", alice.LastReason())
}

func isolatedConfig() *common.ReticulumConfig {
	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	cfg.InMemoryPathTable = true
	cfg.InMemoryKnownDestinations = true
	return cfg
}
