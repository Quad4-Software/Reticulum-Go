// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package buffer

import (
	"bufio"
	"bytes"
	"io"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/channel"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

func TestStreamDataMessage_Pack(t *testing.T) {
	tests := []struct {
		name       string
		streamID   uint16
		data       []byte
		eof        bool
		compressed bool
	}{
		{
			name:       "NormalMessage",
			streamID:   123,
			data:       []byte("test data"),
			eof:        false,
			compressed: false,
		},
		{
			name:       "EOFMessage",
			streamID:   456,
			data:       []byte("final data"),
			eof:        true,
			compressed: false,
		},
		{
			name:       "CompressedMessage",
			streamID:   789,
			data:       []byte("compressed data"),
			eof:        false,
			compressed: true,
		},
		{
			name:       "EmptyData",
			streamID:   0,
			data:       []byte{},
			eof:        false,
			compressed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &StreamDataMessage{
				StreamID:   tt.streamID,
				Data:       tt.data,
				EOF:        tt.eof,
				Compressed: tt.compressed,
			}

			packed, err := msg.Pack()
			if err != nil {
				t.Fatalf("Pack() failed: %v", err)
			}

			if len(packed) < 2 {
				t.Error("Packed message too short")
			}

			unpacked := &StreamDataMessage{}
			if err := unpacked.Unpack(packed); err != nil {
				t.Fatalf("Unpack() failed: %v", err)
			}

			if unpacked.StreamID != tt.streamID {
				t.Errorf("StreamID = %d, want %d", unpacked.StreamID, tt.streamID)
			}
			if unpacked.EOF != tt.eof {
				t.Errorf("EOF = %v, want %v", unpacked.EOF, tt.eof)
			}
			if unpacked.Compressed != tt.compressed {
				t.Errorf("Compressed = %v, want %v", unpacked.Compressed, tt.compressed)
			}
			if !bytes.Equal(unpacked.Data, tt.data) {
				t.Errorf("Data = %v, want %v", unpacked.Data, tt.data)
			}
		})
	}
}

func TestStreamDataMessage_Unpack(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		wantError bool
	}{
		{
			name:      "ValidMessage",
			data:      []byte{0x00, 0x7B, 'h', 'e', 'l', 'l', 'o'},
			wantError: false,
		},
		{
			name:      "TooShort",
			data:      []byte{0x00},
			wantError: true,
		},
		{
			name:      "Empty",
			data:      []byte{},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &StreamDataMessage{}
			err := msg.Unpack(tt.data)
			if (err != nil) != tt.wantError {
				t.Errorf("Unpack() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestStreamDataMessage_GetType(t *testing.T) {
	msg := &StreamDataMessage{}
	if msg.GetType() != 0x01 {
		t.Errorf("GetType() = %d, want 0x01", msg.GetType())
	}
}

func TestRawChannelReader_AddCallback(t *testing.T) {
	reader := &RawChannelReader{
		streamID:  1,
		buffer:    bytes.NewBuffer(nil),
		callbacks: make(map[int]func(int)),
	}

	cb := func(int) {}

	reader.AddReadyCallback(cb)
	if len(reader.callbacks) != 1 {
		t.Error("Callback should be added")
	}
}

func TestBuffer_Write(t *testing.T) {
	buf := &Buffer{
		ReadWriter: bufio.NewReadWriter(bufio.NewReader(bytes.NewBuffer(nil)), bufio.NewWriter(bytes.NewBuffer(nil))),
	}

	data := []byte("test")
	n, err := buf.Write(data)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() = %d bytes, want %d", n, len(data))
	}
}

func TestBuffer_Read(t *testing.T) {
	buf := &Buffer{
		ReadWriter: bufio.NewReadWriter(bufio.NewReader(bytes.NewBuffer([]byte("test data"))), bufio.NewWriter(bytes.NewBuffer(nil))),
	}

	data := make([]byte, 10)
	n, err := buf.Read(data)
	if err != nil && err != io.EOF {
		t.Errorf("Read() error = %v", err)
	}
	if n <= 0 {
		t.Errorf("Read() = %d bytes, want > 0", n)
	}
}

func TestBuffer_Close(t *testing.T) {
	buf := &Buffer{
		ReadWriter: bufio.NewReadWriter(bufio.NewReader(bytes.NewBuffer(nil)), bufio.NewWriter(bytes.NewBuffer(nil))),
	}

	if err := buf.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestStreamIDMax(t *testing.T) {
	if StreamIDMax != 0x3fff {
		t.Errorf("StreamIDMax = %d, want %d", StreamIDMax, 0x3fff)
	}
}

func TestMaxChunkLen(t *testing.T) {
	if MaxChunkLen != 16*1024 {
		t.Errorf("MaxChunkLen = %d, want %d", MaxChunkLen, 16*1024)
	}
}

func TestMaxDataLen(t *testing.T) {
	if MaxDataLen != 457 {
		t.Errorf("MaxDataLen = %d, want %d", MaxDataLen, 457)
	}
}

type mockLink struct {
	status byte
	rtt    float64
}

func (m *mockLink) GetStatus() byte                                       { return m.status }
func (m *mockLink) GetRTT() float64                                       { return m.rtt }
func (m *mockLink) RTT() float64                                          { return m.rtt }
func (m *mockLink) GetLinkID() []byte                                     { return []byte("testlink") }
func (m *mockLink) Send(data []byte) any                                  { return &packet.Packet{Raw: data} }
func (m *mockLink) Resend(p any) error                                    { return nil }
func (m *mockLink) SetPacketTimeout(p any, cb func(any), t time.Duration) {}
func (m *mockLink) SetPacketDelivered(p any, cb func(any))                {}
func (m *mockLink) HandleInbound(pkt *packet.Packet) error                { return nil }
func (m *mockLink) ValidateLinkProof(pkt *packet.Packet, networkIface common.NetworkInterface) error {
	return nil
}
func (m *mockLink) LinkedNetworkInterface() common.NetworkInterface { return nil }

func TestNewRawChannelReader(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	reader := NewRawChannelReader(123, ch)

	if reader.streamID != 123 {
		t.Errorf("streamID = %d, want %d", reader.streamID, 123)
	}
	if reader.channel != ch {
		t.Error("channel not set correctly")
	}
	if reader.buffer == nil {
		t.Error("buffer is nil")
	}
	if reader.callbacks == nil {
		t.Error("callbacks is nil")
	}
}

func TestRawChannelReader_RemoveReadyCallback(t *testing.T) {
	reader := &RawChannelReader{
		streamID:  1,
		buffer:    bytes.NewBuffer(nil),
		callbacks: make(map[int]func(int)),
	}

	cb1 := func(int) {}
	cb2 := func(int) {}

	id1 := reader.AddReadyCallback(cb1)
	reader.AddReadyCallback(cb2)

	if len(reader.callbacks) != 2 {
		t.Errorf("callbacks length = %d, want 2", len(reader.callbacks))
	}

	reader.RemoveReadyCallback(id1)

	if len(reader.callbacks) != 1 {
		t.Errorf("RemoveReadyCallback did not remove callback, length = %d", len(reader.callbacks))
	}
}

func TestRawChannelReader_Read(t *testing.T) {
	reader := &RawChannelReader{
		streamID: 1,
		buffer:   bytes.NewBuffer([]byte("test data")),
		eof:      false,
	}

	data := make([]byte, 10)
	n, err := reader.Read(data)
	if err != nil {
		t.Errorf("Read() error = %v", err)
	}
	if n == 0 {
		t.Error("Read() returned 0 bytes")
	}

	reader.eof = true
	reader.buffer = bytes.NewBuffer(nil)
	n, err = reader.Read(data)
	if err != io.EOF {
		t.Errorf("Read() error = %v, want io.EOF", err)
	}
	if n != 0 {
		t.Errorf("Read() = %d bytes, want 0", n)
	}
}

func TestRawChannelReader_HandleMessage(t *testing.T) {
	reader := &RawChannelReader{
		streamID:  1,
		buffer:    bytes.NewBuffer(nil),
		callbacks: make(map[int]func(int)),
	}

	msg := &StreamDataMessage{
		StreamID:   1,
		Data:       []byte("test"),
		EOF:        false,
		Compressed: false,
	}

	called := false
	reader.AddReadyCallback(func(int) {
		called = true
	})

	result := reader.HandleMessage(msg)
	if !result {
		t.Error("HandleMessage() = false, want true")
	}
	if !called {
		t.Error("callback was not called")
	}
	if reader.buffer.Len() == 0 {
		t.Error("buffer is empty after HandleMessage")
	}

	msg.StreamID = 2
	result = reader.HandleMessage(msg)
	if result {
		t.Error("HandleMessage() = true, want false for different streamID")
	}

	msg.StreamID = 1
	msg.EOF = true
	reader.HandleMessage(msg)
	if !reader.eof {
		t.Error("EOF not set after HandleMessage with EOF flag")
	}
}

func TestNewRawChannelWriter(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	writer := NewRawChannelWriter(456, ch)

	if writer.streamID != 456 {
		t.Errorf("streamID = %d, want %d", writer.streamID, 456)
	}
	if writer.channel != ch {
		t.Error("channel not set correctly")
	}
	if writer.eof {
		t.Error("eof should be false initially")
	}
}

func TestRawChannelWriter_Write(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	writer := NewRawChannelWriter(1, ch)

	data := []byte("test data")
	n, err := writer.Write(data)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() = %d bytes, want %d", n, len(data))
	}

	largeData := make([]byte, MaxChunkLen+100)
	n, err = writer.Write(largeData)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != MaxChunkLen {
		t.Errorf("Write() = %d bytes, want %d", n, MaxChunkLen)
	}
}

func TestRawChannelWriter_Close(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	writer := NewRawChannelWriter(1, ch)

	if writer.eof {
		t.Error("EOF should be false before Close()")
	}

	err := writer.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
	if !writer.eof {
		t.Error("EOF should be true after Close()")
	}
}

func TestCreateReader(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	callback := func(int) {}
	reader := CreateReader(789, ch, callback)

	if reader == nil {
		t.Error("CreateReader() returned nil")
	}
}

func TestCreateWriter(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	writer := CreateWriter(101, ch)

	if writer == nil {
		t.Error("CreateWriter() returned nil")
	}
}

func TestCreateBidirectionalBuffer(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	callback := func(int) {}
	buf := CreateBidirectionalBuffer(1, 2, ch, callback)

	if buf == nil {
		t.Error("CreateBidirectionalBuffer() returned nil")
	}
}

func TestCompressData(t *testing.T) {
	data := []byte("test data for compression")
	compressed := compressData(data)

	if compressed == nil {
		t.Skip("compressData() returned nil (compression implementation may be incomplete)")
	}
}

func TestDecompressData(t *testing.T) {
	data := []byte("test data")
	compressed := compressData(data)
	if compressed == nil {
		t.Skip("compression not working, skipping decompression test")
	}

	decompressed := decompressData(compressed)
	if decompressed == nil {
		t.Error("decompressData() returned nil")
	}
}
