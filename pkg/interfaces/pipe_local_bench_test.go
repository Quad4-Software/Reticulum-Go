// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"io"
	"net"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

var benchPayload = []byte{
	0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
	0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
	0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
	0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
	0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27,
	0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f,
	0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37,
	0x38, 0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f,
}

func BenchmarkAppendFrameHDLC(b *testing.B) {
	dst := make([]byte, 0, len(benchPayload)*2+4)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst = appendFrameHDLC(dst[:0], benchPayload)
	}
}

func BenchmarkPipeProcessOutgoing(b *testing.B) {
	readEnd, writeEnd := io.Pipe()
	pi := &PipeInterface{
		BaseInterface: NewBaseInterface("bench-pipe", common.IFTypePipe, true),
		stdin:         writeEnd,
		txFrame:       make([]byte, 0, pipeHWMTU*2+4),
	}
	pi.MTU = pipeHWMTU
	pi.Online = true
	go func() {
		buf := make([]byte, pipeHWMTU*2+4)
		for {
			if _, err := readEnd.Read(buf); err != nil {
				return
			}
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := pi.ProcessOutgoing(benchPayload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPipeHDLCDecode(b *testing.B) {
	frame := appendFrameHDLC(nil, benchPayload)
	decoder := newHDLCStreamDecoder(pipeHWMTU, func([]byte) {})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decoder.reset()
		decoder.feed(frame)
	}
}

func BenchmarkLocalProcessOutgoing(b *testing.B) {
	readEnd, writeEnd := net.Pipe()
	lc := &LocalClientInterface{
		BaseInterface: NewBaseInterface("bench-local", common.IFTypeUnix, true),
		conn:          writeEnd,
		txFrame:       make([]byte, 0, 512),
	}
	lc.MTU = 262144
	lc.Online = true
	go func() {
		buf := make([]byte, 512)
		for {
			if _, err := readEnd.Read(buf); err != nil {
				return
			}
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := lc.ProcessOutgoing(benchPayload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLocalHDLCDecode(b *testing.B) {
	frame := appendFrameHDLC(nil, benchPayload)
	decoder := newHDLCStreamDecoder(4096, func([]byte) {})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decoder.reset()
		decoder.feed(frame)
	}
}
