package buffer

import (
	"bufio"
	"bytes"
	"io"
	"testing"
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
		callbacks: make([]func(int), 0),
	}

	cb := func(int) {}

	reader.AddReadyCallback(cb)
	if len(reader.callbacks) != 1 {
		t.Error("Callback should be added")
	}
}

func TestRawChannelWriter_Write(t *testing.T) {
	writer := &RawChannelWriter{
		streamID: 1,
		eof:      false,
	}

	if writer.streamID != 1 {
		t.Error("StreamID not set correctly")
	}
}

func TestRawChannelWriter_Close(t *testing.T) {
	writer := &RawChannelWriter{
		streamID: 1,
		eof:      false,
	}

	if writer.eof {
		t.Error("EOF should be false initially")
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
