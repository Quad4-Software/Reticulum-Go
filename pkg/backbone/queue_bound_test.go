package backbone

import (
	"bytes"
	"net"
	"sync"
	"testing"
	"time"
)

// stallConn never completes a write beyond accepting nothing: every Write
// returns 0 with a nil error, simulating a peer that stops reading while the
// kernel buffer never drains.
type stallConn struct{ closed bool }

func (c *stallConn) Read([]byte) (int, error)         { time.Sleep(time.Hour); return 0, nil }
func (c *stallConn) Write([]byte) (int, error)        { return 0, nil }
func (c *stallConn) Close() error                     { c.closed = true; return nil }
func (c *stallConn) LocalAddr() net.Addr              { return nil }
func (c *stallConn) RemoteAddr() net.Addr             { return nil }
func (c *stallConn) SetDeadline(time.Time) error      { return nil }
func (c *stallConn) SetReadDeadline(time.Time) error  { return nil }
func (c *stallConn) SetWriteDeadline(time.Time) error { return nil }

// TestStreamQueueBoundClosesSlowPeer: a peer that never reads must not pin
// unbounded memory; the stream closes once the queue crosses the cap.
func TestStreamQueueBoundClosesSlowPeer(t *testing.T) {
	h := &Hub{streams: make(map[int]*Stream), stop: make(chan struct{})}
	s := &Stream{hub: h, conn: &stallConn{}, fd: 7, decoder: NewHDLCDecoder(4096, nil)}
	h.streams[7] = s

	// 9 x 1 MiB frames exceed the 8 MiB queue cap.
	frame := bytes.Repeat([]byte{0x41}, 1<<20)
	for i := 0; i < 9 && !s.closed.Load(); i++ {
		s.QueueSend(frame)
	}
	if !s.closed.Load() {
		t.Fatal("stream stayed open with an unbounded outbound queue")
	}
	if _, ok := h.streams[7]; ok {
		t.Fatal("closed stream was not deregistered from the hub")
	}
}

// throttledConn accepts at most n bytes total across all writes.
type throttledConn struct {
	mu     sync.Mutex
	left   int
	writes [][]byte
}

func (c *throttledConn) Read([]byte) (int, error) { time.Sleep(time.Hour); return 0, nil }
func (c *throttledConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(b)
	if n > c.left {
		n = c.left
	}
	c.left -= n
	c.writes = append(c.writes, append([]byte(nil), b[:n]...))
	return n, nil
}
func (c *throttledConn) Close() error                     { return nil }
func (c *throttledConn) LocalAddr() net.Addr              { return nil }
func (c *throttledConn) RemoteAddr() net.Addr             { return nil }
func (c *throttledConn) SetDeadline(time.Time) error      { return nil }
func (c *throttledConn) SetReadDeadline(time.Time) error  { return nil }
func (c *throttledConn) SetWriteDeadline(time.Time) error { return nil }

// TestGoModeQueuePreservesOrder: on the go backend a partial write leaves
// queued bytes in txBuf; the next QueueSend must flush the leftovers before
// the new frame, never overwrite them.
func TestGoModeQueuePreservesOrder(t *testing.T) {
	h := &Hub{goMode: true, streams: make(map[int]*Stream), stop: make(chan struct{})}
	conn := &throttledConn{left: 4} // accept only 4 bytes of the first frame
	s := &Stream{hub: h, conn: conn, fd: 9, decoder: NewHDLCDecoder(4096, nil)}

	first := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02}
	s.QueueSend(first)
	if len(s.txBuf) == 0 {
		t.Fatal("partial write left nothing queued")
	}
	conn.mu.Lock()
	conn.left = 1 << 20 // unthrottle
	conn.mu.Unlock()

	second := []byte{0xCA, 0xFE}
	s.QueueSend(second)
	if len(s.txBuf) != 0 {
		t.Fatalf("queue did not drain after unthrottled write, %d bytes left", len(s.txBuf))
	}
	// What the peer received must be frame(first) || frame(second).
	want := appendFrameHDLC(nil, first)
	want = appendFrameHDLC(want, second)
	var got []byte
	conn.mu.Lock()
	for _, w := range conn.writes {
		got = append(got, w...)
	}
	conn.mu.Unlock()
	if !bytes.Equal(got, want) {
		t.Fatalf("go-mode queue corrupted ordering:\n got %x\nwant %x", got, want)
	}
}
