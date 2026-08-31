// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func testRNodeOptions(sim *RNodeSim) RNodeOptions {
	return RNodeOptions{
		Port:           "sim",
		Frequency:      915000000,
		Bandwidth:      125000,
		TXPower:        10,
		SF:             7,
		CR:             5,
		ConfigureDelay: time.Millisecond,
		ValidateDelay:  10 * time.Millisecond,
		DetectTimeout:  100 * time.Millisecond,
		Open: func(SerialOptions) (SerialPort, error) {
			return sim, nil
		},
	}
}

func TestRNodeRoundTripViaSim(t *testing.T) {
	aSim := NewRNodeSim(1)
	bSim := NewRNodeSim(1)
	PairRNodeSims(aSim, bSim)
	a, err := NewRNodeInterface("a", true, testRNodeOptions(aSim))
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewRNodeInterface("b", true, testRNodeOptions(bSim))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	want := []byte{0x01, KISSFend, KISSFesc, 0x04}
	got := make(chan []byte, 1)
	b.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got <- append([]byte(nil), data...)
	})
	if err := a.Send(want, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-got:
		if !bytes.Equal(data, want) {
			t.Fatalf("received %x, want %x", data, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for RNode packet")
	}
}

func TestRNodeConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*RNodeOptions)
	}{
		{"frequency", func(o *RNodeOptions) { o.Frequency = 1 }},
		{"txpower", func(o *RNodeOptions) { o.TXPower = 38 }},
		{"bandwidth", func(o *RNodeOptions) { o.Bandwidth = 100 }},
		{"sf", func(o *RNodeOptions) { o.SF = 13 }},
		{"cr", func(o *RNodeOptions) { o.CR = 9 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := testRNodeOptions(NewRNodeSim(1))
			test.change(&opts)
			if _, err := NewRNodeInterface("invalid", true, opts); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestRNodeFlowControlReady(t *testing.T) {
	sim := NewRNodeSim(1)
	sim.AutoReady = false
	opts := testRNodeOptions(sim)
	opts.FlowControl = true
	r, err := NewRNodeInterface("flow", true, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	var mu sync.Mutex
	var packets [][]byte
	r.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		mu.Lock()
		packets = append(packets, append([]byte(nil), data...))
		mu.Unlock()
	})
	if err := r.Send([]byte("one"), ""); err != nil {
		t.Fatal(err)
	}
	if err := r.Send([]byte("two"), ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	before := len(packets)
	mu.Unlock()
	if before != 1 {
		t.Fatalf("received %d packets before READY, want 1", before)
	}
	sim.enqueue(appendRNodeFrame(nil, rnodeCmdReady, []byte{1}))
	waitForRNodePackets(t, &mu, &packets, 2)
}

func TestRNodeMultiSubinterfaceTXRX(t *testing.T) {
	sim := NewRNodeSim(2)
	subCfg := &common.InterfaceConfig{
		Name: "radio1", Enabled: true, VPort: 1, VPortSet: true,
		FrequencyHz: 915000000, Bandwidth: 125000, TXPower: 10,
		SpreadingFactor: 7, CodingRate: 5,
	}
	opts := RNodeMultiOptions{
		RNodeOptions:  testRNodeOptions(sim),
		SubInterfaces: map[string]*common.InterfaceConfig{"radio1": subCfg},
	}
	m, err := NewRNodeMultiInterface("multi", true, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	sub := m.SubInterfaces()[1]
	if sub == nil {
		t.Fatal("vport 1 was not created")
	}
	got := make(chan []byte, 1)
	sub.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got <- append([]byte(nil), data...)
	})
	want := []byte("multi-data")
	if err := sub.Send(want, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-got:
		if !bytes.Equal(data, want) {
			t.Fatalf("received %q, want %q", data, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for multi packet")
	}
}

func waitForRNodePackets(t *testing.T, mu *sync.Mutex, packets *[][]byte, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := len(*packets)
		mu.Unlock()
		if got >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d packets", count)
}
