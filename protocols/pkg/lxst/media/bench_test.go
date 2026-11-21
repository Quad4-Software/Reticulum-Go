// SPDX-License-Identifier: LicenseRef-Reticulum
package media_test

import (
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/media"
)

func BenchmarkJitterPushPop(b *testing.B) {
	jb := media.NewJitterBuffer(40, 32)
	payload := []byte{1, 2, 3, 4}
	now := time.Now()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		jb.Push(uint16(i), uint32(i), payload)
		_, _ = jb.PopReady(now.Add(time.Duration(i) * time.Millisecond))
	}
}

func TestJitterPushOwnedNoCopy(t *testing.T) {
	jb := media.NewJitterBuffer(40, 32)
	payload := []byte{1, 2, 3}
	jb.PushOwned(1, 1, payload)
	payload[0] = 9
	frame, ok := jb.PopReady(time.Now())
	if !ok {
		t.Fatal("missing frame")
	}
	if frame.Payload[0] != 9 {
		t.Fatalf("payload %v", frame.Payload)
	}
}

func TestJitterPushAllocs(t *testing.T) {
	jb := media.NewJitterBuffer(40, 32)
	payload := []byte{1, 2, 3, 4}
	n := testing.AllocsPerRun(200, func() {
		jb.Push(1, 1, payload)
	})
	if n > 3 {
		t.Fatalf("push allocs %v", n)
	}
}
