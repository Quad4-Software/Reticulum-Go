// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package link

import (
	"sync"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

type asyncPipeInterface struct {
	common.BaseInterface
	tr     *transport.Transport
	peer   *asyncPipeInterface
	online bool

	wg     *sync.WaitGroup
	queue  chan []byte
	closed chan struct{}
	once   sync.Once
}

type asyncPipePair struct {
	A, B *asyncPipeInterface
}

func newAsyncPipe(nameA, nameB string) *asyncPipePair {
	wg := &sync.WaitGroup{}
	a := &asyncPipeInterface{
		BaseInterface: common.BaseInterface{
			Name:    nameA,
			Type:    common.IFTypeUDP,
			Enabled: true,
			Online:  true,
		},
		online: true,
		wg:     wg,
		queue:  make(chan []byte, 256),
		closed: make(chan struct{}),
	}
	b := &asyncPipeInterface{
		BaseInterface: common.BaseInterface{
			Name:    nameB,
			Type:    common.IFTypeUDP,
			Enabled: true,
			Online:  true,
		},
		online: true,
		wg:     wg,
		queue:  make(chan []byte, 256),
		closed: make(chan struct{}),
	}
	a.peer = b
	b.peer = a

	a.startDispatcher()
	b.startDispatcher()
	return &asyncPipePair{A: a, B: b}
}

func (p *asyncPipeInterface) startDispatcher() {
	go func() {
		for {
			select {
			case <-p.closed:
				return
			case data, ok := <-p.queue:
				if !ok {
					return
				}
				if p.tr != nil {
					p.tr.HandlePacket(data, p)
				}
			}
		}
	}()
}

func (p *asyncPipeInterface) Send(data []byte, _ string) error {
	if !p.online || p.peer == nil || !p.peer.online {
		return nil
	}
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	select {
	case p.peer.queue <- dataCopy:
	case <-p.peer.closed:
	}
	return nil
}

func (p *asyncPipeInterface) IsEnabled() bool { return p.Enabled }
func (p *asyncPipeInterface) IsOnline() bool  { return p.online }
func (p *asyncPipeInterface) GetName() string { return p.Name }
func (p *asyncPipeInterface) Start() error    { return nil }
func (p *asyncPipeInterface) Stop() error {
	p.once.Do(func() { close(p.closed) })
	return nil
}
func (p *asyncPipeInterface) Detach() {
	_ = p.Stop()
}
