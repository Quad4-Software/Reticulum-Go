// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package buffer

import (
	"bytes"
	"testing"

	"git.quad4.io/Go-Libs/bzip2/pkg/bzip2"
	"git.quad4.io/Networks/Reticulum-Go/pkg/channel"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

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

func TestDecompressData_CapsAtMaxChunkLen(t *testing.T) {
	const huge = 8 * 1024 * 1024
	bomb := bz2Bomb(t, huge)
	if len(bomb) > 4096 {
		t.Fatalf("bomb stream unexpectedly large: %d bytes", len(bomb))
	}
	t.Logf("bomb stream: %d bytes compressed -> %d bytes decompressed (ratio %dx)",
		len(bomb), huge, huge/len(bomb))

	out := decompressData(bomb)
	if len(out) > MaxChunkLen {
		t.Fatalf("decompressData exceeded MaxChunkLen: got %d bytes, cap %d",
			len(out), MaxChunkLen)
	}
	if len(out) != MaxChunkLen {
		t.Fatalf("expected exactly MaxChunkLen=%d bytes, got %d", MaxChunkLen, len(out))
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

func TestDecompressData_RejectsCorruptStream(t *testing.T) {
	garbage := []byte("not actually a bzip2 stream")
	if got := decompressData(garbage); got != nil {
		t.Fatalf("expected nil from corrupt stream, got %d bytes", len(got))
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

	if reader.buffer.Len() > MaxChunkLen {
		t.Fatalf("reader buffer holds %d bytes after bomb, exceeds MaxChunkLen=%d",
			reader.buffer.Len(), MaxChunkLen)
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
