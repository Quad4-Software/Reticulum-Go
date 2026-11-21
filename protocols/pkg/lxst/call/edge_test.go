// SPDX-License-Identifier: LicenseRef-Reticulum
package call

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestApplyPreferredModeIgnoresInvalid(t *testing.T) {
	c := NewCall(nil, Config{UseAudio: false, Mode: proto.ModeFullDuplex})
	c.state.Store(int32(StateActive))
	c.applyPreferredMode(proto.PreferredMode + 0x0E)
	if c.Mode() != proto.ModeFullDuplex {
		t.Fatalf("mode %d", c.Mode())
	}
}

func TestHandleFrameAcceptsCodecOnly(t *testing.T) {
	c := NewCall(nil, Config{UseAudio: false})
	c.state.Store(int32(StateActive))
	c.incoming.Store(true)
	c.answered.Store(true)
	before := c.RecvFrames()
	c.handleFrame([]byte{proto.CodecOpus})
	if c.RecvFrames() != before+1 {
		t.Fatalf("recv %d want %d", c.RecvFrames(), before+1)
	}
}

func TestHandleFrameRejectsEmpty(t *testing.T) {
	c := NewCall(nil, Config{UseAudio: false})
	c.state.Store(int32(StateActive))
	c.incoming.Store(true)
	c.answered.Store(true)
	before := c.RecvFrames()
	c.handleFrame(nil)
	c.handleFrame([]byte{})
	if c.RecvFrames() != before {
		t.Fatalf("recv %d", c.RecvFrames())
	}
}

func TestPTTIgnoredBeforeActive(t *testing.T) {
	c := NewCall(nil, Config{UseAudio: false, Mode: proto.ModeHalfDuplex})
	c.PTT(true)
	if !c.Squelched() {
		t.Fatal("half duplex should stay squelched before active")
	}
	c.state.Store(int32(StateRinging))
	c.PTT(true)
	if !c.Squelched() {
		t.Fatal("PTT should not apply while ringing")
	}
}

func TestApplyModeSquelchResetsEarlyPTT(t *testing.T) {
	c := NewCall(nil, Config{UseAudio: false, Mode: proto.ModeHalfDuplex})
	c.state.Store(int32(StateActive))
	c.squelched.Store(false)
	c.applyModeSquelch()
	if !c.Squelched() {
		t.Fatal("expected squelch after applyModeSquelch in half duplex")
	}
}

func TestPTTActiveHalfDuplex(t *testing.T) {
	c := NewCall(nil, Config{UseAudio: false, Mode: proto.ModeHalfDuplex})
	c.state.Store(int32(StateActive))
	c.applyModeSquelch()
	c.PTT(true)
	if c.Squelched() {
		t.Fatal("PTT down should unsquelch when active")
	}
	c.PTT(false)
	if !c.Squelched() {
		t.Fatal("PTT up should squelch again")
	}
}

func TestSquelchBlocksTXInFullDuplex(t *testing.T) {
	c := NewCall(nil, Config{UseAudio: false, Mode: proto.ModeFullDuplex})
	c.state.Store(int32(StateActive))
	c.setSquelchState(true)
	if !c.Squelched() {
		t.Fatal("expected squelched")
	}
}
