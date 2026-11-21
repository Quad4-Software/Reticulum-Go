// SPDX-License-Identifier: LicenseRef-Reticulum
package smoke_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/io"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestSmokePackAvailable(t *testing.T) {
	raw, err := proto.PackSignalling([]int{proto.StatusAvailable})
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("empty wire")
	}
	pkt, err := proto.Unpack(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt.Signals) != 1 || pkt.Signals[0] != proto.StatusAvailable {
		t.Fatalf("%+v", pkt.Signals)
	}
}

func TestSmokeNullDevice(t *testing.T) {
	dev := io.NewNullDevice()
	if err := dev.Start(); err != nil {
		t.Fatal(err)
	}
	pcm, err := dev.ReadPCM()
	if err != nil || len(pcm) == 0 {
		t.Fatalf("read %v %d", err, len(pcm))
	}
	_ = dev.Close()
}

func TestSmokeNewCallIdle(t *testing.T) {
	c := call.NewCall(nil, call.Config{UseAudio: false, Profile: proto.DefaultProfile})
	if c.State() != call.StateIdle {
		t.Fatalf("state %v", c.State())
	}
	if c.Profile() != proto.DefaultProfile {
		t.Fatalf("profile %d", c.Profile())
	}
}
