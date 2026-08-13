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
