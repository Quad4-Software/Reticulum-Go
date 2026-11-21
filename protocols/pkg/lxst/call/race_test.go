// SPDX-License-Identifier: LicenseRef-Reticulum
package call_test

import (
	"sync"
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
)

func TestRaceStateAndHangup(t *testing.T) {
	c := call.NewCall(nil, call.Config{UseAudio: false})
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for range 200 {
			_ = c.State()
			_ = c.Status()
			_ = c.Profile()
			_ = c.Incoming()
			_ = c.RecvFrames()
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 200 {
			_ = c.Hangup("race")
			_ = c.Mute(i%2 == 0)
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			_ = c.RemoteIdentity()
		}
	}()
	wg.Wait()
}

func TestRaceOccupyRelease(t *testing.T) {
	sb := call.NewSwitchboard(nil, call.Config{UseAudio: false}, nil)
	var wg sync.WaitGroup
	wg.Add(8)
	for range 8 {
		go func() {
			defer wg.Done()
			c := call.NewCall(nil, call.Config{UseAudio: false})
			if sb.Occupy(c) {
				time.Sleep(time.Millisecond)
				sb.Release(c)
			}
		}()
	}
	wg.Wait()
}
