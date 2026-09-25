// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package buffer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"sync"

	"github.com/Quad4-Software/Reticulum-Go/pkg/channel"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/bzip2/pkg/bzip2"
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
	// genCh is closed and replaced whenever buffer or eof changes, so
	// WaitReadable waiters wake on new data and on EOF. Lazily allocated so
	// literal-constructed readers still work.
	genCh chan struct{}
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

// WaitReadable blocks until the reader has buffered data, reaches EOF, or ctx
// expires. Read on an empty, open stream returns (0, nil), which makes naive
// io.Reader consumers spin; callers that can wait should prefer ReadContext.
func (r *RawChannelReader) WaitReadable(ctx context.Context) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for r.buffer.Len() == 0 && !r.eof {
		if r.genCh == nil {
			r.genCh = make(chan struct{})
		}
		ch := r.genCh
		r.mutex.Unlock()
		select {
		case <-ctx.Done():
			r.mutex.Lock()
			return ctx.Err()
		case <-ch:
			r.mutex.Lock()
		}
	}
	return nil
}

// ReadContext waits for buffered data or EOF under ctx, then reads. Unlike
// Read it never returns (0, nil) on an open stream.
func (r *RawChannelReader) ReadContext(ctx context.Context, p []byte) (int, error) {
	if err := r.WaitReadable(ctx); err != nil {
		return 0, err
	}
	return r.Read(p)
}

// maxReaderBufferBytes bounds unread stream data held for the application.
// The sender is remote-controlled, so without a bound a stalled reader lets
// the peer grow this buffer without limit.
const maxReaderBufferBytes = 8 << 20

func (r *RawChannelReader) HandleMessage(msg channel.MessageBase) bool {
	streamMsg, ok := msg.(*StreamDataMessage)
	if !ok || streamMsg.StreamID != uint16(r.streamID) { // #nosec G115 -- stream ids are uint16 on the wire
		return false
	}

	var data []byte
	if streamMsg.Compressed {
		data = decompressData(streamMsg.Data)
	} else {
		data = streamMsg.Data
	}

	r.mutex.Lock()
	if len(data) > 0 {
		if r.buffer.Len()+len(data) <= maxReaderBufferBytes {
			r.buffer.Write(data)
		} else {
			debug.Log(debug.DebugWarning, "Raw channel reader buffer full; dropping stream data", "stream_id", r.streamID)
		}
	}

	// Honor EOF even when compressed payload fails to decompress so a
	// corrupt final chunk cannot leave the reader blocked forever.
	if streamMsg.EOF {
		r.eof = true
	}

	ready := r.buffer.Len()
	cbs := make([]func(int), 0, len(r.callbacks))
	for _, cb := range r.callbacks {
		cbs = append(cbs, cb)
	}
	if r.genCh != nil {
		close(r.genCh)
		r.genCh = make(chan struct{})
	}
	r.mutex.Unlock()

	// Invoke callbacks without the lock held so a callback that reads or
	// unregisters cannot deadlock against the channel dispatch goroutine.
	for _, cb := range cbs {
		cb(ready)
	}
	return true
}

// CompressionPolicy controls whether RawChannelWriter attempts bzip2
// compression on outgoing stream data.
type CompressionPolicy uint8

const (
	// CompressionAuto keeps the Python-compatible three-probe compression
	// algorithm. This is the default.
	CompressionAuto CompressionPolicy = iota
	// CompressionDisabled always emits uncompressed StreamDataMessages, which
	// Python RNS receivers already accept. For payloads the application knows
	// are incompressible (TLS, SSH, archives, tunnels) this avoids repeated
	// compression probes that cannot succeed.
	CompressionDisabled
)

// WriterOptions configures a RawChannelWriter.
type WriterOptions struct {
	Compression CompressionPolicy
}

type RawChannelWriter struct {
	streamID int
	channel  *channel.Channel
	eof      bool
	opts     WriterOptions
}

func NewRawChannelWriter(streamID int, ch *channel.Channel) *RawChannelWriter {
	return NewRawChannelWriterWithOptions(streamID, ch, WriterOptions{})
}

// NewRawChannelWriterWithOptions creates a writer with an explicit
// compression policy and other options.
func NewRawChannelWriterWithOptions(streamID int, ch *channel.Channel, opts WriterOptions) *RawChannelWriter {
	_ = ch.RegisterSystemMessageType(StreamDataMessageType, func() channel.MessageBase {
		return &StreamDataMessage{}
	})
	return &RawChannelWriter{
		streamID: streamID,
		channel:  ch,
		opts:     opts,
	}
}

// streamMDU returns the payload limit for one stream message, derived from
// the live channel MDU like Python RawChannelWriter._mdu
// (channel.mdu - StreamDataMessage.HEADER_LEN).
func (w *RawChannelWriter) streamMDU() int {
	mdu := w.channel.MDU() - StreamHeaderSize
	if mdu < 1 {
		mdu = 1
	}
	return mdu
}

func (w *RawChannelWriter) Write(p []byte) (n int, err error) {
	return w.WriteContext(context.Background(), p)
}

// WriteContext is Write with caller-controlled cancellation of the
// backpressure wait when the channel TX window is full.
func (w *RawChannelWriter) WriteContext(ctx context.Context, p []byte) (n int, err error) {
	if len(p) > MaxChunkLen {
		p = p[:MaxChunkLen]
	}
	mdu := w.streamMDU()

	msg := &StreamDataMessage{
		StreamID: uint16(w.streamID), // #nosec G115
		EOF:      w.eof,
	}
	processed := 0

	if w.opts.Compression != CompressionDisabled && len(p) > CompressThreshold {
		for try := 1; try < CompressTries; try++ {
			chunkLen := len(p) / try
			compressed := compressData(p[:chunkLen])
			if compressed != nil && len(compressed) < mdu && len(compressed) < chunkLen {
				msg.Data = compressed
				msg.Compressed = true
				processed = chunkLen
				break
			}
		}
	}
	if !msg.Compressed {
		if len(p) > mdu {
			p = p[:mdu]
		}
		msg.Data = p
		processed = len(p)
	}

	if err := w.channel.WaitReady(ctx); err != nil {
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
	return CreateWriterWithOptions(streamID, ch, WriterOptions{})
}

// CreateWriterWithOptions is CreateWriter with an explicit compression policy.
func CreateWriterWithOptions(streamID int, ch *channel.Channel, opts WriterOptions) *bufio.Writer {
	raw := NewRawChannelWriterWithOptions(streamID, ch, opts)
	return bufio.NewWriter(raw)
}

func CreateBidirectionalBuffer(receiveStreamID, sendStreamID int, ch *channel.Channel, readyCallback func(int)) *bufio.ReadWriter {
	return CreateBidirectionalBufferWithOptions(receiveStreamID, sendStreamID, ch, readyCallback, WriterOptions{})
}

// CreateBidirectionalBufferWithOptions is CreateBidirectionalBuffer with an
// explicit compression policy on the send stream.
func CreateBidirectionalBufferWithOptions(receiveStreamID, sendStreamID int, ch *channel.Channel, readyCallback func(int), opts WriterOptions) *bufio.ReadWriter {
	reader := CreateReader(receiveStreamID, ch, readyCallback)
	writer := CreateWriterWithOptions(sendStreamID, ch, opts)
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
