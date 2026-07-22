// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"sync"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/transport"
)

func TestOracleTOCTOUDiscoveryStartStopSingleHandler(t *testing.T) {
	tr := transport.NewTransport(&common.ReticulumConfig{})
	t.Cleanup(func() { _ = tr.Close() })

	d := NewInterfaceDiscovery(tr, 2, nil)
	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() { d.Start() })
		wg.Go(func() { d.Stop() })
	}
	wg.Wait()

	d.Start()
	d.mu.Lock()
	first := d.handler
	d.mu.Unlock()
	if first == nil {
		t.Fatal("handler missing after Start")
	}
	d.Start()
	d.mu.Lock()
	second := d.handler
	d.mu.Unlock()
	if first != second {
		t.Fatal("duplicate Start replaced handler (TOCTOU / double register)")
	}

	d.Stop()
	d.mu.Lock()
	after := d.handler
	d.mu.Unlock()
	if after != nil {
		t.Fatal("handler still set after Stop")
	}
}

func TestAdversarialTOCTOUDiscoveryStartStopRace(t *testing.T) {
	tr := transport.NewTransport(&common.ReticulumConfig{})
	t.Cleanup(func() { _ = tr.Close() })

	d := NewInterfaceDiscovery(tr, DefaultStampValue, nil)
	var wg sync.WaitGroup
	barrier := make(chan struct{})
	for range 32 {
		wg.Go(func() {
			<-barrier
			for range 20 {
				d.Start()
				d.Stop()
			}
		})
	}
	close(barrier)
	wg.Wait()
	d.Stop()
	d.mu.Lock()
	h := d.handler
	d.mu.Unlock()
	if h != nil {
		t.Fatal("handler leaked after adversarial Start/Stop")
	}
}
