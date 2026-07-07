// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package backbone

import (
	"bytes"
	"fmt"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestInitIdempotent(t *testing.T) {
	Shutdown()
	h1, err := Init(BackendGo)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := Init(BackendEpoll)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatal("expected same global hub instance")
	}
	t.Cleanup(Shutdown)
}

func TestDefaultBackendPlatform(t *testing.T) {
	b := DefaultBackend()
	switch runtime.GOOS {
	case "linux", "android":
		if b != BackendEpoll {
			t.Fatalf("linux default=%s", b)
		}
	case "darwin", "freebsd", "netbsd", "openbsd":
		if b != BackendKqueue {
			t.Fatalf("bsd default=%s", b)
		}
	default:
		if b != BackendGo {
			t.Fatalf("generic default=%s", b)
		}
	}
}

func TestHubEchoAllBackends(t *testing.T) {
	for _, backend := range testableBackends(t) {
		t.Run(string(backend), func(t *testing.T) {
			testHubEcho(t, backend)
		})
	}
}

func testHubEcho(t *testing.T, backend Backend) {
	t.Helper()
	hub := testHubWithBackend(t, backend)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var (
		serverStream *Stream
		serverReady  = make(chan struct{})
		serverOnce   sync.Once
	)
	if err := hub.RegisterListener(ln, func(conn net.Conn) {
		var err error
		serverStream, err = hub.RegisterStream(conn, 1<<20, func(frame []byte) {
			serverStream.QueueSend(frame)
		}, nil)
		if err != nil {
			t.Errorf("server stream: %v", err)
			return
		}
		serverOnce.Do(func() { close(serverReady) })
	}); err != nil {
		t.Fatal(err)
	}

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientConn.Close() })

	payload := bytes.Repeat([]byte{0xAB}, 512)
	got := make(chan []byte, 1)
	clientStream, err := hub.RegisterStream(clientConn, 1<<20, func(frame []byte) {
		select {
		case got <- append([]byte(nil), frame...):
		default:
		}
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-serverReady:
	case <-time.After(3 * time.Second):
		t.Fatal("accept timeout")
	}

	clientStream.QueueSend(payload)
	select {
	case rcv := <-got:
		if !bytes.Equal(rcv, payload) {
			t.Fatalf("payload mismatch len=%d", len(rcv))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("echo timeout")
	}
}

func TestHubManyConcurrentClients(t *testing.T) {
	for _, backend := range testableBackends(t) {
		t.Run(string(backend), func(t *testing.T) {
			testHubManyClients(t, backend, 32)
		})
	}
}

func testHubManyClients(t *testing.T, backend Backend, n int) {
	t.Helper()
	hub := testHubWithBackend(t, backend)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var served atomic.Int32
	if err := hub.RegisterListener(ln, func(conn net.Conn) {
		served.Add(1)
		_, _ = hub.RegisterStream(conn, 4096, nil, func() { _ = conn.Close() })
	}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(id int) {
			defer wg.Done()
			c, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Errorf("dial %d: %v", id, err)
				return
			}
			defer c.Close()
			time.Sleep(10 * time.Millisecond)
		}(i)
	}
	wg.Wait()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if int(served.Load()) >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("served %d want %d", served.Load(), n)
}

func TestHubLargePayload(t *testing.T) {
	hub := testHubWithBackend(t, BackendGo)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	ready := make(chan *Stream, 1)
	if err := hub.RegisterListener(ln, func(conn net.Conn) {
		var st *Stream
		var regErr error
		st, regErr = hub.RegisterStream(conn, 1<<20, func(frame []byte) {
			st.QueueSend(frame)
		}, nil)
		if regErr != nil {
			t.Errorf("stream: %v", regErr)
			return
		}
		ready <- st
	}); err != nil {
		t.Fatal(err)
	}

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })

	payload := bytes.Repeat([]byte{0xCD}, 64*1024)
	done := make(chan []byte, 1)
	cs, err := hub.RegisterStream(c, 1<<20, func(frame []byte) {
		done <- append([]byte(nil), frame...)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("server timeout")
	}

	cs.QueueSend(payload)
	select {
	case got := <-done:
		if !bytes.Equal(got, payload) {
			t.Fatalf("large payload mismatch: got %d want %d", len(got), len(payload))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("large echo timeout")
	}
}

func TestHubCloseDrains(t *testing.T) {
	hub := testHubWithBackend(t, BackendGo)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := hub.RegisterListener(ln, func(net.Conn) {}); err != nil {
		t.Fatal(err)
	}
	hub.Close()
}

func TestStreamDoubleClose(t *testing.T) {
	hub := testHubWithBackend(t, BackendGo)
	c1, c2 := net.Pipe()
	t.Cleanup(func() { _ = c1.Close(); _ = c2.Close() })

	s, err := hub.RegisterStream(c1, 1024, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s.Close()
}

func TestParseBackendAliases(t *testing.T) {
	if ParseBackend("kqueue") != BackendKqueue {
		t.Fatal("kqueue")
	}
	if ParseBackend("go") != BackendGo {
		t.Fatal("go")
	}
	if ParseBackend("unknown-value") != BackendAuto {
		t.Fatal("unknown")
	}
}

func TestInitUringFallback(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	Shutdown()
	hub, err := Init(BackendUring)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(Shutdown)
	if hub.Backend() != BackendUring && hub.Backend() != BackendEpoll {
		t.Fatalf("unexpected backend %s", hub.Backend())
	}
}

// Race: concurrent QueueSend on one stream over TCP.
func TestRaceStreamConcurrentSend(t *testing.T) {
	hub := testHubWithBackend(t, BackendGo)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	ready := make(chan *Stream, 1)
	if err := hub.RegisterListener(ln, func(conn net.Conn) {
		var st *Stream
		var regErr error
		st, regErr = hub.RegisterStream(conn, 1<<20, func([]byte) {}, nil)
		if regErr != nil {
			t.Errorf("stream: %v", regErr)
			return
		}
		ready <- st
	}); err != nil {
		t.Fatal(err)
	}

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })

	var st *Stream
	select {
	case st = <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("accept timeout")
	}

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := range 32 {
				st.QueueSend([]byte{byte(n), byte(j), 0x01})
			}
		}(i)
	}
	wg.Wait()
}

// Race: hub shutdown while clients active.
func TestRaceHubCloseWithActiveStreams(t *testing.T) {
	for range 4 {
		hub := testHubWithBackend(t, BackendGo)
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		_ = hub.RegisterListener(ln, func(conn net.Conn) {
			_, _ = hub.RegisterStream(conn, 4096, func(frame []byte) {
				// echo
			}, nil)
		})
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				c, err := net.Dial("tcp", ln.Addr().String())
				if err != nil {
					return
				}
				defer c.Close()
				s, err := hub.RegisterStream(c, 4096, nil, nil)
				if err != nil {
					return
				}
				for range 16 {
					s.QueueSend([]byte{1, 2, 3})
				}
			})
		}
		wg.Wait()
		_ = ln.Close()
		hub.Close()
		Shutdown()
	}
}

// Race: accept storm.
func TestRaceAcceptStorm(t *testing.T) {
	hub := testHubWithBackend(t, DefaultBackend())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var accepted atomic.Int32
	if err := hub.RegisterListener(ln, func(conn net.Conn) {
		accepted.Add(1)
		_, _ = hub.RegisterStream(conn, 2048, nil, nil)
	}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() {
			c, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				return
			}
			defer c.Close()
		})
	}
	wg.Wait()
	time.Sleep(200 * time.Millisecond)
	if accepted.Load() == 0 {
		t.Fatal("no connections accepted")
	}
}

func TestHubWireFormatMatchesInterfacesPackage(t *testing.T) {
	payload := []byte{0x01, hdlcFlag, hdlcEsc, 0xFF}
	backboneFrame := frameHDLC(payload)
	if backboneFrame[0] != hdlcFlag || backboneFrame[len(backboneFrame)-1] != hdlcFlag {
		t.Fatal("missing frame flags")
	}
	var got []byte
	d := NewHDLCDecoder(4096, func(pkt []byte) { got = pkt })
	d.Feed(backboneFrame)
	if !bytes.Equal(got, payload) {
		t.Fatalf("decode got %x want %x", got, payload)
	}
}

func BenchmarkHubEcho(b *testing.B) {
	for _, backend := range []Backend{BackendGo, DefaultBackend()} {
		b.Run(string(backend), func(b *testing.B) {
			runHubEchoBenchmark(b, backend)
		})
	}
}

func runHubEchoBenchmark(b *testing.B, backend Backend) {
	Shutdown()
	hub, err := Init(backend)
	if err != nil {
		b.Fatal(err)
	}
	defer Shutdown()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	defer ln.Close()

	ready := make(chan struct{})
	if err := hub.RegisterListener(ln, func(conn net.Conn) {
		var st *Stream
		var regErr error
		st, regErr = hub.RegisterStream(conn, 1<<20, func(frame []byte) {
			st.QueueSend(frame)
		}, nil)
		if regErr != nil {
			b.Error(regErr)
			return
		}
		close(ready)
	}); err != nil {
		b.Fatal(err)
	}

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()

	payload := bytes.Repeat([]byte{0x42}, 1024)
	ack := make(chan struct{}, 1)
	cs, err := hub.RegisterStream(c, 1<<20, func([]byte) {
		select {
		case ack <- struct{}{}:
		default:
		}
	}, nil)
	if err != nil {
		b.Fatal(err)
	}
	<-ready

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cs.QueueSend(payload)
		<-ack
	}
}

func BenchmarkHDLCEscape(b *testing.B) {
	data := bytes.Repeat([]byte{0x7E}, 1024)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = escapeHDLC(data)
	}
}

func BenchmarkHDLCDecoderFeed(b *testing.B) {
	payload := bytes.Repeat([]byte{0x42}, 512)
	frame := frameHDLC(payload)
	d := NewHDLCDecoder(4096, func([]byte) {})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		d.Reset()
		d.Feed(frame)
	}
}

func BenchmarkFrameHDLC(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = frameHDLC(data)
	}
}

func ExampleHub() {
	hub, err := Init(BackendGo)
	if err != nil {
		fmt.Println("init failed")
		return
	}
	defer Shutdown()
	fmt.Println(hub.Backend() == BackendGo)
	// Output: true
}
