// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"net"
	"testing"
)

func TestTCPClientReconnectSynthesizesTunnel(t *testing.T) {
	tc, err := NewTCPClientInterfaceWithRetries("t", "127.0.0.1", 1, false, false, false, -1)
	if err != nil {
		t.Fatal(err)
	}
	tc.SetWantsTunnel(true)
	var called bool
	tc.SetTunnelSynth(func(p TunnelPeer) {
		called = true
		if p != tc {
			t.Fatal("tunnel peer mismatch")
		}
	})
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	tc.onConnected(c1)
	if !called {
		t.Fatal("expected tunnel synthesis on connect")
	}
	_ = tc.Stop()
}

func TestTCPClientSkipsTunnelWhenKISS(t *testing.T) {
	tc, err := NewTCPClientInterfaceWithRetries("t", "127.0.0.1", 1, true, false, false, -1)
	if err != nil {
		t.Fatal(err)
	}
	var called bool
	tc.SetTunnelSynth(func(TunnelPeer) { called = true })
	if tc.WantsTunnel() {
		t.Fatal("kiss framing should disable tunnel by default")
	}
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	tc.onConnected(c1)
	if called {
		t.Fatal("did not expect tunnel synthesis with kiss framing")
	}
	_ = tc.Stop()
}
