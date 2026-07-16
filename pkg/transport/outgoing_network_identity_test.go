// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
)

type outgoingMockIface struct {
	common.BaseInterface
	sent int
}

func (m *outgoingMockIface) Send(data []byte, _ string) error {
	m.sent++
	return nil
}

func (m *outgoingMockIface) ProcessOutgoing([]byte) error { return nil }

func TestAllowsOutgoingDefaultTrue(t *testing.T) {
	m := &outgoingMockIface{BaseInterface: common.NewBaseInterface("t", common.IFTypeUDP, true)}
	m.Online = true
	if !AllowsOutgoing(m) {
		t.Fatal("expected AllowsOutgoing true by default")
	}
}

func TestSendOnInterfaceReceiveOnly(t *testing.T) {
	m := &outgoingMockIface{BaseInterface: common.NewBaseInterface("ro", common.IFTypeUDP, true)}
	m.Online = true
	m.SetOutgoingAllowed(false)
	err := sendOnInterface(m, []byte("x"), "")
	if !errors.Is(err, ErrInterfaceReceiveOnly) {
		t.Fatalf("got %v, want ErrInterfaceReceiveOnly", err)
	}
	if m.sent != 0 {
		t.Fatalf("sent %d, want 0", m.sent)
	}
}

func TestSendOnInterfaceOutgoing(t *testing.T) {
	m := &outgoingMockIface{BaseInterface: common.NewBaseInterface("tx", common.IFTypeUDP, true)}
	m.Online = true
	if err := sendOnInterface(m, []byte("x"), ""); err != nil {
		t.Fatal(err)
	}
	if m.sent != 1 {
		t.Fatalf("sent %d, want 1", m.sent)
	}
}

func TestInitializeNetworkIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "network_id")
	cfg := common.DefaultConfig()
	cfg.ConfigPath = filepath.Join(dir, "config")
	cfg.NetworkIdentityPath = path
	cfg.ShareInstance = false

	tr := NewTransport(cfg)
	defer func() { _ = tr.Close() }()

	if err := tr.InitializeNetworkIdentity(); err != nil {
		t.Fatal(err)
	}
	if !tr.HasNetworkIdentity() {
		t.Fatal("expected network identity")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("identity file: %v", err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("network identity mode %o allows group/other access", st.Mode().Perm())
	}

	id2, err := identity.FromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(tr.NetworkIdentity().Hash()) != string(id2.Hash()) {
		t.Fatal("reloaded identity hash mismatch")
	}

	tr2 := NewTransport(cfg)
	defer func() { _ = tr2.Close() }()
	if err := tr2.InitializeNetworkIdentity(); err != nil {
		t.Fatal(err)
	}
	if string(tr2.NetworkIdentity().Hash()) != string(id2.Hash()) {
		t.Fatal("second load created different identity")
	}
}

func TestSetNetworkIdentityOnce(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	tr := NewTransport(cfg)
	defer func() { _ = tr.Close() }()

	a, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	b, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr.SetNetworkIdentity(a)
	tr.SetNetworkIdentity(b)
	if string(tr.NetworkIdentity().Hash()) != string(a.Hash()) {
		t.Fatal("second SetNetworkIdentity should be ignored")
	}
}
