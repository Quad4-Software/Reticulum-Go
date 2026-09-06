// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/transport"
)

// pipeTestBitrate models an in-process lossless pipe, matching the guess
// used by the real Pipe interface (pkg/interfaces/pipe.go). Using
// common.BitrateMinimum here would make AwaitPath treat the loopback as an
// uninitialized slow radio and scale its path-response window to minutes.
const pipeTestBitrate = 1_000_000

type pipeInterface struct {
	common.BaseInterface
	peer   *pipeInterface
	tr     *transport.Transport
	online bool
}

func newPipeInterface(name string) *pipeInterface {
	return &pipeInterface{
		Name:    name,
		Type:    common.IFTypeUDP,
		Mode:    common.IFModeFull,
		Enabled: true,
		Online:  true,
		MTU:     common.DefaultMTU,
		Bitrate: pipeTestBitrate,
		online:  true,
	}
}

func (p *pipeInterface) Send(data []byte, _ string) error {
	if !p.online || p.peer == nil || !p.peer.online || p.peer.tr == nil {
		return nil
	}
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	p.peer.tr.HandlePacket(dataCopy, p.peer)
	return nil
}

func (p *pipeInterface) IsEnabled() bool { return p.Enabled }
func (p *pipeInterface) IsOnline() bool  { return p.online }
func (p *pipeInterface) GetName() string { return p.Name }
func (p *pipeInterface) Start() error    { return nil }
func (p *pipeInterface) Stop() error     { return nil }
func (p *pipeInterface) Detach()         {}

func mustCreateNode(t *testing.T) uint64 {
	t.Helper()
	id, code := NodeCreate("")
	if code != OK || id == 0 {
		t.Fatalf("NodeCreate: code=%d err=%q", code, LastError())
	}
	t.Cleanup(func() { _ = NodeDestroy(id) })
	return id
}

func mustIdentity(t *testing.T) uint64 {
	t.Helper()
	id, code := IdentityGenerate()
	if code != OK || id == 0 {
		t.Fatalf("IdentityGenerate: code=%d err=%q", code, LastError())
	}
	t.Cleanup(func() { _ = IdentityDestroy(id) })
	return id
}

func mustRegisterOutgoingIface(t *testing.T, node uint64, name string) {
	t.Helper()
	rec, err := nodeByHandle(node)
	if err != nil {
		t.Fatal(err)
	}
	iface := newPipeInterface(name)
	if err := rec.node.Transport().RegisterInterface(name, iface); err != nil {
		t.Fatal(err)
	}
	if err := rec.node.Transport().InitializePathRequestHandler(); err != nil {
		t.Fatal(err)
	}
}

func attachPipePair(t *testing.T, nodeA, nodeB uint64) {
	t.Helper()
	recA, err := nodeByHandle(nodeA)
	if err != nil {
		t.Fatal(err)
	}
	recB, err := nodeByHandle(nodeB)
	if err != nil {
		t.Fatal(err)
	}
	pipeA := newPipeInterface("pipeA")
	pipeB := newPipeInterface("pipeB")
	pipeA.peer = pipeB
	pipeB.peer = pipeA
	pipeA.tr = recA.node.Transport()
	pipeB.tr = recB.node.Transport()
	if err := recA.node.Transport().RegisterInterface("pipeA", pipeA); err != nil {
		t.Fatal(err)
	}
	if err := recB.node.Transport().RegisterInterface("pipeB", pipeB); err != nil {
		t.Fatal(err)
	}
	if err := recA.node.Transport().InitializePathRequestHandler(); err != nil {
		t.Fatal(err)
	}
}
