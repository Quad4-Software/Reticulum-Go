// SPDX-License-Identifier: LicenseRef-Reticulum
package call_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestHalfDuplexStartsSquelched(t *testing.T) {
	c := call.NewCall(nil, call.Config{UseAudio: false, Mode: proto.ModeHalfDuplex})
	if !c.Squelched() {
		t.Fatal("half duplex call should start squelched")
	}
	if c.Mode() != proto.ModeHalfDuplex {
		t.Fatalf("mode %d", c.Mode())
	}
}

func TestPTTUnsquelchesHalfDuplex(t *testing.T) {
	c := call.NewCall(nil, call.Config{UseAudio: false, Mode: proto.ModeHalfDuplex})
	c.PTT(true)
	if !c.Squelched() {
		t.Fatal("PTT should not apply before call is active")
	}
}

func TestFullDuplexDefaultMode(t *testing.T) {
	c := call.NewCall(nil, call.Config{UseAudio: false, Mode: proto.ModeFullDuplex})
	if c.Mode() != proto.ModeFullDuplex {
		t.Fatalf("mode %d", c.Mode())
	}
}

func TestSwitchMode(t *testing.T) {
	c := call.NewCall(nil, call.Config{UseAudio: false})
	if err := c.SwitchMode(proto.ModeHalfDuplex); err != nil {
		t.Fatal(err)
	}
	if c.Mode() != proto.ModeHalfDuplex {
		t.Fatalf("mode %d", c.Mode())
	}
	if !c.Squelched() {
		t.Fatal("half duplex should squelch on switch")
	}
	if err := c.SwitchMode(proto.ModeFullDuplex); err != nil {
		t.Fatal(err)
	}
	if c.Mode() != proto.ModeFullDuplex {
		t.Fatalf("mode %d", c.Mode())
	}
}

func TestSwitchModeInvalid(t *testing.T) {
	c := call.NewCall(nil, call.Config{UseAudio: false})
	if err := c.SwitchMode(0x99); err != call.ErrInvalidMode {
		t.Fatalf("got %v", err)
	}
}

func TestSwitchProfileInvalid(t *testing.T) {
	c := call.NewCall(nil, call.Config{UseAudio: false})
	if err := c.SwitchProfile(0x99); err != call.ErrInvalidProfile {
		t.Fatalf("got %v", err)
	}
}
