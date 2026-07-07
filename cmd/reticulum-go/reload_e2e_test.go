// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/node"
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
	cfg.ShareInstance = false
	cfg.Interfaces = map[string]*common.InterfaceConfig{
		"udpe2e": {Name: "udpe2e", Type: "UDPInterface", Address: "127.0.0.1:0", TargetHost: "", Enabled: true},
	}
	n, err := node.New(cfg)
	if err != nil {
		t.Fatalf("node.New: %v", err)
	}
	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	r := &Reticulum{Node: n, config: cfg}
	cleanup := func() {
		_ = n.Stop()
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
	if n := len(r.Transport().GetInterfaces()); n != 0 {
		t.Fatalf("expected 0 registered ifaces, got %d", n)
	}

	on := cloneReticulumCfg(off)
	on.Interfaces["udpe2e"].Enabled = true
	if err := r.ReloadInterfaces(on); err != nil {
		t.Fatal(err)
	}
	if n := len(r.Transport().GetInterfaces()); n != 1 {
		t.Fatalf("expected 1 registered iface, got %d", n)
	}
}

func TestStopAfterReloadSerial(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	r, cleanup := minimalReticulumUDP(t)
	defer cleanup()
	for i := range 8 {
		cfg := cloneReticulumCfg(r.config)
		cfg.Interfaces["udpe2e"].Enabled = i%2 == 0
		if err := r.ReloadInterfaces(cfg); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.StopDaemon(); err != nil {
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
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := cloneReticulumCfg(r.config)
			cfg.Interfaces["udpe2e"].Enabled = true
			_ = r.ReloadInterfaces(cfg)
		}()
	}
	time.Sleep(10 * time.Millisecond)
	_ = r.StopDaemon()
	wg.Wait()
}
