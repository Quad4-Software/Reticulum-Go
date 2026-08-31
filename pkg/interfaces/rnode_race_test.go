// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"sync"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestRNodeConcurrentSendAndIncoming(t *testing.T) {
	sim := NewRNodeSim(1)
	r, err := NewRNodeInterface("race", true, testRNodeOptions(sim))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	r.SetPacketCallback(func([]byte, common.NetworkInterface) {})

	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func(seed byte) {
			defer wg.Done()
			for iteration := range 100 {
				_ = r.Send([]byte{seed, byte(iteration)}, "")
				sim.enqueue(appendRNodeDataFrame(nil, []byte{byte(iteration), seed}))
			}
		}(byte(worker))
	}
	wg.Wait()
}
