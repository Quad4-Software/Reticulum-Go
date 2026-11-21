// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestChaos_ParallelMarshal(t *testing.T) {
	if testing.Short() {
		t.Skip("chaos skipped in -short")
	}
	const workers = 32
	const iters = 200
	var wg sync.WaitGroup
	var fails atomic.Int64
	for w := range workers {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			sender := make([]byte, IdentityLength)
			for i := range sender {
				sender[i] = byte(seed + i)
			}
			for range iters {
				env, err := NewEnvelope(TypeMsg, sender)
				if err != nil {
					fails.Add(1)
					return
				}
				env.Room = "#chaos"
				env.HasRoom = true
				env.Body = "x"
				env.HasBody = true
				raw, err := env.Marshal()
				if err != nil {
					fails.Add(1)
					return
				}
				if _, err := UnmarshalEnvelope(raw); err != nil {
					fails.Add(1)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	if fails.Load() != 0 {
		t.Fatalf("failures=%d", fails.Load())
	}
}

func TestChaos_HubConcurrentClients(t *testing.T) {
	if testing.Short() {
		t.Skip("chaos skipped in -short")
	}

	m := newTestMesh(t, 42720, HubConfig{
		Name: "chaos-hub",
		Limits: HubLimits{
			RateLimitMsgsPerMinute: 600,
			MaxMsgBodyBytes:        200,
		},
		IncludeMemberList: true,
	})

	joinedA := make(chan struct{}, 8)
	joinedB := make(chan struct{}, 8)
	a := dialMeshClient(t, m, 'A', ClientConfig{
		Nick: "A",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#chaos" {
					select {
					case joinedA <- struct{}{}:
					default:
					}
				}
			},
		},
	})
	b := dialMeshClient(t, m, 'B', ClientConfig{
		Nick: "B",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#chaos" {
					select {
					case joinedB <- struct{}{}:
					default:
					}
				}
			},
		},
	})

	var wg sync.WaitGroup
	var errs atomic.Int64
	type job struct {
		c      *Client
		joined <-chan struct{}
		label  string
	}
	for _, j := range []job{
		{a, joinedA, "A"},
		{b, joinedB, "B"},
	} {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			if err := j.c.Join("#chaos"); err != nil {
				errs.Add(1)
				return
			}
			select {
			case <-j.joined:
			case <-time.After(10 * time.Second):
				errs.Add(1)
				return
			}
			for range 20 {
				if err := j.c.SendMsg("#chaos", "ping"); err != nil {
					errs.Add(1)
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
			if err := j.c.Part("#chaos"); err != nil {
				errs.Add(1)
			}
		}(j)
	}
	wg.Wait()
	if errs.Load() != 0 {
		t.Fatalf("client errors=%d", errs.Load())
	}
}

func TestStress_RRC_RateLimitWindow(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	h := &Hub{cfg: HubConfig{Limits: HubLimits{RateLimitMsgsPerMinute: 5}}}
	p := &hubPeer{}
	ok := 0
	for range 10 {
		if h.allowRate(p) {
			ok++
		}
	}
	if ok != 5 {
		t.Fatalf("allowed=%d want 5", ok)
	}
}
