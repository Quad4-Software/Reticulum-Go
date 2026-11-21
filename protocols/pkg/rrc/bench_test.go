// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"bytes"
	"testing"
	"time"
)

func BenchmarkRRC_Marshal(b *testing.B) {
	sender := bytes.Repeat([]byte{0x9c}, IdentityLength)
	env, err := NewEnvelope(TypeMsg, sender)
	if err != nil {
		b.Fatal(err)
	}
	env.Room = "#lobby"
	env.HasRoom = true
	env.Body = "Hello, RRC benchmark."
	env.HasBody = true
	env.Nick = "alice"
	env.HasNick = true
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := env.Marshal(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRRC_Unmarshal(b *testing.B) {
	sender := bytes.Repeat([]byte{0x9c}, IdentityLength)
	env, err := NewEnvelope(TypeMsg, sender)
	if err != nil {
		b.Fatal(err)
	}
	env.Room = "#lobby"
	env.HasRoom = true
	env.Body = "Hello, RRC benchmark."
	env.HasBody = true
	raw, err := env.Marshal()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := UnmarshalEnvelope(raw); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHubFanout measures the full hub relay path over the loopback
// mesh: inbound unmarshal, rate limiting, member lookup, forward marshal,
// and link send for each recipient.
func BenchmarkHubFanout(b *testing.B) {
	m := newTestMesh(b, 43300, HubConfig{
		Limits: HubLimits{
			RateLimitMsgsPerMinute: 1 << 50,
			MaxMsgBodyBytes:        1 << 20,
		},
	})
	joinedA := make(chan struct{}, 4)
	joinedB := make(chan struct{}, 4)
	a := dialMeshClient(b, m, 'A', ClientConfig{
		Nick: "alice",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#bench" {
					joinedA <- struct{}{}
				}
			},
		},
	})
	dialMeshClient(b, m, 'B', ClientConfig{
		Nick: "bob",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#bench" {
					joinedB <- struct{}{}
				}
			},
		},
	})
	if err := a.Join("#bench"); err != nil {
		b.Fatal(err)
	}
	select {
	case <-joinedA:
	case <-time.After(10 * time.Second):
		b.Fatal("join timeout")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := a.SendMsg("#bench", "fanout bench message"); err != nil {
			b.Fatal(err)
		}
	}
}
