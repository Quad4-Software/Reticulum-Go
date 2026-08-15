// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"errors"
	"sync"
	"testing"
)

func TestMemoryBudgetConcurrentReserveReleaseRace(t *testing.T) {
	b := NewMemoryBudget(1 << 20)
	var wg sync.WaitGroup
	const workers = 32
	const iters = 200
	for range workers {
		wg.Go(func() {
			for range iters {
				if err := b.TryReserve(64); err != nil {
					if !errors.Is(err, ErrMemoryBudgetExceeded) {
						t.Errorf("unexpected reserve error: %v", err)
					}
					continue
				}
				b.Release(64)
			}
		})
	}
	wg.Wait()
	if used := b.Used(); used < 0 {
		t.Fatalf("used went negative: %d", used)
	}
}

func TestConfigProviderGetters(t *testing.T) {
	var nilCfg *ReticulumConfig
	if nilCfg.GetConfigPath() != "" {
		t.Fatal("nil GetConfigPath")
	}
	if nilCfg.GetLogLevel() != DefaultLogLevel {
		t.Fatal("nil GetLogLevel")
	}
	if len(nilCfg.GetInterfaces()) != 0 {
		t.Fatal("nil GetInterfaces")
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = "/tmp/rns.cfg"
	cfg.LogLevel = 3
	cfg.Interfaces = map[string]*InterfaceConfig{
		"udp0": {Type: "UDPInterface", Enabled: true},
		"skip": nil,
	}
	if cfg.GetConfigPath() != "/tmp/rns.cfg" {
		t.Fatal("GetConfigPath")
	}
	if cfg.GetLogLevel() != 3 {
		t.Fatal("GetLogLevel")
	}
	ifaces := cfg.GetInterfaces()
	if len(ifaces) != 1 || ifaces["udp0"].Type != "UDPInterface" {
		t.Fatalf("GetInterfaces = %#v", ifaces)
	}
}

func TestSharedInstanceUsesUnix(t *testing.T) {
	if !SharedInstanceUsesUnix(SharedInstanceUnix) {
		t.Fatal("unix should use unix")
	}
	if SharedInstanceUsesUnix(SharedInstanceTCP) {
		t.Fatal("tcp should not use unix")
	}
	_ = SharedInstanceUsesUnix("")
	_ = SharedInstanceUsesUnix("bogus")
}
