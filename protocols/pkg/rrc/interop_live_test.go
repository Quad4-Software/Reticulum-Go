// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"encoding/hex"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"strconv"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

type interopGoHub struct {
	tr      *transport.Transport
	iface   interfaces.Interface
	dest    *destination.Destination
	hub     *Hub
	hubHash []byte
	id      *identity.Identity
}

func startInteropGoHub(t *testing.T, goPort, pyPort int, tag string, cfg HubConfig) *interopGoHub {
	t.Helper()
	udpName := "RRCI-" + tag
	trCfg := testconfig.New()
	trCfg.Interfaces = map[string]*common.InterfaceConfig{
		udpName: {Type: "UDPInterface", Enabled: true, Address: "127.0.0.1:0", TargetHost: "127.0.0.1:0", Name: udpName},
	}
	tr := transport.NewTransport(trCfg)
	if err := tr.Start(); err != nil {
		t.Fatalf("transport: %v", err)
	}
	var iface interfaces.Interface
	var err error
	iface, err = interfaces.NewUDPInterface(udpName, addr(goPort), addr(pyPort), true)
	if err != nil {
		t.Fatalf("udp: %v", err)
	}
	iface.SetPacketCallback(func(d []byte, ni common.NetworkInterface) { tr.HandlePacket(d, ni) })
	if err := iface.Start(); err != nil {
		t.Fatalf("udp start: %v", err)
	}
	if ni, ok := iface.(common.NetworkInterface); ok {
		if err := tr.RegisterInterface(udpName, ni); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := NewHubDestination(id, tr)
	if err != nil {
		t.Fatal(err)
	}
	hub, err := NewHub(tr, dest, cfg)
	if err != nil {
		t.Fatal(err)
	}
	hub.Start()
	h := &interopGoHub{tr: tr, iface: iface, dest: dest, hub: hub, hubHash: dest.GetHash(), id: id}
	t.Cleanup(func() {
		hub.Close()
		_ = iface.Stop()
		_ = tr.Close()
	})
	identity.Remember(nil, h.hubHash, id.GetPublicKey(), nil)
	return h
}

func addr(port int) string {
	return "127.0.0.1:" + strconv.Itoa(port)
}

func hubHashHex(h *interopGoHub) string {
	return hex.EncodeToString(h.hubHash)
}
