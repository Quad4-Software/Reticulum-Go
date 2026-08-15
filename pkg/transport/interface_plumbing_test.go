// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"errors"
	"net"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/interfaces"
)

const tcpPlumbingEnvVar = "RETICULUM_RUN_TCP_PLUMBING"

// requireTCPPlumbing skips the calling test unless the opt-in environment
// variable is set to a truthy value.
func requireTCPPlumbing(t *testing.T) {
	t.Helper()
	switch strings.ToLower(os.Getenv(tcpPlumbingEnvVar)) {
	case "1", "true", "yes", "on":
		return
	default:
		t.Skipf("skipping TCP plumbing test; set %s=1 to enable", tcpPlumbingEnvVar)
	}
}

type trackingIface struct {
	common.BaseInterface
	mu              sync.Mutex
	getNameCalls    int
	sendCalls       int
	processOutCalls int
	sent            [][]byte
}

func newTrackingIface(name string) *trackingIface {
	c := &trackingIface{
		BaseInterface: common.NewBaseInterface(name, common.IFTypeTCP, true),
	}
	c.Enable()
	return c
}

func (c *trackingIface) Send(data []byte, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sendCalls++
	cp := make([]byte, len(data))
	copy(cp, data)
	c.sent = append(c.sent, cp)
	return nil
}

func (c *trackingIface) ProcessOutgoing(data []byte) error {
	c.mu.Lock()
	c.processOutCalls++
	c.mu.Unlock()
	return c.Send(data, "")
}

func (c *trackingIface) GetName() string {
	c.mu.Lock()
	c.getNameCalls++
	c.mu.Unlock()
	return c.Name
}

func (c *trackingIface) IsEnabled() bool { return c.Enabled }

func (c *trackingIface) counts() (getName, send, processOut int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getNameCalls, c.sendCalls, c.processOutCalls
}

// TestRegisterInterfaceRejectsAbstractBase ensures the transport refuses to
// register a bare BaseInterface, which would silently swallow every outgoing
// packet because BaseInterface.ProcessOutgoing is a no-op.
func TestRegisterInterfaceRejectsAbstractBase(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	bareCommon := &common.BaseInterface{Name: "bare-common"}
	if err := tr.RegisterInterface("bare-common", bareCommon); err == nil {
		t.Fatal("expected error registering *common.BaseInterface, got nil")
	} else if !strings.Contains(err.Error(), "abstract base") {
		t.Fatalf("expected abstract-base error, got %v", err)
	}

	bareIfaces := &interfaces.BaseInterface{Name: "bare-ifaces"}
	if err := tr.RegisterInterface("bare-ifaces", bareIfaces); err == nil {
		t.Fatal("expected error registering *interfaces.BaseInterface, got nil")
	}
}

// TestRegisterInterfacePreservesConcreteType verifies that RegisterInterface
// stores and forwards the concrete iface pointer (not the embedded
// *BaseInterface) so subsequent calls through GetInterface and through the
// transport's own packet callback closure operate on the wrapper that owns
// the real Send / ProcessOutgoing implementations.
//
// The check that actually catches the original dynamic-dispatch bug:
// invoke the installed callback with a *different* iface pointer as its
// NetworkInterface argument and assert that the registered iface (not the
// supplied one) is the one HandlePacket goes on to use. We detect that by
// counting GetName calls on each iface - HandlePacket calls iface.GetName
// at the top of every dispatch.
func TestRegisterInterfacePreservesConcreteType(t *testing.T) {
	prevLevel := debug.GetDebugLevel()
	debug.SetDebugLevel(debug.DebugTrace)
	defer debug.SetDebugLevel(prevLevel)

	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	registered := newTrackingIface("registered-iface")
	if err := tr.RegisterInterface(registered.GetName(), registered); err != nil {
		t.Fatalf("RegisterInterface failed: %v", err)
	}

	stored, err := tr.GetInterface(registered.GetName())
	if err != nil {
		t.Fatalf("GetInterface: %v", err)
	}
	if stored != common.NetworkInterface(registered) {
		t.Fatalf("transport stored wrong iface pointer:\n  want=%T %p\n   got=%T %p",
			registered, registered, stored, stored)
	}

	cb := registered.GetPacketCallback()
	if cb == nil {
		t.Fatal("transport did not install a packet callback on the registered iface")
	}

	// Reset the counter for the registered iface AFTER capturing it from
	// transport state (transport startup may have invoked GetName).
	registered.mu.Lock()
	registered.getNameCalls = 0
	registered.mu.Unlock()

	imposter := newTrackingIface("imposter-iface")

	// Hand the imposter to the callback as the NetworkInterface argument.
	// If RegisterInterface's closure mistakenly forwards its argument to
	// HandlePacket (the original bug), HandlePacket would call
	// imposter.GetName. The fix substitutes the captured registered iface
	// instead, so registered.GetName must increment and imposter.GetName
	// must stay at zero.
	cb([]byte{0x00, 0xff}, imposter)

	regCalls, _, _ := registered.counts()
	impCalls, _, _ := imposter.counts()

	if regCalls == 0 {
		t.Fatalf("registered iface GetName not called by HandlePacket; closure did not substitute the captured concrete iface")
	}
	if impCalls != 0 {
		t.Fatalf("imposter iface GetName called %d times; closure forwarded its argument instead of the captured concrete iface (this is the dynamic-dispatch bug)", impCalls)
	}
}

// TestTransportLoopbackOverTCP spins up a real TCP server interface and a
// real TCP client interface inside a single process. It verifies bytes
// pushed via the client's Send method arrive at the server transport's
// packet pipeline, and vice versa. This is the smallest possible test that
// would have caught the dynamic-dispatch bug end-to-end: if Send silently
// no-ops, no packet ever reaches the peer.
func TestTransportLoopbackOverTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TCP loopback test in -short mode")
	}
	requireTCPPlumbing(t)

	port, err := pickFreeTCPPort()
	if err != nil {
		t.Fatalf("pick free port: %v", err)
	}

	server, err := interfaces.NewTCPServerInterface("loopback-server", "127.0.0.1", port, false, false, false)
	if err != nil {
		t.Fatalf("server interface: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("server start: %v", err)
	}
	defer server.Stop() // #nosec G104

	srvTr := NewTransport(&common.ReticulumConfig{})
	defer srvTr.Close()

	srvRx := newRecordingPacketSink()
	if err := srvTr.RegisterInterface(server.GetName(), server); err != nil {
		t.Fatalf("register server iface: %v", err)
	}
	teeCallback(server, srvRx.record)

	client, err := interfaces.NewTCPClientInterface("loopback-client", "127.0.0.1", port, false, false, true)
	if err != nil {
		t.Fatalf("client interface: %v", err)
	}
	defer client.Stop() // #nosec G104

	cliTr := NewTransport(&common.ReticulumConfig{})
	defer cliTr.Close()

	cliRx := newRecordingPacketSink()
	if err := cliTr.RegisterInterface(client.GetName(), client); err != nil {
		t.Fatalf("register client iface: %v", err)
	}
	teeCallback(client, cliRx.record)

	if err := waitForServerConnection(server, 2*time.Second); err != nil {
		t.Fatalf("server never accepted connection: %v", err)
	}

	clientToServer := []byte{0x21, 0x00, 0xde, 0xad, 0xbe, 0xef}
	if err := client.Send(clientToServer, ""); err != nil {
		t.Fatalf("client send: %v", err)
	}
	if !srvRx.waitFor(clientToServer, 2*time.Second) {
		t.Fatalf("server never received bytes from client; got %d packets", len(srvRx.snapshot()))
	}

	serverToClient := []byte{0x22, 0x01, 0xfe, 0xed, 0xfa, 0xce}
	if err := server.Send(serverToClient, ""); err != nil {
		t.Fatalf("server send: %v", err)
	}
	if !cliRx.waitFor(serverToClient, 2*time.Second) {
		t.Fatalf("client never received bytes from server; got %d packets", len(cliRx.snapshot()))
	}
}

func pickFreeTCPPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close() // #nosec G104
	return l.Addr().(*net.TCPAddr).Port, nil
}

// teeCallback wraps the existing packet callback installed on iface so that
// fn runs in addition to the original callback. This lets a test observe
// inbound bytes without disconnecting them from the transport.
func teeCallback(iface interface {
	GetPacketCallback() common.PacketCallback
	SetPacketCallback(common.PacketCallback)
}, fn func([]byte, common.NetworkInterface)) {
	prev := iface.GetPacketCallback()
	iface.SetPacketCallback(func(data []byte, ni common.NetworkInterface) {
		fn(data, ni)
		if prev != nil {
			prev(data, ni)
		}
	})
}

type recordingPacketSink struct {
	mu      sync.Mutex
	cond    *sync.Cond
	packets [][]byte
}

func newRecordingPacketSink() *recordingPacketSink {
	r := &recordingPacketSink{}
	r.cond = sync.NewCond(&r.mu)
	return r
}

func (r *recordingPacketSink) record(data []byte, _ common.NetworkInterface) {
	cp := make([]byte, len(data))
	copy(cp, data)
	r.mu.Lock()
	r.packets = append(r.packets, cp)
	r.cond.Broadcast()
	r.mu.Unlock()
}

func (r *recordingPacketSink) snapshot() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]byte, len(r.packets))
	for i, p := range r.packets {
		c := make([]byte, len(p))
		copy(c, p)
		out[i] = c
	}
	return out
}

func (r *recordingPacketSink) waitFor(want []byte, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	// Background goroutine to wake the cond when the deadline expires.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-time.After(time.Until(deadline)):
			r.mu.Lock()
			r.cond.Broadcast()
			r.mu.Unlock()
		case <-stop:
		}
	}()

	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		for _, p := range r.packets {
			if bytes.Equal(p, want) {
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		r.cond.Wait()
	}
}

func waitForServerConnection(server *interfaces.TCPServerInterface, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if hasServerConn(server) {
			time.Sleep(20 * time.Millisecond)
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("timed out waiting for incoming TCP connection")
}

func hasServerConn(server *interfaces.TCPServerInterface) bool {
	rv := reflect.ValueOf(server).Elem().FieldByName("connections")
	if !rv.IsValid() {
		return false
	}
	return rv.Len() > 0
}

// Compile-time assertions: the abstract base types referenced by the
// transport guard remain assignable to common.NetworkInterface. If the
// interface contract changes, these blocks will fail to compile and the
// abstractBaseInterfaceTypes list in transport.go must be updated.
var (
	_ common.NetworkInterface = (*common.BaseInterface)(nil)
	_ common.NetworkInterface = (*interfaces.BaseInterface)(nil)
)
