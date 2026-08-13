// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/backbone"
)

const (
	burstFlag    = 0x7E
	burstEsc     = 0x7D
	burstEscMask = 0x20
)

func burstPayload(i int) []byte {
	body := bytes.Repeat([]byte{byte(i)}, 24)
	return append([]byte{burstFlag, burstEsc}, body...)
}

func burstFrame(payload []byte) []byte {
	out := []byte{burstFlag}
	for _, b := range payload {
		if b == burstFlag || b == burstEsc {
			out = append(out, burstEsc, b^burstEscMask)
		} else {
			out = append(out, b)
		}
	}
	return append(out, burstFlag)
}

func TestLiveInteropHDLCBurstPythonToGo(t *testing.T) {
	liveOrSkip(t)
	const n = 64
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	port := ln.Addr().(*net.TCPAddr).Port

	got := make(chan []byte, n)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		d := backbone.NewHDLCDecoder(4096, func(pkt []byte) {
			got <- append([]byte(nil), pkt...)
		})
		buf := make([]byte, 64*1024)
		for {
			nr, err := conn.Read(buf)
			if nr > 0 {
				d.Feed(buf[:nr])
			}
			if err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "hdlc_burst.py"))
	cmd.Env = append(os.Environ(),
		"INTEROP_MODE=client",
		"INTEROP_TARGET_PORT="+strconv.Itoa(port),
		"INTEROP_FRAMES="+strconv.Itoa(n),
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("python client: %v", err)
	}

	deadline := time.After(5 * time.Second)
	var frames [][]byte
	for len(frames) < n {
		select {
		case f := <-got:
			frames = append(frames, f)
		case <-deadline:
			t.Fatalf("got %d frames want %d", len(frames), n)
		}
	}
	for i := range n {
		if !bytes.Equal(frames[i], burstPayload(i)) {
			t.Fatalf("frame %d mismatch", i)
		}
	}
}

func TestLiveInteropHDLCBurstGoToPython(t *testing.T) {
	liveOrSkip(t)
	const n = 64
	port := freeTCPPort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "hdlc_burst.py"))
	cmd.Env = append(os.Environ(),
		"INTEROP_MODE=server",
		"INTEROP_PORT="+strconv.Itoa(port),
		"INTEROP_FRAMES="+strconv.Itoa(n),
	)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	br := bufio.NewReader(out)
	line, err := readLineTimeout(ctx, br, 10*time.Second)
	if err != nil {
		t.Fatalf("READY: %v", err)
	}
	if strings.TrimSpace(line) != "READY" {
		t.Fatalf("expected READY, got %q", line)
	}

	c, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var blob []byte
	for i := range n {
		blob = append(blob, burstFrame(burstPayload(i))...)
	}
	if _, err := c.Write(blob); err != nil {
		t.Fatal(err)
	}
	countLine, err := readLineTimeout(ctx, br, 10*time.Second)
	if err != nil {
		t.Fatalf("COUNT: %v", err)
	}
	if strings.TrimSpace(countLine) != "COUNT "+strconv.Itoa(n) {
		t.Fatalf("got %q", countLine)
	}
}

func listenHDLCFrames(t *testing.T, accepts int, n int) (port int, got <-chan []byte) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	ch := make(chan []byte, n+8)
	go func() {
		for range accepts {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			d := backbone.NewHDLCDecoder(4096, func(pkt []byte) {
				ch <- append([]byte(nil), pkt...)
			})
			buf := make([]byte, 64*1024)
			for {
				nr, err := conn.Read(buf)
				if nr > 0 {
					d.Feed(buf[:nr])
				}
				if err != nil {
					break
				}
			}
			_ = conn.Close()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, ch
}

func runPythonHDLCClient(t *testing.T, port, n int, fault string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pythonExe(), pyScript(t, "hdlc_burst.py"))
	cmd.Env = append(os.Environ(),
		"INTEROP_MODE=client",
		"INTEROP_TARGET_PORT="+strconv.Itoa(port),
		"INTEROP_FRAMES="+strconv.Itoa(n),
		"INTEROP_FAULT="+fault,
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("python client: %v", err)
	}
}

func collectFrames(t *testing.T, got <-chan []byte, min int) [][]byte {
	t.Helper()
	deadline := time.After(5 * time.Second)
	var frames [][]byte
	for {
		select {
		case f := <-got:
			frames = append(frames, f)
			if len(frames) >= min {
				drain := time.After(200 * time.Millisecond)
			drainLoop:
				for {
					select {
					case extra := <-got:
						frames = append(frames, extra)
					case <-drain:
						break drainLoop
					}
				}
				return frames
			}
		case <-deadline:
			if len(frames) >= min {
				return frames
			}
			t.Fatalf("got %d frames want >= %d", len(frames), min)
		}
	}
}

func TestLiveInteropHDLCCorruptPythonToGo(t *testing.T) {
	liveOrSkip(t)
	const n = 16
	port, got := listenHDLCFrames(t, 1, n)
	runPythonHDLCClient(t, port, n, "corrupt")
	frames := collectFrames(t, got, n)
	if len(frames) != n {
		t.Fatalf("got %d want %d", len(frames), n)
	}
	if !bytes.Equal(frames[0], burstPayload(0)) {
		t.Fatal("first frame lost")
	}
	if bytes.Equal(frames[1], burstPayload(1)) {
		t.Fatal("flipped byte did not change frame 1")
	}
	if !bytes.Equal(frames[n-1], burstPayload(n-1)) {
		t.Fatal("last good frame lost after flipped byte")
	}
}

func TestLiveInteropHDLCDropPythonToGo(t *testing.T) {
	liveOrSkip(t)
	const n = 16
	port, got := listenHDLCFrames(t, 1, n)
	runPythonHDLCClient(t, port, n, "drop")
	frames := collectFrames(t, got, n-1)
	if len(frames) != n-1 {
		t.Fatalf("got %d want %d", len(frames), n-1)
	}
	mid := burstPayload(n / 2)
	for _, f := range frames {
		if bytes.Equal(f, mid) {
			t.Fatal("dropped frame was delivered")
		}
	}
}

func TestLiveInteropHDLCReorderPythonToGo(t *testing.T) {
	liveOrSkip(t)
	const n = 16
	port, got := listenHDLCFrames(t, 1, n)
	runPythonHDLCClient(t, port, n, "reorder")
	frames := collectFrames(t, got, n)
	if len(frames) != n {
		t.Fatalf("got %d want %d", len(frames), n)
	}
	if !bytes.Equal(frames[0], burstPayload(1)) || !bytes.Equal(frames[1], burstPayload(0)) {
		t.Fatalf("reorder not preserved: first=%x", frames[0])
	}
}

func TestLiveInteropHDLCFlapPythonToGo(t *testing.T) {
	liveOrSkip(t)
	const n = 16
	port, got := listenHDLCFrames(t, 2, n)
	runPythonHDLCClient(t, port, n, "flap")
	frames := collectFrames(t, got, n)
	if len(frames) != n {
		t.Fatalf("got %d want %d after flap", len(frames), n)
	}
	for i := range n {
		if !bytes.Equal(frames[i], burstPayload(i)) {
			t.Fatalf("frame %d mismatch after flap", i)
		}
	}
}
