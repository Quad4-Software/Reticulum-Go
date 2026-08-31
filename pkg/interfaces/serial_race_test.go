// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestSerialRaceStopSend(t *testing.T) {
	pair := newMemSerialPair()
	aPort, bPort := pair.ends()
	a, err := NewSerialInterface("a", true, SerialOptions{
		Device:         "a",
		ReconnectDelay: 20 * time.Millisecond,
		Open:           func(SerialOptions) (SerialPort, error) { return aPort, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSerialInterface("b", true, SerialOptions{
		Device: "b",
		Open:   func(SerialOptions) (SerialPort, error) { return bPort, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	var n atomic.Int32
	b.SetPacketCallback(func([]byte, common.NetworkInterface) { n.Add(1) })

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 100 {
			_ = a.Send([]byte{byte(i)}, "")
			time.Sleep(time.Millisecond)
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(30 * time.Millisecond)
		_ = a.Stop()
	}()
	wg.Wait()
	_ = n.Load()
}
