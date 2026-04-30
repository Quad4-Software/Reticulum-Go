// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"sync"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/buffer"
	"git.quad4.io/Networks/Reticulum-Go/pkg/channel"
	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

func cloneReticulumCfg(c *common.ReticulumConfig) *common.ReticulumConfig {
	if c == nil {
		return nil
	}
	out := *c
	out.Interfaces = make(map[string]*common.InterfaceConfig)
	for k, v := range c.Interfaces {
		if v == nil {
			continue
		}
		cv := *v
		out.Interfaces[k] = &cv
	}
	return &out
}

func minimalReticulumUDP(t *testing.T) (*Reticulum, func()) {
	t.Helper()
	cfg := common.DefaultConfig()
	cfg.Interfaces = map[string]*common.InterfaceConfig{
		"udpe2e": {Name: "udpe2e", Type: "UDPInterface", Address: "127.0.0.1:0", TargetHost: "", Enabled: true},
	}
	tr := transport.NewTransport(cfg)
	iface, err := interfaces.NewFromConfig("udpe2e", cfg.Interfaces["udpe2e"])
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	if err := iface.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ni := iface.(common.NetworkInterface)
	if err := tr.RegisterInterface("udpe2e", ni); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}
	r := &Reticulum{
		config:     cfg,
		transport:  tr,
		buffers:    make(map[string]*buffer.Buffer),
		channels:   make(map[string]*channel.Channel),
		interfaces: []interfaces.Interface{iface},
	}
	r.handleInterface(ni)
	cleanup := func() {
		r.reloadMu.Lock()
		defer r.reloadMu.Unlock()
		for _, x := range r.interfaces {
			r.transport.UnregisterInterface(x.GetName())
			_ = x.Stop()
		}
		r.interfaces = nil
		_ = tr.Close()
	}
	return r, cleanup
}

func TestReloadInterfacesDisableReenableUDP(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	r, cleanup := minimalReticulumUDP(t)
	defer cleanup()

	off := cloneReticulumCfg(r.config)
	off.Interfaces["udpe2e"].Enabled = false
	if err := r.ReloadInterfaces(off); err != nil {
		t.Fatal(err)
	}
	if n := len(r.transport.GetInterfaces()); n != 0 {
		t.Fatalf("expected 0 registered ifaces, got %d", n)
	}

	on := cloneReticulumCfg(off)
	on.Interfaces["udpe2e"].Enabled = true
	if err := r.ReloadInterfaces(on); err != nil {
		t.Fatal(err)
	}
	if n := len(r.transport.GetInterfaces()); n != 1 {
		t.Fatalf("expected 1 registered iface, got %d", n)
	}
}

func TestStopAfterReloadSerial(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	r, cleanup := minimalReticulumUDP(t)
	defer cleanup()
	for i := 0; i < 8; i++ {
		cfg := cloneReticulumCfg(r.config)
		cfg.Interfaces["udpe2e"].Enabled = i%2 == 0
		if err := r.ReloadInterfaces(cfg); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentReloadWhileStop(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	r, cleanup := minimalReticulumUDP(t)
	defer cleanup()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 40; i++ {
			cfg := cloneReticulumCfg(r.config)
			if cfg.Interfaces["udpe2e"] != nil {
				cfg.Interfaces["udpe2e"].Enabled = i%2 == 0
			}
			_ = r.ReloadInterfaces(cfg)
		}
	}()
	time.Sleep(8 * time.Millisecond)
	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}
