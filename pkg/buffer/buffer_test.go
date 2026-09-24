// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package buffer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math/rand"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/channel"
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/bzip2/pkg/bzip2"
	"github.com/Quad4-Software/pbt/pkg/pbt"
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
	if msg.GetType() != 0xff00 {
		t.Errorf("GetType() = %d, want 0xff00", msg.GetType())
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
	status    byte
	rtt       float64
	mdu       int
	noDeliver bool

	sentMu sync.Mutex
	sent   [][]byte

	delivered map[any]func(any)
}

func (m *mockLink) GetStatus() byte { return m.status }
func (m *mockLink) GetRTT() float64 { return m.rtt }
func (m *mockLink) RTT() float64    { return m.rtt }
func (m *mockLink) GetMDU() int {
	if m.mdu > 0 {
		return m.mdu
	}
	return channel.DefaultOutletMDU
}
func (m *mockLink) GetLinkID() []byte { return []byte("testlink") }
func (m *mockLink) Send(data []byte) any {
	m.sentMu.Lock()
	m.sent = append(m.sent, data)
	m.sentMu.Unlock()
	return &packet.Packet{Raw: data}
}
func (m *mockLink) Resend(p any) error                                    { return nil }
func (m *mockLink) SetPacketTimeout(p any, cb func(any), t time.Duration) {}
func (m *mockLink) SetPacketDelivered(p any, cb func(any)) {
	if m.noDeliver {
		if m.delivered == nil {
			m.delivered = make(map[any]func(any))
		}
		m.delivered[p] = cb
		return
	}
	if cb != nil {
		cb(p)
	}
}

func (m *mockLink) sentFrames() [][]byte {
	m.sentMu.Lock()
	defer m.sentMu.Unlock()
	return append([][]byte(nil), m.sent...)
}
func (m *mockLink) HandleInbound(pkt *packet.Packet) error { return nil }
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

	// Highly compressible: Python/Go compress the full MaxChunkLen prefix.
	zeros := make([]byte, MaxChunkLen+100)
	n, err = writer.Write(zeros)
	if err != nil {
		t.Errorf("Write() zeros error = %v", err)
	}
	if n != MaxChunkLen {
		t.Errorf("Write() zeros = %d bytes, want %d (compressed full chunk)", n, MaxChunkLen)
	}

	// Incompressible: falls back to an uncompressed slice bounded by the live
	// channel MDU minus the stream header, like Python RawChannelWriter._mdu.
	want := ch.MDU() - StreamHeaderSize
	incomp := make([]byte, MaxChunkLen)
	for i := range incomp {
		incomp[i] = byte(i)
	}
	n, err = writer.Write(incomp)
	if err != nil {
		t.Errorf("Write() incompressible error = %v", err)
	}
	if n != want {
		t.Errorf("Write() incompressible = %d bytes, want channel-derived %d", n, want)
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
		t.Fatal("compressData() returned nil")
	}
	if bytes.Equal(compressed, data) {
		t.Fatal("compressData() returned uncompressed input")
	}
}

func TestDecompressData(t *testing.T) {
	data := []byte("test data")
	compressed := compressData(data)
	if compressed == nil {
		t.Fatal("compressData() returned nil")
	}

	decompressed := decompressData(compressed)
	if decompressed == nil {
		t.Fatal("decompressData() returned nil")
	}
	if !bytes.Equal(decompressed, data) {
		t.Fatalf("roundtrip mismatch: got %q want %q", decompressed, data)
	}
}

func genStreamDataMessage(r *rand.Rand, size int) StreamDataMessage {
	maxData := 8192
	if size > 0 && size*80 < maxData {
		maxData = size * 80
	}
	if maxData < 0 {
		maxData = 0
	}
	n := r.Intn(maxData + 1)
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(r.Intn(256))
	}
	return StreamDataMessage{
		StreamID:   uint16(r.Intn(0x4000)),
		Data:       data,
		EOF:        r.Intn(2) == 1,
		Compressed: r.Intn(2) == 1,
	}
}

func TestPBTStreamDataMessageRoundTrip(t *testing.T) {
	gen := pbt.NewGenerator("streamData", genStreamDataMessage)
	prop := pbt.ForAll(
		"pack unpack preserves stream fields",
		gen,
		func(orig StreamDataMessage) bool {
			raw, err := orig.Pack()
			if err != nil {
				return false
			}
			var got StreamDataMessage
			if err := got.Unpack(raw); err != nil {
				return false
			}
			wantID := orig.StreamID & StreamIDMax
			return got.StreamID == wantID && got.EOF == orig.EOF && got.Compressed == orig.Compressed &&
				bytes.Equal(got.Data, orig.Data)
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(100), pbt.WithSeed(5), pbt.WithMaxSize(120))
}

func bz2Bomb(t *testing.T, decompressedLen int) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := bzip2.NewWriter(&buf, 9)
	if err != nil {
		t.Fatalf("bzip2.NewWriter: %v", err)
	}
	zeros := make([]byte, 64*1024)
	remaining := decompressedLen
	for remaining > 0 {
		n := min(len(zeros), remaining)
		if _, err := w.Write(zeros[:n]); err != nil {
			t.Fatalf("bzip2 write: %v", err)
		}
		remaining -= n
	}
	if err := w.Close(); err != nil {
		t.Fatalf("bzip2 close: %v", err)
	}
	return buf.Bytes()
}

func bz2Stream(t *testing.T, plaintext []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := bzip2.NewWriter(&buf, 9)
	if err != nil {
		t.Fatalf("bzip2.NewWriter: %v", err)
	}
	if _, err := w.Write(plaintext); err != nil {
		t.Fatalf("bzip2 write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("bzip2 close: %v", err)
	}
	return buf.Bytes()
}

func TestDecompressData_RejectsBombPastMaxChunkLen(t *testing.T) {
	const huge = 8 * 1024 * 1024
	bomb := bz2Bomb(t, huge)
	if len(bomb) > 4096 {
		t.Fatalf("bomb stream unexpectedly large: %d bytes", len(bomb))
	}
	t.Logf("bomb stream: %d bytes compressed -> %d bytes decompressed (ratio %dx)",
		len(bomb), huge, huge/len(bomb))

	out := decompressData(bomb)
	if out != nil {
		t.Fatalf("expected nil for bz2 bomb past MaxChunkLen, got %d bytes", len(out))
	}
}

func TestDecompressData_RoundTripsHonestPayload(t *testing.T) {
	plaintext := bytes.Repeat([]byte("buf-test-payload"), 64)
	compressed := bz2Stream(t, plaintext)
	out := decompressData(compressed)
	if !bytes.Equal(out, plaintext) {
		t.Fatalf("roundtrip mismatch: got %d bytes, want %d", len(out), len(plaintext))
	}
}

func TestDecompressData_AcceptsPythonBz2Compress(t *testing.T) {
	plaintext := bytes.Repeat([]byte("py-go-buffer-interop-"), 64)
	cmd := exec.Command("python3", "-c",
		"import bz2,sys; sys.stdout.buffer.write(bz2.compress(sys.stdin.buffer.read()))")
	cmd.Stdin = bytes.NewReader(plaintext)
	compressed, err := cmd.Output()
	if err != nil {
		t.Fatalf("python bz2.compress: %v", err)
	}
	got := decompressData(compressed)
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Go decompress of Python bz2 mismatch: got %d want %d", len(got), len(plaintext))
	}
}

func TestCompressData_PythonCanDecompress(t *testing.T) {
	plaintext := bytes.Repeat([]byte("go-py-buffer-interop-"), 64)
	compressed := compressData(plaintext)
	if compressed == nil {
		t.Fatal("compressData returned nil")
	}
	cmd := exec.Command("python3", "-c",
		"import bz2,sys; sys.stdout.buffer.write(bz2.decompress(sys.stdin.buffer.read()))")
	cmd.Stdin = bytes.NewReader(compressed)
	got, err := cmd.Output()
	if err != nil {
		t.Fatalf("python bz2.decompress of Go stream: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Python decompress of Go bz2 mismatch: got %d want %d", len(got), len(plaintext))
	}
}

func TestRawChannelReader_HandleMessage_BombCannotOverflowBuffer(t *testing.T) {
	const huge = 8 * 1024 * 1024
	bomb := bz2Bomb(t, huge)

	link := &mockLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	reader := NewRawChannelReader(7, ch)

	msg := &StreamDataMessage{
		StreamID:   7,
		Data:       bomb,
		EOF:        true,
		Compressed: true,
	}

	if !reader.HandleMessage(msg) {
		t.Fatalf("HandleMessage returned false for matching streamID")
	}

	if reader.buffer.Len() != 0 {
		t.Fatalf("reader buffer holds %d bytes after rejected bomb, want 0", reader.buffer.Len())
	}
	if !reader.eof {
		t.Fatal("EOF flag must still be set when compressed bomb is rejected")
	}
	n, err := reader.Read(make([]byte, 8))
	if n != 0 || err != io.EOF {
		t.Fatalf("Read after rejected EOF bomb: n=%d err=%v want 0, io.EOF", n, err)
	}
}

func TestRawChannelReader_HandleMessage_HonestCompressedFlows(t *testing.T) {
	plaintext := []byte("hello reticulum buffered stream")
	compressed := bz2Stream(t, plaintext)

	link := &mockLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	reader := NewRawChannelReader(11, ch)

	msg := &StreamDataMessage{
		StreamID:   11,
		Data:       compressed,
		EOF:        true,
		Compressed: true,
	}

	if !reader.HandleMessage(msg) {
		t.Fatalf("HandleMessage returned false")
	}

	got := make([]byte, len(plaintext)+16)
	n, _ := reader.Read(got)
	if !bytes.Equal(got[:n], plaintext) {
		t.Fatalf("read mismatch: got %q, want %q", got[:n], plaintext)
	}
}

// streamFrameFlags extracts the StreamDataMessage header from a wire envelope.
func streamFrameFlags(t *testing.T, raw []byte) uint16 {
	t.Helper()
	if len(raw) < 8 {
		t.Fatalf("frame too short: %d", len(raw))
	}
	return binary.BigEndian.Uint16(raw[6:8])
}

func TestRawChannelWriterCompressionDisabled(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	defer func() { _ = ch.Close() }()
	writer := NewRawChannelWriterWithOptions(7, ch, WriterOptions{Compression: CompressionDisabled})

	// Highly compressible payload must still go out uncompressed.
	zeros := make([]byte, 4096)
	if _, err := writer.Write(zeros); err != nil {
		t.Fatalf("Write: %v", err)
	}
	frames := link.sentFrames()
	if len(frames) != 1 {
		t.Fatalf("sent %d frames, want 1", len(frames))
	}
	if streamFrameFlags(t, frames[0])&StreamHeaderCompressed != 0 {
		t.Fatal("CompressionDisabled writer sent a compressed stream message")
	}
}

func TestRawChannelWriterCompressionAutoStillCompresses(t *testing.T) {
	link := &mockLink{status: transport.StatusActive}
	ch := channel.NewChannel(link)
	defer func() { _ = ch.Close() }()
	writer := NewRawChannelWriter(7, ch)

	zeros := make([]byte, 4096)
	if _, err := writer.Write(zeros); err != nil {
		t.Fatalf("Write: %v", err)
	}
	frames := link.sentFrames()
	if len(frames) != 1 {
		t.Fatalf("sent %d frames, want 1", len(frames))
	}
	if streamFrameFlags(t, frames[0])&StreamHeaderCompressed == 0 {
		t.Fatal("auto writer did not compress a compressible payload")
	}
}

// The stream payload bound tracks the live channel MDU, so a write must fit
// the envelope the channel will accept instead of the old fixed 457.
func TestRawChannelWriterHonorsChannelMDU(t *testing.T) {
	link := &mockLink{status: transport.StatusActive, mdu: 431}
	ch := channel.NewChannel(link)
	defer func() { _ = ch.Close() }()
	writer := NewRawChannelWriter(9, ch)

	incomp := make([]byte, MaxChunkLen)
	for i := range incomp {
		incomp[i] = byte(i*31 + 7)
	}
	n, err := writer.Write(incomp)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := 431 - channel.ChannelHeaderSize - StreamHeaderSize
	if n != want {
		t.Fatalf("Write = %d bytes, want %d", n, want)
	}
	frames := link.sentFrames()
	if len(frames) != 1 {
		t.Fatalf("sent %d frames, want 1", len(frames))
	}
	if len(frames[0]) > 431 {
		t.Fatalf("frame %d bytes exceeds outlet MDU 431", len(frames[0]))
	}
}

func TestRawChannelWriterWriteContextCancel(t *testing.T) {
	link := &mockLink{status: transport.StatusActive, rtt: 0.5, noDeliver: true}
	ch := channel.NewChannel(link)
	defer func() { _ = ch.Close() }()
	writer := NewRawChannelWriter(3, ch)

	for ch.IsReadyToSend() {
		if _, err := writer.Write([]byte("x")); err != nil {
			t.Fatalf("fill write: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := writer.WriteContext(ctx, []byte("blocked")); err != context.DeadlineExceeded {
		t.Fatalf("WriteContext = %v, want context.DeadlineExceeded", err)
	}
}
