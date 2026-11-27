// SPDX-License-Identifier: LicenseRef-Reticulum
// Package faultproxy implements a seeded TCP fault-injecting proxy for
// chaos testing. It understands RNS HDLC framing so frame-level faults
// (drop, duplication, reordering) are frame-aligned; a byte-level mode
// corrupts the raw stream instead.
package faultproxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"
	"time"
)

// Faults configures injection probabilities per direction. Probabilities
// are 0..1 per frame (frame mode) or per byte chunk (byte mode).
type Faults struct {
	Drop        float64 // drop whole frames
	Duplicate   float64 // deliver a frame an extra time
	Reorder     float64 // hold a frame back behind the next one
	DelayMs     int     // fixed extra latency per frame
	JitterMs    int     // uniform random extra latency 0..JitterMs
	Corrupt     float64 // byte mode: flip bytes mid-stream
	ByteMode    bool    // operate on raw bytes, not frames
	ThrottleBps int     // max bytes per second throughput, 0 = unlimited
	ResetAfter  int     // reset connection after N frames, 0 = never
}

// Stats reports what the proxy did, for assertions.
type Stats struct {
	FramesIn   int
	FramesOut  int
	Dropped    int
	Duplicated int
	Reordered  int
	BytesIn    int64
	BytesOut   int64
	Corrupted  int
	Reset      bool
}

// Proxy is a TCP listener forwarding to a target with fault injection.
type Proxy struct {
	Addr   string
	Seed   int64
	Faults Faults // applied on client->server traffic; reverse is symmetric copy unless overridden

	mu     sync.Mutex
	ln     net.Listener
	conns  map[net.Conn]struct{}
	closed bool
	stats  Stats
	target string
}

const (
	hdlcFlag   = 0x7e
	hdlcEscape = 0x7d
	hdlcEscXor = 0x20
)

// New starts a proxy listening on 127.0.0.1:0 forwarding to targetAddr.
func New(targetAddr string, seed int64, faults Faults) (*Proxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &Proxy{
		Addr:   ln.Addr().String(),
		Seed:   seed,
		Faults: faults,
		ln:     ln,
		conns:  make(map[net.Conn]struct{}),
		target: targetAddr,
	}
	go p.acceptLoop()
	return p, nil
}

// Snapshot returns a copy of the stats.
func (p *Proxy) Snapshot() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

func (p *Proxy) addStats(fn func(*Stats)) {
	p.mu.Lock()
	fn(&p.stats)
	p.mu.Unlock()
}

// Close stops the proxy and all active connections.
func (p *Proxy) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	ln := p.ln
	conns := make([]net.Conn, 0, len(p.conns))
	for c := range p.conns {
		conns = append(conns, c)
	}
	p.mu.Unlock()
	_ = ln.Close()
	for _, c := range conns {
		_ = c.Close()
	}
	return nil
}

// Flap resets every active connection (simulated link flap).
func (p *Proxy) Flap() {
	p.mu.Lock()
	conns := make([]net.Conn, 0, len(p.conns))
	for c := range p.conns {
		conns = append(conns, c)
	}
	p.mu.Unlock()
	for _, c := range conns {
		if tc, ok := c.(*net.TCPConn); ok {
			_ = tc.SetLinger(0)
		}
		_ = c.Close()
	}
}

func (p *Proxy) acceptLoop() {
	for {
		client, err := p.ln.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			_ = client.Close()
			return
		}
		p.conns[client] = struct{}{}
		p.mu.Unlock()
		go p.handle(client)
	}
}

func (p *Proxy) track(c net.Conn) func() {
	return func() {
		p.mu.Lock()
		delete(p.conns, c)
		p.mu.Unlock()
		_ = c.Close()
	}
}

func (p *Proxy) handle(client net.Conn) {
	defer p.track(client)()
	server, err := net.DialTimeout("tcp", p.target, 5*time.Second)
	if err != nil {
		return
	}
	defer func() { _ = server.Close() }()

	// Per-connection RNG derived from the seed for determinism.
	p.mu.Lock()
	connIdx := int64(len(p.conns))
	p.mu.Unlock()

	done := make(chan struct{}, 2)
	go func() {
		p.pipe(client, server, rand.New(rand.NewSource(p.Seed+connIdx*2)), p.Faults)
		done <- struct{}{}
	}()
	go func() {
		// reverse direction runs without faults on duplication/reorder of
		// control sequencing but keeps delay/drop for realism
		p.pipe(server, client, rand.New(rand.NewSource(p.Seed+connIdx*2+1)), p.Faults)
		done <- struct{}{}
	}()
	<-done
	// when either direction dies, tear down the other
	if tc, ok := client.(*net.TCPConn); ok {
		_ = tc.SetLinger(0)
	}
	_ = client.Close()
	_ = server.Close()
}

// pipe copies src->dst applying faults. Frame mode parses HDLC frames.
func (p *Proxy) pipe(dst net.Conn, src net.Conn, rng *rand.Rand, f Faults) {
	if f.ByteMode {
		p.pipeBytes(dst, src, rng, f)
		return
	}
	p.pipeFrames(dst, src, rng, f)
}

func (p *Proxy) throttle(n int, f Faults, budget *time.Duration) {
	if f.ThrottleBps <= 0 {
		return
	}
	d := time.Duration(n) * time.Second / time.Duration(f.ThrottleBps)
	time.Sleep(d)
}

func (p *Proxy) pipeBytes(dst net.Conn, src net.Conn, rng *rand.Rand, f Faults) {
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			if f.Corrupt > 0 && rng.Float64() < f.Corrupt && len(chunk) > 0 {
				i := rng.Intn(len(chunk))
				chunk[i] ^= byte(1 << rng.Intn(8))
				p.addStats(func(s *Stats) { s.Corrupted++ })
			}
			p.addStats(func(s *Stats) { s.BytesIn += int64(n) })
			if f.DelayMs > 0 || f.JitterMs > 0 {
				d := time.Duration(f.DelayMs) * time.Millisecond
				if f.JitterMs > 0 {
					d += time.Duration(rng.Intn(f.JitterMs)) * time.Millisecond
				}
				time.Sleep(d)
			}
			p.throttle(len(chunk), f, nil)
			if _, werr := dst.Write(chunk); werr != nil {
				return
			}
			p.addStats(func(s *Stats) { s.BytesOut += int64(len(chunk)) })
		}
		if err != nil {
			// forward half-close so the peer sees EOF
			if tc, ok := dst.(*net.TCPConn); ok {
				_ = tc.CloseWrite()
			}
			return
		}
	}
}

// pipeFrames reads HDLC-framed traffic. Data arrives as a raw stream; frames
// are delimited by 0x7e flags. The proxy re-emits the raw bytes unchanged
// unless faults apply.
func (p *Proxy) pipeFrames(dst net.Conn, src net.Conn, rng *rand.Rand, f Faults) {
	var stream []byte
	buf := make([]byte, 4096)
	var held [][]byte // reorder buffer: held frames emitted after the next one
	frameCount := 0
	flushFrame := func(frame []byte) error {
		p.throttle(len(frame), f, nil)
		if _, err := dst.Write(frame); err != nil {
			return err
		}
		p.addStats(func(s *Stats) {
			s.FramesOut++
			s.BytesOut += int64(len(frame))
		})
		return nil
	}
	process := func(frame []byte) error {
		frameCount++
		p.addStats(func(s *Stats) {
			s.FramesIn++
			s.BytesIn += int64(len(frame))
		})
		if f.ResetAfter > 0 && frameCount >= f.ResetAfter {
			p.addStats(func(s *Stats) { s.Reset = true })
			if tc, ok := dst.(*net.TCPConn); ok {
				_ = tc.SetLinger(0)
			}
			_ = dst.Close()
			_ = src.Close()
			return errReset
		}
		if f.Drop > 0 && rng.Float64() < f.Drop {
			p.addStats(func(s *Stats) { s.Dropped++ })
			return nil
		}
		if f.DelayMs > 0 || f.JitterMs > 0 {
			d := time.Duration(f.DelayMs) * time.Millisecond
			if f.JitterMs > 0 {
				d += time.Duration(rng.Intn(f.JitterMs)) * time.Millisecond
			}
			time.Sleep(d)
		}
		out := frame
		if f.Reorder > 0 && rng.Float64() < f.Reorder {
			held = append(held, out)
			p.addStats(func(s *Stats) { s.Reordered++ })
		} else {
			if err := flushFrame(out); err != nil {
				return err
			}
			// emit any held frames after this one (bounded reorder)
			for _, h := range held {
				if err := flushFrame(h); err != nil {
					return err
				}
			}
			held = held[:0]
		}
		if f.Duplicate > 0 && rng.Float64() < f.Duplicate {
			p.addStats(func(s *Stats) { s.Duplicated++ })
			if err := flushFrame(out); err != nil {
				return err
			}
		}
		return nil
	}

	for {
		n, err := src.Read(buf)
		if n > 0 {
			stream = append(stream, buf[:n]...)
			// A piece is payload bytes up to and including the next flag:
			// in HDLC the flag terminates a frame, so a piece completes as
			// soon as its flag arrives. Concatenating all pieces reproduces
			// the input exactly, keeping the proxy byte-transparent when no
			// faults apply. One-byte pieces are padding flags passed through
			// unfaulted; stream-start partial pieces pass through verbatim.
			for len(stream) > 0 {
				idx := bytes.IndexByte(stream, hdlcFlag)
				if idx < 0 {
					break
				}
				piece := append([]byte(nil), stream[:idx+1]...)
				stream = append([]byte(nil), stream[idx+1:]...)
				if len(piece) == 1 {
					if err := flushFrame(piece); err != nil {
						return
					}
					continue
				}
				if perr := process(piece); perr != nil {
					return
				}
			}
		}
		if err != nil {
			// flush remaining partial stream verbatim
			if len(stream) > 0 {
				_, _ = dst.Write(stream)
			}
			for _, h := range held {
				_, _ = dst.Write(h)
			}
			if tc, ok := dst.(*net.TCPConn); ok {
				_ = tc.CloseWrite()
			}
			return
		}
	}
}

var errReset = errors.New("faultproxy: injected reset")

func (f Faults) String() string {
	return fmt.Sprintf("drop=%.2f dup=%.2f reorder=%.2f delay=%dms jitter=%dms corrupt=%.2f byte=%v throttle=%d reset@%d",
		f.Drop, f.Duplicate, f.Reorder, f.DelayMs, f.JitterMs, f.Corrupt, f.ByteMode, f.ThrottleBps, f.ResetAfter)
}

var _ = io.EOF
