// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestApplyOutgoingFromConfig(t *testing.T) {
	ui, err := NewUDPInterface("u", "127.0.0.1:0", "127.0.0.1:1", true)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &common.InterfaceConfig{Type: "UDPInterface", Enabled: true, Outgoing: false, OutgoingSet: true}
	applyOutgoingFromConfig(ui, cfg)
	if ui.AllowsOutgoing() {
		t.Fatal("expected outgoing blocked")
	}
	cfg.Outgoing = true
	applyOutgoingFromConfig(ui, cfg)
	if !ui.AllowsOutgoing() {
		t.Fatal("expected outgoing allowed")
	}
}

func TestApplyOutgoingFromConfigExternalFactory(t *testing.T) {
	const typeName = "OutgoingFactoryIface"
	defer UnregisterExternalFactory(typeName)

	RegisterExternalFactory(typeName, func(name string, cfg *common.InterfaceConfig, ctx *FromConfigContext) (Interface, error) {
		return NewUDPInterface(name, "127.0.0.1:0", "127.0.0.1:9", true)
	})

	cfg := &common.InterfaceConfig{Type: typeName, Enabled: true, Outgoing: false, OutgoingSet: true}
	iface, err := NewFromConfigWithContext("factory", cfg, &FromConfigContext{})
	if err != nil {
		t.Fatal(err)
	}
	ui, ok := iface.(*UDPInterface)
	if !ok {
		t.Fatalf("got %T, want *UDPInterface", iface)
	}
	if ui.AllowsOutgoing() {
		t.Fatal("expected factory-built iface to honor outgoing=no")
	}
}

func TestPluginTypePathTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	cfg := &common.InterfaceConfig{Type: "../escape", Enabled: true}
	_, err := loadExternalInterface("bad", cfg, &FromConfigContext{ConfigDir: dir})
	if err == nil {
		t.Fatal("expected path traversal rejection")
	}
}

func TestUDPSendRejectsReceiveOnly(t *testing.T) {
	ui, err := NewUDPInterface("ro", "127.0.0.1:0", "127.0.0.1:1", true)
	if err != nil {
		t.Fatal(err)
	}
	ui.Online = true
	ui.SetOutgoingAllowed(false)
	err = ui.Send([]byte("x"), "")
	if !errors.Is(err, common.ErrInterfaceReceiveOnly) {
		t.Fatalf("got %v, want ErrInterfaceReceiveOnly", err)
	}
}

func TestLoadExternalInterfaceManifest(t *testing.T) {
	dir := t.TempDir()
	ifaceDir := filepath.Join(dir, "interfaces")
	if err := os.MkdirAll(ifaceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	man := pluginManifest{Driver: "pipe", Command: "true", RespawnDelay: 1}
	data, err := json.Marshal(man)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ifaceDir, "ExamplePlugin.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &common.InterfaceConfig{Type: "ExamplePlugin", Enabled: true}
	ctx := &FromConfigContext{ConfigDir: dir}
	iface, err := loadExternalInterface("ex", cfg, ctx)
	if err != nil {
		t.Fatal(err)
	}
	pi, ok := iface.(*PipeInterface)
	if !ok {
		t.Fatalf("got %T, want *PipeInterface", iface)
	}
	if pi.GetName() != "ex" {
		t.Fatalf("name: got %q", pi.GetName())
	}
}

func TestRegisterExternalFactory(t *testing.T) {
	const typeName = "TestFactoryIface"
	defer UnregisterExternalFactory(typeName)

	RegisterExternalFactory(typeName, func(name string, cfg *common.InterfaceConfig, ctx *FromConfigContext) (Interface, error) {
		return NewUDPInterface(name, "127.0.0.1:0", "127.0.0.1:9", true)
	})

	cfg := &common.InterfaceConfig{Type: typeName, Enabled: true}
	iface, err := NewFromConfigWithContext("factory", cfg, &FromConfigContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := iface.(*UDPInterface); !ok {
		t.Fatalf("got %T, want *UDPInterface", iface)
	}
}
