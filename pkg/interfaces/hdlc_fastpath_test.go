// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"net"
	"runtime"
	"testing"
	"time"
)

func TestStreamReadSizeAtLeastChunk(t *testing.T) {
	if streamReadSize(500) != streamReadChunk {
		t.Fatalf("small MTU size=%d want %d", streamReadSize(500), streamReadChunk)
	}
	if streamReadSize(streamReadChunk+1) != streamReadChunk+1 {
		t.Fatalf("large MTU should win, got %d", streamReadSize(streamReadChunk+1))
	}
}

func TestAppendFrameHDLCNoInteriorFlag(t *testing.T) {
	payload := []byte{0x01, HDLCFlag, HDLCEsc, 0x02}
	frame := appendFrameHDLC(nil, payload)
	if frame[0] != HDLCFlag || frame[len(frame)-1] != HDLCFlag {
		t.Fatalf("missing flags %x", frame)
	}
	for i := 1; i < len(frame)-1; i++ {
		if frame[i] == HDLCFlag {
			t.Fatalf("unescaped flag at %d", i)
		}
	}
	legacy := append([]byte{HDLCFlag}, escapeHDLC(payload)...)
	legacy = append(legacy, HDLCFlag)
	if !bytes.Equal(frame, legacy) {
		t.Fatalf("appendFrameHDLC diverged from flag+escape+flag")
	}
}

func TestTCPHDLCDecoderBatchManyFrames(t *testing.T) {
	const n = 80
	var got [][]byte
	d := newTCPHDLCStreamDecoder(DefaultMTU, func(p []byte) {
		got = append(got, append([]byte(nil), p...))
	})
	var blob []byte
	want := make([][]byte, n)
	for i := range n {
		p := bytes.Repeat([]byte{byte(i + 1)}, 24)
		p[0], p[1] = HDLCFlag, HDLCEsc
		want[i] = p
		blob = appendFrameHDLC(blob, p)
	}
	d.feed(blob)
	if len(got) != n {
		t.Fatalf("got %d frames want %d", len(got), n)
	}
	for i := range n {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("frame %d mismatch", i)
		}
	}
}

func TestTCPHDLCDecoderDropsHeaderMinSize(t *testing.T) {
	var n int
	d := newTCPHDLCStreamDecoder(DefaultMTU, func([]byte) { n++ })
	d.feed(appendFrameHDLC(nil, bytes.Repeat([]byte{0x01}, 19)))
	if n != 0 {
		t.Fatalf("HEADER_MINSIZE frame delivered, n=%d", n)
	}
	d.feed(appendFrameHDLC(nil, bytes.Repeat([]byte{0x01}, 20)))
	if n != 1 {
		t.Fatalf("got %d want 1", n)
	}
}

func TestOracle_HDLCCallbackMustCopy(t *testing.T) {
	var aliased []byte
	d := newTCPHDLCStreamDecoder(DefaultMTU, func(p []byte) {
		aliased = p
	})
	first := bytes.Repeat([]byte{0x01}, 24)
	d.feed(appendFrameHDLC(nil, first))
	if aliased == nil {
		t.Fatal("first frame not delivered")
	}
	held := aliased
	second := bytes.Repeat([]byte{0x02}, 24)
	d.feed(appendFrameHDLC(nil, second))
	if bytes.Equal(held, first) {
		t.Fatal("callback slice was copied, assembler reuse contract changed")
	}
}

func TestHDLCResetNoGrowth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping HDLC leak-growth in short mode")
	}
	var blob []byte
	for i := range 64 {
		blob = appendFrameHDLC(blob, bytes.Repeat([]byte{byte(i + 1)}, 24))
	}
	d := newTCPHDLCStreamDecoder(DefaultMTU, func([]byte) {})
	d.feed(blob)
	runtime.GC()
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	for range 200 {
		d.reset()
		d.feed(blob)
	}
	runtime.GC()
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	if m2.HeapAlloc > m1.HeapAlloc+(2<<20) {
		t.Fatalf("HDLC reset/feed grew %d -> %d", m1.HeapAlloc, m2.HeapAlloc)
	}
}

func TestHDLCSiblingSweepSameBurst(t *testing.T) {
	const n = 16
	var blob []byte
	want := make([][]byte, n)
	for i := range n {
		p := bytes.Repeat([]byte{byte(i + 1)}, 24)
		p[0], p[1] = HDLCFlag, HDLCEsc
		want[i] = p
		blob = appendFrameHDLC(blob, p)
	}
	decoders := []struct {
		name string
		feed func(onFrame func([]byte)) func([]byte)
	}{
		{"tcp", func(on func([]byte)) func([]byte) {
			d := newTCPHDLCStreamDecoder(DefaultMTU, on)
			return d.feed
		}},
		{"toggle", func(on func([]byte)) func([]byte) {
			d := newHDLCToggleStreamDecoder(DefaultMTU, on)
			return d.feed
		}},
		{"serial", func(on func([]byte)) func([]byte) {
			d := newHDLCStreamDecoder(DefaultMTU, on)
			return d.feed
		}},
	}
	for _, dc := range decoders {
		t.Run(dc.name, func(t *testing.T) {
			var got [][]byte
			feed := dc.feed(func(p []byte) {
				got = append(got, append([]byte(nil), p...))
			})
			feed(blob)
			if len(got) != n {
				t.Fatalf("got %d frames want %d", len(got), n)
			}
			for i := range n {
				if !bytes.Equal(got[i], want[i]) {
					t.Fatalf("frame %d mismatch", i)
				}
			}
		})
	}
}

func TestTCPLoopbackBurstManyFrames(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	const n = 64
	got := make(chan []byte, n)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		decoder := newTCPHDLCStreamDecoder(DefaultMTU, func(data []byte) {
			got <- append([]byte(nil), data...)
		})
		buf := make([]byte, streamReadSize(DefaultMTU))
		for {
			nr, err := conn.Read(buf)
			if nr > 0 {
				decoder.feed(buf[:nr])
			}
			if err != nil {
				return
			}
		}
	}()

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })

	var blob []byte
	want := make([][]byte, n)
	for i := range n {
		p := bytes.Repeat([]byte{byte(i + 30)}, 32)
		want[i] = p
		blob = appendFrameHDLC(blob, p)
	}
	if _, err := c.Write(blob); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(3 * time.Second)
	var frames [][]byte
	for len(frames) < n {
		select {
		case f := <-got:
			frames = append(frames, f)
		case <-deadline:
			t.Fatalf("got %d frames want %d", len(frames), n)
		}
	}
	for i := range n {
		if !bytes.Equal(frames[i], want[i]) {
			t.Fatalf("frame %d mismatch", i)
		}
	}
}

func TestEnqueueWSMessageCaps(t *testing.T) {
	var q [][]byte
	for i := range maxWSMessageQueue + 40 {
		q = enqueueWSMessage(q, []byte{byte(i)})
	}
	if len(q) != maxWSMessageQueue {
		t.Fatalf("queue len=%d want %d", len(q), maxWSMessageQueue)
	}
}

func TestTCPHDLCDecoderFeedAllocBudget(t *testing.T) {
	var blob []byte
	for i := range 8 {
		blob = appendFrameHDLC(blob, bytes.Repeat([]byte{byte(i + 1)}, 24))
	}
	d := newTCPHDLCStreamDecoder(DefaultMTU, func([]byte) {})
	d.feed(blob)
	allocs := testing.AllocsPerRun(50, func() {
		d.reset()
		d.feed(blob)
	})
	if allocs > 1 {
		t.Fatalf("HDLC feed allocs=%.1f want <= 1", allocs)
	}
}

func BenchmarkTCPHDLCDecoderBurst(b *testing.B) {
	var blob []byte
	for range 32 {
		blob = appendFrameHDLC(blob, bytes.Repeat([]byte{0x42}, 64))
	}
	d := newTCPHDLCStreamDecoder(DefaultMTU, func([]byte) {})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.reset()
		d.feed(blob)
	}
}
