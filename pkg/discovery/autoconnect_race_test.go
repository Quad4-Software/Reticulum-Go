// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRacePersistLoadDiscoveredInterfaces(t *testing.T) {
	dir := t.TempDir()
	info := &ReceivedAnnounceInfo{
		Info: Info{
			Type:        "BackboneInterface",
			Name:        "race",
			ReachableOn: "192.0.2.44",
			Port:        4242,
			HasPort:     true,
			Transport:   true,
			TransportID: bytes.Repeat([]byte{0x11}, 16),
		},
		RemoteIdentity: bytes.Repeat([]byte{0x22}, 16),
	}
	var wg sync.WaitGroup
	var writes atomic.Int32
	for range 12 {
		wg.Go(func() {
			for range 40 {
				_ = PersistDiscoveredInterface(dir, info)
				writes.Add(1)
			}
		})
		wg.Go(func() {
			for range 40 {
				list, err := LoadPersistedInterfaces(dir)
				if err != nil {
					t.Error(err)
					return
				}
				if len(list) > 1 {
					t.Errorf("loaded %d entries want at most 1", len(list))
				}
			}
		})
	}
	wg.Wait()
	if writes.Load() == 0 {
		t.Fatal("no persist attempts")
	}
	list, err := LoadPersistedInterfaces(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("final loaded %d", len(list))
	}
}

func TestRaceEndpointHashConcurrent(t *testing.T) {
	info := &ReceivedAnnounceInfo{
		Info: Info{ReachableOn: "192.0.2.55", Port: 9001, HasPort: true},
	}
	want := EndpointHash(info)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 200 {
				got := EndpointHash(info)
				if !bytes.Equal(got, want) {
					t.Error("endpoint hash drift under race")
					return
				}
			}
		})
	}
	wg.Wait()
}
