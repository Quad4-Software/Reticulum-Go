// SPDX-License-Identifier: LicenseRef-Reticulum
package proto_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestPackSignallingAllocs(t *testing.T) {
	sig := []int{proto.StatusAvailable, proto.StatusRinging, proto.SignalPreferredProfile(proto.DefaultProfile)}
	n := testing.AllocsPerRun(200, func() {
		_, _ = proto.PackSignalling(sig)
	})
	if n > 12 {
		t.Fatalf("PackSignalling allocs %f", n)
	}
}

func TestPackFrameAllocs(t *testing.T) {
	payload := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	n := testing.AllocsPerRun(200, func() {
		_, _ = proto.PackFrame(proto.CodecOpus, payload)
	})
	if n > 1 {
		t.Fatalf("PackFrame allocs %f", n)
	}
}

func TestPackFrameIntoZeroAlloc(t *testing.T) {
	payload := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	buf := make([]byte, 0, 64)
	n := testing.AllocsPerRun(200, func() {
		var err error
		buf, err = proto.PackFrameInto(buf[:0], proto.CodecOpus, payload)
		if err != nil {
			t.Fatal(err)
		}
	})
	if n != 0 {
		t.Fatalf("PackFrameInto allocs %f", n)
	}
}
