// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !rns_slim

package interfaces

import (
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestI2PRaceStopSend(t *testing.T) {
	addr, cleanup := fakeSAMForDial(t, false, nil)
	defer cleanup()

	parent, err := NewI2PInterface("i2p_race", &common.InterfaceConfig{
		Type:          "I2PInterface",
		Enabled:       true,
		I2PSAMAddress: addr,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.controller.Stop()

	peer := NewI2PInterfacePeer(parent, "i2p_race to dest",
		"abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrst.b32.i2p", 2, parent.cfg)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if peer.IsOnline() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 40 {
			_ = peer.ProcessOutgoing([]byte{0x7e, byte(i), 0x7e})
			time.Sleep(2 * time.Millisecond)
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(30 * time.Millisecond)
		_ = peer.Stop()
		_ = parent.Stop()
	}()
	wg.Wait()
}
