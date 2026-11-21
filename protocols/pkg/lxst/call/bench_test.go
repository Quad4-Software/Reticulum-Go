// SPDX-License-Identifier: LicenseRef-Reticulum
package call

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func BenchmarkPackFrameIntoHotPath(b *testing.B) {
	payload := make([]byte, 80)
	var wire []byte
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		wire, err = proto.PackFrameInto(wire[:0], proto.CodecOpus, payload)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestSendMediaTickPackAllocs(t *testing.T) {
	payload := make([]byte, 80)
	wire := make([]byte, 0, 128)
	n := testing.AllocsPerRun(500, func() {
		var err error
		wire, err = proto.PackFrameInto(wire[:0], proto.CodecOpus, payload)
		if err != nil {
			t.Fatal(err)
		}
	})
	if n != 0 {
		t.Fatalf("PackFrameInto hot path allocs %f", n)
	}
}
