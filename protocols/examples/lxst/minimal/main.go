// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"context"
	"fmt"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
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
	ifA.SetPacketCallback(func(data []byte, ni common.NetworkInterface) { tA.HandlePacket(data, ni) })
	ifB.SetPacketCallback(func(data []byte, ni common.NetworkInterface) { tB.HandlePacket(data, ni) })
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

	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		fmt.Println("destination:", err)
		return
	}
	destB.AcceptsLinks(true)

	incoming := make(chan *call.Call, 1)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events: call.Events{
			OnRinging: func(c *call.Call) { incoming <- c },
		},
	}, nil)
	sb.Bind(destB)

	if err := destB.Announce(false, nil, nil); err != nil {
		fmt.Println("announce:", err)
		return
	}
	time.Sleep(100 * time.Millisecond)

	caller := call.NewCall(tA, call.Config{Identity: idA, UseAudio: false})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		select {
		case callee := <-incoming:
			_ = callee.Answer(ctx)
		case <-ctx.Done():
		}
	}()

	if err := caller.Dial(ctx, idB); err != nil {
		fmt.Println("dial:", err)
		return
	}
	fmt.Println("minimal example: call active")
	_ = caller.Hangup("done")
}

func isolatedConfig() *common.ReticulumConfig {
	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	cfg.InMemoryPathTable = true
	cfg.InMemoryKnownDestinations = true
	return cfg
}
