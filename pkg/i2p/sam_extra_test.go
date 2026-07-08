// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package i2p

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/cryptography"
)

// fakeSAM starts a single-connection SAM-like TCP server that replies to
// each command line with the first canned reply whose key is a prefix of the
// command. It returns the listener address and a cleanup function.
func fakeSAM(t *testing.T, replies map[string]string) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				br := bufio.NewReader(conn)
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					matched := false
					for prefix, reply := range replies {
						if strings.HasPrefix(line, prefix) {
							_, _ = conn.Write([]byte(reply))
							matched = true
							break
						}
					}
					if !matched {
						_, _ = conn.Write([]byte("SESSION STATUS RESULT=I2P_ERROR\n"))
					}
				}
			}(conn)
		}
	}()
	cleanup := func() {
		_ = ln.Close()
		<-done
	}
	return ln.Addr().String(), cleanup
}

func mustPrivateB64(t *testing.T, n int) string {
	t.Helper()
	raw := make([]byte, n)
	for i := range raw {
		raw[i] = 0x41
	}
	// Embed an explicit zero cert length at the cert_len offset so the
	// payload parses regardless of n.
	if n >= 387 {
		binary.BigEndian.PutUint16(raw[385:387], 0)
	}
	return i2pB64Encode(raw)
}

func TestI2PB64RoundTrip(t *testing.T) {
	in := []byte("hello i2p world")
	enc := i2pB64Encode(in)
	dec, err := i2pB64Decode(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(dec) != string(in) {
		t.Fatalf("round-trip mismatch: %q vs %q", dec, in)
	}
}

func TestI2PB64DecodeInvalid(t *testing.T) {
	if _, err := i2pB64Decode("!!!invalid base64!!!"); err == nil {
		t.Fatal("expected decode error for invalid input")
	}
}

func TestNewDestinationFromB64(t *testing.T) {
	raw := make([]byte, 387)
	for i := range raw {
		raw[i] = 0x42
	}
	b64 := i2pB64Encode(raw)
	d, err := NewDestinationFromB64(b64)
	if err != nil {
		t.Fatalf("NewDestinationFromB64: %v", err)
	}
	if d.Base64() != b64 {
		t.Errorf("Base64 mismatch: got %q, want %q", d.Base64(), b64)
	}
	if got := d.Base32(); len(got) != 52 {
		t.Errorf("Base32 length: got %d, want 52 (%q)", len(got), got)
	}
	if d.PrivateKeyB64() != "" {
		t.Errorf("public-only destination should have empty private key, got %q", d.PrivateKeyB64())
	}
}

func TestNewDestinationFromB64Invalid(t *testing.T) {
	if _, err := NewDestinationFromB64("!!!not b64!!!"); err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestNewDestinationFromPrivateB64(t *testing.T) {
	b64 := mustPrivateB64(t, 400)
	d, err := NewDestinationFromPrivateB64(b64)
	if err != nil {
		t.Fatalf("NewDestinationFromPrivateB64: %v", err)
	}
	if d.PrivateKeyB64() == "" {
		t.Fatal("expected non-empty private key")
	}
	if d.Base32() == "" {
		t.Fatal("expected non-empty base32")
	}
}

func TestNewDestinationFromPrivateB64TooShort(t *testing.T) {
	short := i2pB64Encode(make([]byte, 100))
	if _, err := NewDestinationFromPrivateB64(short); err == nil {
		t.Fatal("expected error for too-short private key")
	}
}

func TestNewDestinationFromPrivateB64CertOverflow(t *testing.T) {
	raw := make([]byte, 387)
	for i := range raw {
		raw[i] = 0x55
	}
	// Declare a cert length that overflows the available buffer.
	binary.BigEndian.PutUint16(raw[385:387], 100)
	if _, err := NewDestinationFromPrivateB64(i2pB64Encode(raw)); err == nil {
		t.Fatal("expected error when declared cert length exceeds buffer")
	}
}

func TestResolveDestinationEmpty(t *testing.T) {
	if _, err := ResolveDestination("  ", nil); err == nil {
		t.Fatal("expected error for empty destination")
	}
}

func TestResolveDestinationI2pWithoutLookup(t *testing.T) {
	// A .i2p name with a nil lookup falls through to passthrough.
	got, err := ResolveDestination("nomatch.i2p", nil)
	if err != nil {
		t.Fatalf("ResolveDestination: %v", err)
	}
	if got != "nomatch.i2p" {
		t.Errorf("got %q, want passthrough", got)
	}
}

func TestResolveDestinationLookupError(t *testing.T) {
	wantErr := fmt.Errorf("lookup failed")
	_, err := ResolveDestination("example.i2p", func(name string) (string, error) {
		return "", wantErr
	})
	if err == nil || !strings.Contains(err.Error(), "lookup failed") {
		t.Fatalf("expected lookup error propagation, got %v", err)
	}
}

func TestResolveDestinationNonB64Passthrough(t *testing.T) {
	// Neither valid b64 nor .i2p: should pass through unchanged.
	got, err := ResolveDestination("some-identifier", nil)
	if err != nil {
		t.Fatalf("ResolveDestination: %v", err)
	}
	if got != "some-identifier" {
		t.Errorf("got %q, want passthrough", got)
	}
}

func TestSAMErrorText(t *testing.T) {
	e := &SAMError{Code: "I2P_ERROR"}
	if e.Error() != "i2p: SAM I2P_ERROR" {
		t.Errorf("unexpected error text: %q", e.Error())
	}
}

func TestSamErrorFromResult(t *testing.T) {
	cases := []string{
		"CANT_REACH_PEER", "DUPLICATED_DEST", "DUPLICATED_ID",
		"I2P_ERROR", "INVALID_ID", "INVALID_KEY", "KEY_NOT_FOUND",
		"PEER_NOT_FOUND", "TIMEOUT", "UNKNOWN_CODE",
	}
	for _, c := range cases {
		err := samErrorFromResult(c)
		if err == nil {
			t.Errorf("samErrorFromResult(%q) returned nil", c)
			continue
		}
		if !strings.Contains(err.Error(), c) {
			t.Errorf("error %q missing code %q", err.Error(), c)
		}
	}
}

func TestParseMessageEmpty(t *testing.T) {
	if _, err := parseMessage(""); err == nil {
		t.Fatal("expected error for empty line")
	}
	if _, err := parseMessage("   \n"); err == nil {
		t.Fatal("expected error for whitespace-only line")
	}
}

func TestParseMessageTooShort(t *testing.T) {
	if _, err := parseMessage("oneword"); err == nil {
		t.Fatal("expected error for single-token line")
	}
}

func TestParseMessageBareToken(t *testing.T) {
	// A token without '=' is stored as "true".
	m, err := parseMessage("STREAM STATUS SILENT\n")
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if m.Opts["SILENT"] != "true" {
		t.Errorf("bare token should map to true, got %q", m.Opts["SILENT"])
	}
}

func TestParseMessageNoResultIsInvalid(t *testing.T) {
	m, err := parseMessage("STREAM STATUS\n")
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if m.OK() {
		t.Fatal("message without RESULT on non-REPLY action should not be OK")
	}
	if err := m.ResultError(); err != ErrInvalidResponse {
		t.Errorf("expected ErrInvalidResponse, got %v", err)
	}
}

func TestMessageOKOnReplyWithoutResult(t *testing.T) {
	// DEST REPLY without explicit RESULT is considered OK.
	m, err := parseMessage("NAMING REPLY VALUE=abc\n")
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if !m.OK() {
		t.Fatal("REPLY without RESULT should be OK")
	}
	if err := m.ResultError(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if m.Opts["VALUE"] != "abc" {
		t.Errorf("VALUE: got %q", m.Opts["VALUE"])
	}
}

func TestReadLineSuccess(t *testing.T) {
	a, b := net.Pipe()
	go func() {
		_, _ = a.Write([]byte("HELLO REPLY RESULT=OK\n"))
		_ = a.Close()
	}()
	got, err := readLine(b)
	if err != nil {
		t.Fatalf("readLine: %v", err)
	}
	if strings.TrimSpace(got) != "HELLO REPLY RESULT=OK" {
		t.Errorf("readLine: got %q", got)
	}
	_ = b.Close()
}

func TestReadLineEOFReportsSAMOffline(t *testing.T) {
	a, b := net.Pipe()
	_ = a.Close()
	if _, err := readLine(b); err != ErrSAMOffline {
		t.Fatalf("expected ErrSAMOffline, got %v", err)
	}
	_ = b.Close()
}

func TestFreePort(t *testing.T) {
	p, err := FreePort()
	if err != nil {
		t.Fatalf("FreePort: %v", err)
	}
	if p < 1 || p > 65535 {
		t.Fatalf("FreePort returned out-of-range port %d", p)
	}
	// A second call should return a different port almost always.
	p2, err := FreePort()
	if err != nil {
		t.Fatalf("FreePort second: %v", err)
	}
	if p == p2 {
		t.Logf("note: FreePort returned same port twice (%d)", p)
	}
}

func TestGenerateSessionID(t *testing.T) {
	id := GenerateSessionID()
	if !strings.HasPrefix(id, "reticulum-") {
		t.Errorf("session id should have reticulum- prefix, got %q", id)
	}
	if len(id) != len("reticulum-")+8 {
		t.Errorf("session id length: got %d, want %d", len(id), len("reticulum-")+8)
	}
	id2 := GenerateSessionID()
	if id == id2 {
		t.Logf("note: GenerateSessionID returned same value twice (%q)", id)
	}
}

func TestNewClientDefaults(t *testing.T) {
	t.Setenv("I2P_SAM_ADDRESS", "")
	c := NewClient("")
	if c.Address != defaultSAMAddress {
		t.Errorf("default address: got %q, want %q", c.Address, defaultSAMAddress)
	}
	if c.Timeout != defaultSAMTimeout*time.Second {
		t.Errorf("default timeout: got %v, want %v", c.Timeout, defaultSAMTimeout*time.Second)
	}
}

func TestNewClientEnvOverride(t *testing.T) {
	t.Setenv("I2P_SAM_ADDRESS", "127.0.0.1:1234")
	c := NewClient("")
	if c.Address != "127.0.0.1:1234" {
		t.Errorf("env override: got %q", c.Address)
	}
}

func TestClientDialAndHelloOK(t *testing.T) {
	addr, cleanup := fakeSAM(t, map[string]string{
		"HELLO": "HELLO REPLY RESULT=OK\n",
	})
	defer cleanup()
	c := NewClient(addr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := c.dial(ctx)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()
}

func TestClientDialHelloFails(t *testing.T) {
	addr, cleanup := fakeSAM(t, map[string]string{
		"HELLO": "HELLO REPLY RESULT=I2P_ERROR\n",
	})
	defer cleanup()
	c := NewClient(addr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.dial(ctx); err == nil {
		t.Fatal("expected dial to fail when HELLO returns I2P_ERROR")
	}
}

func TestClientDialOffline(t *testing.T) {
	c := NewClient("127.0.0.1:1")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := c.dial(ctx); err == nil {
		t.Fatal("expected dial to fail on unreachable address")
	}
}

func TestClientNamingLookup(t *testing.T) {
	addr, cleanup := fakeSAM(t, map[string]string{
		"HELLO":         "HELLO REPLY RESULT=OK\n",
		"NAMING LOOKUP": "NAMING REPLY RESULT=OK VALUE=looked-up\n",
	})
	defer cleanup()
	c := NewClient(addr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := c.NamingLookup(ctx, "example.i2p")
	if err != nil {
		t.Fatalf("NamingLookup: %v", err)
	}
	if got != "looked-up" {
		t.Errorf("NamingLookup: got %q", got)
	}
}

func TestClientNamingLookupError(t *testing.T) {
	addr, cleanup := fakeSAM(t, map[string]string{
		"HELLO":         "HELLO REPLY RESULT=OK\n",
		"NAMING LOOKUP": "NAMING REPLY RESULT=KEY_NOT_FOUND\n",
	})
	defer cleanup()
	c := NewClient(addr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.NamingLookup(ctx, "missing.i2p"); err == nil {
		t.Fatal("expected KEY_NOT_FOUND error")
	}
}

func TestClientDestGenerate(t *testing.T) {
	priv := mustPrivateB64(t, 400)
	addr, cleanup := fakeSAM(t, map[string]string{
		"HELLO":         "HELLO REPLY RESULT=OK\n",
		"DEST GENERATE": fmt.Sprintf("DEST REPLY PRIV=%s\n", priv),
	})
	defer cleanup()
	c := NewClient(addr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dest, err := c.DestGenerate(ctx)
	if err != nil {
		t.Fatalf("DestGenerate: %v", err)
	}
	if dest == nil || dest.PrivateKeyB64() == "" {
		t.Fatalf("DestGenerate returned incomplete destination: %+v", dest)
	}
}

func TestClientOpenAndCreateSession(t *testing.T) {
	addr, cleanup := fakeSAM(t, map[string]string{
		"HELLO":          "HELLO REPLY RESULT=OK\n",
		"SESSION CREATE": "SESSION STATUS RESULT=OK\n",
	})
	defer cleanup()
	c := NewClient(addr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sess, err := c.OpenSession(ctx, "sid", "TRANSIENT")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	if sess.ID != "sid" {
		t.Errorf("session id: got %q", sess.ID)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Session.Close: %v", err)
	}
	// Second close must be a safe no-op.
	if err := sess.Close(); err != nil {
		t.Fatalf("Session.Close second call: %v", err)
	}

	// CreateSession opens then immediately closes; should succeed.
	if err := c.CreateSession(ctx, "sid2", "TRANSIENT"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
}

func TestClientOpenSessionFails(t *testing.T) {
	addr, cleanup := fakeSAM(t, map[string]string{
		"HELLO":          "HELLO REPLY RESULT=OK\n",
		"SESSION CREATE": "SESSION STATUS RESULT=DUPLICATED_ID\n",
	})
	defer cleanup()
	c := NewClient(addr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.OpenSession(ctx, "sid", "TRANSIENT"); err == nil {
		t.Fatal("expected OpenSession to fail on DUPLICATED_ID")
	}
}

func TestClientStreamConnectAndAccept(t *testing.T) {
	addr, cleanup := fakeSAM(t, map[string]string{
		"HELLO":          "HELLO REPLY RESULT=OK\n",
		"SESSION CREATE": "SESSION STATUS RESULT=OK\n",
		"STREAM CONNECT": "STREAM STATUS RESULT=OK\n",
		"STREAM ACCEPT":  "STREAM STATUS RESULT=OK\n",
	})
	defer cleanup()
	c := NewClient(addr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := c.StreamConnect(ctx, "sid", "destination-b64")
	if err != nil {
		t.Fatalf("StreamConnect: %v", err)
	}
	_ = conn.Close()

	accConn, err := c.StreamAccept(ctx, "sid")
	if err != nil {
		t.Fatalf("StreamAccept: %v", err)
	}
	_ = accConn.Close()
}

func TestClientStreamAcceptEmptySession(t *testing.T) {
	c := NewClient("127.0.0.1:1")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := c.StreamAccept(ctx, ""); err != ErrInvalidResponse {
		t.Fatalf("expected ErrInvalidResponse for empty session, got %v", err)
	}
}

func TestNewClientTunnelAllocatesPort(t *testing.T) {
	tun, err := NewClientTunnel(NewClient(""), "dest.b32.i2p", 0)
	if err != nil {
		t.Fatalf("NewClientTunnel: %v", err)
	}
	if tun.LocalAddr() == "" || !strings.HasPrefix(tun.LocalAddr(), "127.0.0.1:") {
		t.Errorf("LocalAddr: got %q", tun.LocalAddr())
	}
	st := tun.Status()
	if st.SetupRan {
		t.Error("fresh tunnel should not report SetupRan")
	}
}

func TestNewClientTunnelExplicitPort(t *testing.T) {
	tun, err := NewClientTunnel(NewClient(""), "dest.b32.i2p", 12345)
	if err != nil {
		t.Fatalf("NewClientTunnel: %v", err)
	}
	if tun.LocalAddr() != "127.0.0.1:12345" {
		t.Errorf("LocalAddr: got %q, want 127.0.0.1:12345", tun.LocalAddr())
	}
}

func TestNewServerTunnelAllocatesPort(t *testing.T) {
	tun, err := NewServerTunnel(NewClient(""), nil, 0)
	if err != nil {
		t.Fatalf("NewServerTunnel: %v", err)
	}
	if tun.LocalAddr() == "" {
		t.Error("expected non-empty LocalAddr")
	}
	if tun.Destination() != nil {
		t.Error("expected nil destination to round-trip")
	}
	st := tun.Status()
	if st.SetupRan {
		t.Error("fresh server tunnel should not report SetupRan")
	}
}

func TestServerTunnelStopIdempotent(t *testing.T) {
	tun, err := NewServerTunnel(NewClient(""), nil, 0)
	if err != nil {
		t.Fatalf("NewServerTunnel: %v", err)
	}
	tun.Stop()
	tun.Stop() // must not panic
}

func TestNewController(t *testing.T) {
	dir := t.TempDir()
	ctrl := NewController(dir, "127.0.0.1:7656")
	if ctrl == nil {
		t.Fatal("NewController returned nil")
	}
	p, err := ctrl.FreePort()
	if err != nil {
		t.Fatalf("FreePort: %v", err)
	}
	if p < 1 {
		t.Fatalf("FreePort returned %d", p)
	}
	// Stop must be safe to call even with no tunnels running.
	ctrl.Stop()
	ctrl.Stop()
}

func TestControllerLoadOrCreateDestinationPersists(t *testing.T) {
	dir := t.TempDir()
	ctrl := NewController(dir, "127.0.0.1:7656")
	defer ctrl.Stop()

	transportID := []byte("transport-id-bytes")
	priv := mustPrivateB64(t, 400)
	dest, err := NewDestinationFromPrivateB64(priv)
	if err != nil {
		t.Fatalf("NewDestinationFromPrivateB64: %v", err)
	}

	// Persist a destination file at the "old" name-hash path the controller
	// checks first, then ensure loadOrCreateDestination loads it instead of
	// contacting the (offline) SAM router.
	nameHash := cryptography.Hash(cryptography.Hash([]byte("iface-x")))
	keyFile := filepath.Join(dir, hex.EncodeToString(nameHash)+".i2p")
	if err := os.WriteFile(keyFile, []byte(dest.PrivateKeyB64()), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	got, err := ctrl.loadOrCreateDestination("iface-x", transportID)
	if err != nil {
		t.Fatalf("loadOrCreateDestination: %v", err)
	}
	if got.Base32() != dest.Base32() {
		t.Errorf("loaded destination mismatch: got %q, want %q",
			got.Base32(), dest.Base32())
	}
}
