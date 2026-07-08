// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"net"
	"os/exec"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestPipeInterfaceConcurrentSendRace(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not available")
	}
	pi, err := NewPipeInterface("race-pipe", "cat", true, time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	defer pi.Stop()

	var wg sync.WaitGroup
	const workers = 16
	const iters = 50
	for range workers {
		wg.Go(func() {
			payload := []byte{0x01, 0x02, 0x03, 0x04}
			for range iters {
				_ = pi.Send(payload, "")
			}
		})
	}
	wg.Wait()
}

func TestPipeInterfaceConcurrentStopSendRace(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not available")
	}
	pi, err := NewPipeInterface("stop-race", "cat", true, time.Second, false)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 100 {
			_ = pi.Send([]byte{0xAA}, "")
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		_ = pi.Stop()
	}()
	wg.Wait()
}

func TestLocalClientConcurrentSendRace(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	server, client := net.Pipe()
	lc := &LocalClientInterface{
		BaseInterface: NewBaseInterface("local-race", common.IFTypeUnix, true),
		conn:          client,
		done:          make(chan struct{}),
	}
	lc.MTU = 262144
	lc.Online = true

	go lc.readLoop()

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			frame := []byte{HDLCFlag, 0x42, HDLCFlag}
			for range 40 {
				_, _ = server.Write(frame)
			}
		})
	}
	wg.Wait()
	_ = lc.Stop()
	_ = server.Close()
}
