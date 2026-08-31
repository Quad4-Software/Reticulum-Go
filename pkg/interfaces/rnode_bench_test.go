// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"bytes"
	"io"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func BenchmarkAppendRNodeDataFrame(b *testing.B) {
	payload := make([]byte, 200)
	for i := range payload {
		payload[i] = byte(i)
	}
	dst := make([]byte, 0, len(payload)*2+4)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst = appendRNodeDataFrame(dst[:0], payload)
	}
}

func BenchmarkRNodeDecodeDataFrame(b *testing.B) {
	payload := make([]byte, 200)
	for i := range payload {
		payload[i] = byte(i)
	}
	frame := appendRNodeDataFrame(nil, payload)
	var got int
	decoder := newRNodeCmdDecoder(rnodeHWMTU, func(cmd byte, data []byte) {
		if cmd == rnodeCmdData {
			got += len(data)
		}
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decoder.feed(frame)
	}
	if got == 0 {
		b.Fatal("decoder delivered no data")
	}
}

type discardSerial struct{}

func (d *discardSerial) Read([]byte) (int, error)    { return 0, io.EOF }
func (d *discardSerial) Write(p []byte) (int, error) { return len(p), nil }
func (d *discardSerial) Close() error                { return nil }

func BenchmarkRNodeProcessOutgoing(b *testing.B) {
	sink := &discardSerial{}
	r, err := NewRNodeInterface("bench", true, testRNodeOptions(NewRNodeSim(1)))
	if err != nil {
		b.Fatal(err)
	}
	r.port = sink
	r.Online = true
	r.interfaceReady = true
	payload := make([]byte, 200)
	for i := range payload {
		payload[i] = byte(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := r.ProcessOutgoing(payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRNodeMultiProcessOutgoing(b *testing.B) {
	sink := &discardSerial{}
	sub := &RNodeSubInterface{
		BaseInterface: NewBaseInterface("radio0", common.IFTypeRNode, true),
		parent: &RNodeMultiInterface{
			BaseInterface: NewBaseInterface("multi-bench", common.IFTypeRNodeMulti, true),
			port:          sink,
			txFrame:       make([]byte, 0, rnodeHWMTU*2+8),
		},
		index:          0,
		interfaceReady: true,
	}
	sub.Online = true
	sub.parent.Online = true
	payload := make([]byte, 200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := sub.ProcessOutgoing(payload); err != nil {
			b.Fatal(err)
		}
	}
}

func TestRNodeWireInteropMatchesPythonKISS(t *testing.T) {
	payload := []byte{0x01, KISSFend, 0x02, KISSFesc, 0x03}
	want := []byte{
		KISSFend, rnodeCmdData,
		0x01, KISSFesc, KISSTFend, 0x02, KISSFesc, KISSTFesc, 0x03,
		KISSFend,
	}
	got := appendRNodeDataFrame(nil, payload)
	if !bytes.Equal(got, want) {
		t.Fatalf("wire frame\n got %x\nwant %x", got, want)
	}

	var decoded []byte
	d := newRNodeCmdDecoder(rnodeHWMTU, func(cmd byte, data []byte) {
		if cmd == rnodeCmdData {
			decoded = append([]byte(nil), data...)
		}
	})
	d.feed(got)
	if !bytes.Equal(decoded, payload) {
		t.Fatalf("decode got %x want %x", decoded, payload)
	}
}

func TestRNodeBitrateMatchesPythonFormula(t *testing.T) {
	got := rnodeComputeBitrate(7, 5, 125000)
	want := 5468.75
	if got != want {
		t.Fatalf("bitrate got %v want %v", got, want)
	}
}
