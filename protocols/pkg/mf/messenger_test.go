package mf

import (
	"bytes"
	"encoding/hex"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

const (
	testAddrLoopback = "127.0.0.1:0"
	testInterface1   = "UDP1"
	testInterface2   = "UDP2"
	testAppName      = "mf"
	testMaxWait      = 10 * time.Second
)

func TestMessenger_TwoWayCommunication(t *testing.T) {
	cfg1 := testconfig.New()
	cfg1.Interfaces = make(map[string]*common.InterfaceConfig)
	cfg1.Interfaces[testInterface1] = &common.InterfaceConfig{
		Type:       "UDPInterface",
		Enabled:    true,
		Address:    testAddrLoopback,
		TargetHost: testAddrLoopback,
		Name:       testInterface1,
	}

	cfg2 := testconfig.New()
	cfg2.Interfaces = make(map[string]*common.InterfaceConfig)
	cfg2.Interfaces[testInterface2] = &common.InterfaceConfig{
		Type:       "UDPInterface",
		Enabled:    true,
		Address:    testAddrLoopback,
		TargetHost: testAddrLoopback,
		Name:       testInterface2,
	}

	tr1 := transport.NewTransport(cfg1)
	tr2 := transport.NewTransport(cfg2)

	if err := tr1.Start(); err != nil {
		t.Fatalf("Failed to start transport 1: %v", err)
	}
	defer tr1.Close()

	if err := tr2.Start(); err != nil {
		t.Fatalf("Failed to start transport 2: %v", err)
	}
	defer tr2.Close()

	addr1 := "127.0.0.1:42420"
	addr2 := "127.0.0.1:42421"

	var iface1 interfaces.Interface
	iface1, err := interfaces.NewUDPInterface(testInterface1, addr1, addr2, true)
	if err != nil {
		t.Fatalf("Failed to create interface 1: %v", err)
	}
	iface1.SetPacketCallback(func(data []byte, ni common.NetworkInterface) {
		tr1.HandlePacket(data, ni)
	})
	if err := iface1.Start(); err != nil {
		t.Fatalf("Failed to start interface 1: %v", err)
	}
	defer iface1.Stop()

	if netIface1, ok := iface1.(common.NetworkInterface); ok {
		if err := tr1.RegisterInterface(testInterface1, netIface1); err != nil {
			t.Fatalf("Failed to register interface 1: %v", err)
		}
	}

	t.Logf("Interface 1 listening on: %s, targeting: %s", addr1, addr2)

	var iface2 interfaces.Interface
	iface2, err = interfaces.NewUDPInterface(testInterface2, addr2, addr1, true)
	if err != nil {
		t.Fatalf("Failed to create interface 2: %v", err)
	}
	iface2.SetPacketCallback(func(data []byte, ni common.NetworkInterface) {
		tr2.HandlePacket(data, ni)
	})
	if err := iface2.Start(); err != nil {
		t.Fatalf("Failed to start interface 2: %v", err)
	}
	defer iface2.Stop()

	if netIface2, ok := iface2.(common.NetworkInterface); ok {
		if err := tr2.RegisterInterface(testInterface2, netIface2); err != nil {
			t.Fatalf("Failed to register interface 2: %v", err)
		}
	}

	t.Logf("Interface 2 listening on: %s, targeting: %s", addr2, addr1)

	id1, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("Failed to create identity 1: %v", err)
	}

	id2, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("Failed to create identity 2: %v", err)
	}

	dest1, err := destination.New(id1, destination.In, destination.Single, testAppName, tr1)
	if err != nil {
		t.Fatalf("Failed to create destination 1: %v", err)
	}

	dest2, err := destination.New(id2, destination.In, destination.Single, testAppName, tr2)
	if err != nil {
		t.Fatalf("Failed to create destination 2: %v", err)
	}

	messenger1 := NewMessenger(tr1, dest1)
	messenger2 := NewMessenger(tr2, dest2)

	dest1Hash := dest1.GetHash()
	dest2Hash := dest2.GetHash()

	t.Logf("Destination 1 hash: %s", hex.EncodeToString(dest1Hash))
	t.Logf("Destination 2 hash: %s", hex.EncodeToString(dest2Hash))

	identity.Remember(nil, dest1Hash, id1.GetPublicKey(), nil)
	identity.Remember(nil, dest2Hash, id2.GetPublicKey(), nil)

	if err := dest1.Announce(false, nil, nil); err != nil {
		t.Fatalf("Failed to announce destination 1: %v", err)
	}
	t.Logf("Destination 1 announced")

	if err := dest2.Announce(false, nil, nil); err != nil {
		t.Fatalf("Failed to announce destination 2: %v", err)
	}
	t.Logf("Destination 2 announced")

	time.Sleep(2 * time.Second)

	start := time.Now()
	for {
		if tr1.HasPath(dest2Hash) && tr2.HasPath(dest1Hash) {
			t.Logf("Paths established after %v", time.Since(start))
			break
		}
		if time.Since(start) > testMaxWait {
			t.Fatalf("Timeout waiting for paths to be established")
		}
		time.Sleep(100 * time.Millisecond)
	}

	testMessage := "Hello from messenger 1!"
	t.Logf("Sending message from messenger 1: %s", testMessage)

	if err := messenger1.SendMessage(dest2Hash, testMessage); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	t.Logf("Message sent successfully")

	testMessage2 := "Hello from messenger 2!"
	t.Logf("Sending message from messenger 2: %s", testMessage2)

	if err := messenger2.SendMessage(dest1Hash, testMessage2); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	t.Logf("Both messages sent successfully")
}

func TestMessenger_QoL(t *testing.T) {
	cfg := testconfig.New()
	tr := transport.NewTransport(cfg)

	id, _ := identity.NewIdentity()
	dest, _ := destination.New(id, destination.In, destination.Single, testAppName, tr)

	m := NewMessenger(tr, dest)

	if !bytes.Equal(m.GetDestinationHash(), dest.GetHash()) {
		t.Error("GetDestinationHash returned incorrect hash")
	}

	if m.GetDestination() != dest {
		t.Error("GetDestination returned incorrect destination")
	}
}
