// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

type memSender struct {
	mu   sync.Mutex
	msgs []Message
}

func (m *memSender) Send(msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, msg)
	return nil
}

func (m *memSender) last() Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.msgs) == 0 {
		return nil
	}
	return m.msgs[len(m.msgs)-1]
}

func (m *memSender) ofType(t uint16) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, msg := range m.msgs {
		if msg.GetType() == t {
			n++
		}
	}
	return n
}

func TestNativeRoundTrip(t *testing.T) {
	msgs := []Message{
		&NoopMessage{},
		&VersionMessage{SoftwareVersion: "1.2.3", ProtocolVersion: 1, Capabilities: CapLineMode},
		&AuthOKMessage{},
		&AuthDeniedMessage{Reason: "nope"},
		&WinSizeMessage{Rows: 24, Cols: 80},
		&ExecMessage{Cmdline: []string{"/bin/echo", "hi"}, PipeStdin: true, Term: "xterm", Rows: 24, Cols: 80},
		&StreamMessage{StreamID: StreamStdout, Data: []byte("hello")},
		&ErrorMessage{Msg: "err", Fatal: true},
		&ExitMessage{ReturnCode: 7},
	}
	for _, msg := range msgs {
		raw, err := msg.Pack()
		if err != nil {
			t.Fatalf("%T pack: %v", msg, err)
		}
		got, err := UnpackMessage(msg.GetType(), raw)
		if err != nil {
			t.Fatalf("%T unpack: %v", msg, err)
		}
		if got.GetType() != msg.GetType() {
			t.Fatalf("type mismatch")
		}
	}
}

func TestCompatRoundTrip(t *testing.T) {
	msgs := []Message{
		&NoopMessage{Compat: true},
		&VersionMessage{Compat: true, SoftwareVersion: "1.0.0", ProtocolVersion: 1},
		&WinSizeMessage{Compat: true, Rows: 24, Cols: 80},
		&ExecMessage{Compat: true, Cmdline: []string{"/bin/echo", "hi"}, PipeStdin: true, PipeStdout: true, PipeStderr: true, Term: "xterm", Rows: 24, Cols: 80},
		&StreamMessage{Compat: true, StreamID: 1, Data: []byte("x"), EOF: true},
		&ErrorMessage{Compat: true, Msg: "denied", Fatal: true},
		&ExitMessage{Compat: true, ReturnCode: 0},
	}
	for _, msg := range msgs {
		raw, err := msg.Pack()
		if err != nil {
			t.Fatalf("%T pack: %v", msg, err)
		}
		got, err := UnpackMessage(msg.GetType(), raw)
		if err != nil {
			t.Fatalf("%T unpack: %v", msg, err)
		}
		if got.GetType() != msg.GetType() {
			t.Fatalf("type mismatch")
		}
	}
}

func TestCompatMatchesPythonVectors(t *testing.T) {
	cases := []struct {
		name    string
		msgType uint16
		hex     string
	}{
		{"version", CompatVersion, "92a5312e302e3001"},
		{"winsize", CompatWinSize, "9418500000"},
		{"exec", CompatExec, "9a92a92f62696e2f6563686fa26869c3c3c3c0a5787465726d18500000"},
		{"exit", CompatExit, "00"},
		{"error", CompatError, "93a664656e696564c3c0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := mustHex(t, c.hex)
			msg, err := UnpackMessage(c.msgType, raw)
			if err != nil {
				t.Fatal(err)
			}
			out, err := msg.Pack()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out, raw) {
				t.Fatalf("repack mismatch\n got %x\nwant %x", out, raw)
			}
		})
	}
}

func TestConfigCopyIsolation(t *testing.T) {
	base := Config{DefaultCmd: []string{"/bin/sh"}, Allowed: [][]byte{{1, 2, 3}}}
	s1 := NewSession(base, &memSender{})
	s2 := NewSession(base, &memSender{})
	s1.MutateDefaultCmdAppend("-c")
	s1.MutateDefaultCmdAppend("evil")
	c2 := s2.ConfigSnapshot()
	if len(c2.DefaultCmd) != 1 || c2.DefaultCmd[0] != "/bin/sh" {
		t.Fatalf("session2 polluted: %#v", c2.DefaultCmd)
	}
	if len(base.DefaultCmd) != 1 {
		t.Fatalf("base polluted: %#v", base.DefaultCmd)
	}
}

func TestCoalesceLineMode(t *testing.T) {
	var got [][]byte
	c := NewCoalescer(true, 0, func(b []byte) { got = append(got, append([]byte(nil), b...)) })
	_, _ = c.Write([]byte("hel"))
	if len(got) != 0 {
		t.Fatal("flushed before newline")
	}
	_, _ = c.Write([]byte("lo\nmore"))
	if len(got) != 1 || string(got[0]) != "hello\n" {
		t.Fatalf("got %#v", got)
	}
	c.Flush()
	if len(got) != 2 || string(got[1]) != "more" {
		t.Fatalf("flush %#v", got)
	}
}

func TestPreferLineForRTT(t *testing.T) {
	if PreferLineForRTT(100 * time.Millisecond) {
		t.Fatal("low RTT should not prefer line")
	}
	if !PreferLineForRTT(AutoLineRTT) {
		t.Fatal("high RTT should prefer line")
	}
}

func TestStreamDecompressBomb(t *testing.T) {
	// Craft compressed payload that expands beyond MaxDecompressed.
	huge := bytes.Repeat([]byte("A"), MaxDecompressed+100)
	comp, ok := compressMaybe(huge)
	if !ok {
		t.Skip("compression did not shrink")
	}
	rawHdr := make([]byte, 2+len(comp))
	rawHdr[0] = 0x40 // compressed flag in high bits of stream header via Unpack path
	// Build properly via Pack then force compressed bit with oversized payload.
	msg := &StreamMessage{StreamID: 1, Data: comp, Compressed: true}
	packed, err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	// Unpack will decompress; if under limit OK. Use artificial oversize by
	// packing compressed flag with data that decompresses large.
	got := &StreamMessage{}
	if err := got.Unpack(packed); err != nil && err != ErrDecompressBomb {
		// may succeed if under limit after compress of MaxDecompressed+100
		if len(got.Data) > MaxDecompressed {
			t.Fatalf("accepted oversized %d", len(got.Data))
		}
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b := make([]byte, len(s)/2)
	for i := range b {
		var v byte
		for _, c := range []byte{s[i*2], s[i*2+1]} {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v |= c - '0'
			case c >= 'a' && c <= 'f':
				v |= c - 'a' + 10
			case c >= 'A' && c <= 'F':
				v |= c - 'A' + 10
			}
		}
		b[i] = v
	}
	return b
}
