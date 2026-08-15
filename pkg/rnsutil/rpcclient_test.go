// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"runtime"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestDialRPCTargets(t *testing.T) {
	tcpCfg := &common.ReticulumConfig{
		SharedInstanceType:  common.SharedInstanceTCP,
		InstanceControlPort: 37429,
		RPCKey:              make([]byte, 32),
	}
	c, err := DialRPC(tcpCfg, tcpCfg.RPCKey)
	if err != nil {
		t.Fatal(err)
	}
	if c.network != "tcp" || !strings.Contains(c.addr, "37429") {
		t.Fatalf("tcp target: network=%q addr=%q", c.network, c.addr)
	}
	if c.altNetwork != "" {
		t.Fatalf("explicit tcp should not set alt, got %q", c.altNetwork)
	}

	unixCfg := &common.ReticulumConfig{
		SharedInstanceType: common.SharedInstanceUnix,
		InstanceName:       "testrpc",
		RPCKey:             make([]byte, 32),
	}
	c, err = DialRPC(unixCfg, unixCfg.RPCKey)
	if err != nil {
		t.Fatal(err)
	}
	if c.network != "unix" || c.addr != "@rns/testrpc/rpc" {
		t.Fatalf("unix target: network=%q addr=%q", c.network, c.addr)
	}

	autoCfg := &common.ReticulumConfig{
		InstanceName:        "auto",
		InstanceControlPort: 37429,
		RPCKey:              make([]byte, 32),
	}
	c, err = DialRPC(autoCfg, autoCfg.RPCKey)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" {
		if c.network != "unix" || c.addr != "@rns/auto/rpc" {
			t.Fatalf("linux auto primary: network=%q addr=%q", c.network, c.addr)
		}
		if c.altNetwork != "tcp" {
			t.Fatalf("linux auto alt: network=%q", c.altNetwork)
		}
	} else {
		if c.network != "tcp" {
			t.Fatalf("non-linux auto primary: network=%q", c.network)
		}
		if c.altNetwork != "unix" {
			t.Fatalf("non-linux auto alt: network=%q", c.altNetwork)
		}
	}
}
