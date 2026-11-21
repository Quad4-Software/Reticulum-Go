// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"sync"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

func TestRace_AllowJoinAndFlags(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	founder := mustID(1)
	reg.Ensure("r", founder, true)
	var wg sync.WaitGroup
	for n := range 32 {
		wg.Add(1)
		go func(seed byte) {
			defer wg.Done()
			peer := mustID(seed)
			for i := range 80 {
				_ = reg.AllowJoin("r", peer, nil, false)
				_ = reg.SetFlag("r", "m", i%2 == 0, "")
				_ = reg.SetFlag("r", "n", i%3 == 0, "")
				_ = reg.AllowContent("r", peer, true, false)
				_ = reg.ModeString("r")
				reg.Touch("r")
			}
		}(byte(n + 2))
	}
	wg.Wait()
}

func TestRace_BanUnbanInvite(t *testing.T) {
	reg := NewRoomRegistry("", 900)
	founder := mustID(1)
	target := mustID(2)
	reg.Ensure("r", founder, true)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 100 {
				_ = reg.AddBan("r", target)
				_ = reg.IsBanned("r", target)
				_ = reg.DelBan("r", target)
				_ = reg.AddInvite("r", target, 900)
				_ = reg.IsInvited("r", target)
				reg.ConsumeInvite("r", target)
			}
		})
	}
	wg.Wait()
}

func TestRace_TrustAndStats(t *testing.T) {
	tr := NewTrust()
	st := NewStats()
	id := mustID(4)
	_ = tr.Load([]string{id.Hex()}, nil)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 200 {
				_ = tr.IsTrusted(id)
				tr.Ban(id)
				_ = tr.IsBanned(id)
				_ = tr.Unban(id)
				st.Inc("joins", 1)
				st.Inc("pkts_in", 1)
				st.Inc("bytes_in", 3)
				_, _ = tr.Counts()
			}
		})
	}
	wg.Wait()
}

func TestRace_ServicePolicy(t *testing.T) {
	s := NewService(testConfig(), nil)
	var wg sync.WaitGroup
	for n := range 16 {
		wg.Add(1)
		go func(seed byte) {
			defer wg.Done()
			peer := mustPeer(seed)
			env, err := rrc.NewEnvelope(rrc.TypeMsg, peer)
			if err != nil {
				return
			}
			env.Room = "race"
			env.HasRoom = true
			env.Body = "hi"
			env.HasBody = true
			for range 60 {
				_ = s.AllowJoin(peer, "race", nil)
				s.AfterJoin(peer, "race")
				_ = s.AllowContent(peer, env)
				s.handleCommand(peer, "race", "/list")
				s.AfterPart(peer, "race")
				s.OnPong(peer)
			}
		}(byte(n + 10))
	}
	wg.Wait()
}
