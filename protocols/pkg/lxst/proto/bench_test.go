// SPDX-License-Identifier: LicenseRef-Reticulum
package proto_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func BenchmarkPackSignalling(b *testing.B) {
	sigs := []int{proto.StatusAvailable, proto.StatusRinging}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := proto.PackSignalling(sigs); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnpackSignalling(b *testing.B) {
	raw, err := proto.PackSignalling([]int{proto.StatusAvailable})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := proto.Unpack(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPackUnpackFrame(b *testing.B) {
	payload := make([]byte, 80)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		raw, err := proto.PackFrame(proto.CodecOpus, payload)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := proto.Unpack(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func TestUnpackAllocsBounded(t *testing.T) {
	raw, err := proto.PackSignalling([]int{proto.StatusAvailable, proto.StatusRinging})
	if err != nil {
		t.Fatal(err)
	}
	n := testing.AllocsPerRun(200, func() {
		_, _ = proto.Unpack(raw)
	})
	if n > 16 {
		t.Fatalf("unpack allocs %v", n)
	}
}
