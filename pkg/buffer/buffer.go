// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package buffer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"sync"

	"quad4/bzip2/pkg/bzip2"
	"quad4/reticulum-go/pkg/channel"
)

type StreamDataMessage struct {
	StreamID   uint16
	Data       []byte
	EOF        bool
	Compressed bool
}

func (m *StreamDataMessage) Pack() ([]byte, error) {
	headerVal := uint16(m.StreamID & StreamIDMax)
	if m.EOF {
		headerVal |= StreamHeaderEOF
	}
	if m.Compressed {
		headerVal |= StreamHeaderCompressed
	}

	n := StreamHeaderSize + len(m.Data)
	buf := make([]byte, n)
	binary.BigEndian.PutUint16(buf, headerVal)
	copy(buf[StreamHeaderSize:], m.Data)
	return buf, nil
}

func (m *StreamDataMessage) GetType() uint16 {
	return StreamDataMessageType
}

func (m *StreamDataMessage) Unpack(data []byte) error {
	if len(data) < StreamHeaderSize {
		return io.ErrShortBuffer
	}

	header := binary.BigEndian.Uint16(data[:StreamHeaderSize])
	m.StreamID = header & StreamIDMax
	m.EOF = (header & StreamHeaderEOF) != 0
	m.Compressed = (header & StreamHeaderCompressed) != 0
	m.Data = data[StreamHeaderSize:]

	return nil
}

type RawChannelReader struct {
	streamID         int
	channel          *channel.Channel
	buffer           *bytes.Buffer
	eof              bool
	callbacks        map[int]func(int)
	nextCallbackID   int
	messageHandlerID int
	mutex            sync.RWMutex
}

func NewRawChannelReader(streamID int, ch *channel.Channel) *RawChannelReader {
	reader := &RawChannelReader{
		streamID:  streamID,
		channel:   ch,
		buffer:    bytes.NewBuffer(nil),
		callbacks: make(map[int]func(int)),
	}

	_ = ch.RegisterSystemMessageType(StreamDataMessageType, func() channel.MessageBase {
		return &StreamDataMessage{}
	})
	reader.messageHandlerID = ch.AddMessageHandler(reader.HandleMessage)
	return reader
}

func (r *RawChannelReader) AddReadyCallback(cb func(int)) int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	id := r.nextCallbackID
	r.nextCallbackID++
	r.callbacks[id] = cb
	return id
}

func (r *RawChannelReader) RemoveReadyCallback(id int) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.callbacks, id)
}

func (r *RawChannelReader) Read(p []byte) (n int, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.buffer.Len() == 0 && r.eof {
		return 0, io.EOF
	}

	n, err = r.buffer.Read(p)
	if err == io.EOF && !r.eof {
		err = nil
	}
	return n, err
}

func (r *RawChannelReader) HandleMessage(msg channel.MessageBase) bool { // #nosec G115
	if streamMsg, ok := msg.(*StreamDataMessage); ok && streamMsg.StreamID == uint16(r.streamID) {
		r.mutex.Lock()
		defer r.mutex.Unlock()

		if streamMsg.Compressed {
			decompressed := decompressData(streamMsg.Data)
			if decompressed != nil {
				r.buffer.Write(decompressed)
			}
		} else {
			r.buffer.Write(streamMsg.Data)
		}

		// Honor EOF even when compressed payload fails to decompress so a
		// corrupt final chunk cannot leave the reader blocked forever.
		if streamMsg.EOF {
			r.eof = true
		}

		for _, cb := range r.callbacks {
			cb(r.buffer.Len())
		}

		return true
	}
	return false
}

type RawChannelWriter struct {
	streamID int
	channel  *channel.Channel
	eof      bool
}

func NewRawChannelWriter(streamID int, ch *channel.Channel) *RawChannelWriter {
	_ = ch.RegisterSystemMessageType(StreamDataMessageType, func() channel.MessageBase {
		return &StreamDataMessage{}
	})
	return &RawChannelWriter{
		streamID: streamID,
		channel:  ch,
	}
}

func (w *RawChannelWriter) Write(p []byte) (n int, err error) {
	if len(p) > MaxChunkLen {
		p = p[:MaxChunkLen]
	}

	msg := &StreamDataMessage{
		StreamID: uint16(w.streamID), // #nosec G115
		EOF:      w.eof,
	}
	processed := 0

	if len(p) > CompressThreshold {
		for try := 1; try < CompressTries; try++ {
			chunkLen := len(p) / try
			compressed := compressData(p[:chunkLen])
			if compressed != nil && len(compressed) < MaxDataLen && len(compressed) < chunkLen {
				msg.Data = compressed
				msg.Compressed = true
				processed = chunkLen
				break
			}
		}
	}
	if !msg.Compressed {
		if len(p) > MaxDataLen {
			p = p[:MaxDataLen]
		}
		msg.Data = p
		processed = len(p)
	}

	if err := w.channel.WaitReady(context.Background()); err != nil {
		return 0, err
	}
	if err := w.channel.Send(msg); err != nil {
		return 0, err
	}

	return processed, nil
}

func (w *RawChannelWriter) Close() error {
	w.eof = true
	_, err := w.Write(nil)
	return err
}

type Buffer struct {
	ReadWriter *bufio.ReadWriter
}

func (b *Buffer) Write(p []byte) (n int, err error) {
	return b.ReadWriter.Write(p)
}

func (b *Buffer) Read(p []byte) (n int, err error) {
	return b.ReadWriter.Read(p)
}

func (b *Buffer) Close() error {
	return b.ReadWriter.Writer.Flush()
}

func CreateReader(streamID int, ch *channel.Channel, readyCallback func(int)) *bufio.Reader {
	raw := NewRawChannelReader(streamID, ch)
	if readyCallback != nil {
		raw.AddReadyCallback(readyCallback)
	}
	return bufio.NewReader(raw)
}

func CreateWriter(streamID int, ch *channel.Channel) *bufio.Writer {
	raw := NewRawChannelWriter(streamID, ch)
	return bufio.NewWriter(raw)
}

func CreateBidirectionalBuffer(receiveStreamID, sendStreamID int, ch *channel.Channel, readyCallback func(int)) *bufio.ReadWriter {
	reader := CreateReader(receiveStreamID, ch, readyCallback)
	writer := CreateWriter(sendStreamID, ch)
	return bufio.NewReadWriter(reader, writer)
}

func compressData(data []byte) []byte {
	var compressed bytes.Buffer
	w, err := bzip2.NewWriter(&compressed, 9)
	if err != nil {
		return nil
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return nil
	}
	if err := w.Close(); err != nil {
		return nil
	}
	return compressed.Bytes()
}

func decompressData(data []byte) []byte {
	reader := bzip2.NewReader(bytes.NewReader(data))
	// Cap at MaxChunkLen and reject streams that would expand further
	limited := io.LimitReader(reader, int64(MaxChunkLen)+1) // #nosec G110
	decompressed, err := io.ReadAll(limited)
	if err != nil {
		return nil
	}
	if len(decompressed) > MaxChunkLen {
		return nil
	}
	return decompressed
}
