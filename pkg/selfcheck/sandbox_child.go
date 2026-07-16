// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package selfcheck

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/sandbox"
)

// runSandboxChild applies the sandbox in this process and probes allowed I/O.
// Exit codes: 0 pass, 1 fail, 2 soft-unavailable / warn.
func runSandboxChild() int {
	dir := os.Getenv(envChildDir)
	if dir == "" {
		fmt.Fprintln(os.Stderr, "RETICULUM_SELFCHECK_DIR unset")
		return 1
	}
	cfgPath := filepath.Join(dir, "config")
	cfg := &common.ReticulumConfig{
		EnableSandbox: true,
		EnableSeccomp: true,
		ConfigPath:    cfgPath,
	}

	probe := filepath.Join(dir, "sandbox-probe")
	// #nosec G703 -- dir is a temp path set by the parent self-check process
	if err := os.WriteFile(probe, []byte("pre"), fileModePrivate); err != nil {
		fmt.Fprintf(os.Stderr, "pre-apply write: %v\n", err)
		return 1
	}
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pre-apply udp: %v\n", err)
		return 1
	}
	defer c.Close()

	if err := sandbox.Apply(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox apply: %v\n", err)
		return 1
	}

	// FreeBSD CapEnter and OpenBSD pledge restrict new opens after apply.
	// Pre-apply probes already proved the process could use the config dir and UDP.
	switch runtime.GOOS {
	case "freebsd", "openbsd":
		fmt.Println("sandbox child ok (capability mode)")
		return 0
	}

	// #nosec G703 -- same temp dir as pre-apply probe
	if err := os.WriteFile(probe, []byte("post"), fileModePrivate); err != nil {
		fmt.Fprintf(os.Stderr, "post-apply write: %v\n", err)
		return 1
	}
	c2, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "post-apply udp: %v\n", err)
		return 1
	}
	_ = c2.Close()

	fmt.Println("sandbox child ok")
	return 0
}
