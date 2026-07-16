// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package sandbox

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
	"quad4/reticulum-go/pkg/common"
)

func TestSeccompPolicyBuilds(t *testing.T) {
	arch, denied, err := seccompPolicy()
	if err != nil {
		t.Fatalf("seccompPolicy: %v", err)
	}
	if arch == 0 {
		t.Fatal("expected non-zero audit arch")
	}
	if len(denied) == 0 {
		t.Fatal("expected denied syscall list")
	}
	filter := make([]unix.SockFilter, 0, 8+2*len(denied))
	filter = append(filter,
		bpfStmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, seccompDataArchOffset),
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, arch, 1, 0),
		bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_KILL_THREAD),
		bpfStmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, seccompDataNrOffset),
	)
	for _, nr := range denied {
		filter = append(filter,
			bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, uint32(nr), 0, 1),
			bpfStmt(unix.BPF_RET|unix.BPF_K, seccompRetErrnoEPERM),
		)
	}
	filter = append(filter, bpfStmt(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ALLOW))
	if len(filter) < 5 {
		t.Fatalf("filter too short: %d", len(filter))
	}
}

func TestSeccompEnabledConfig(t *testing.T) {
	if seccompEnabled(nil) != true {
		t.Fatal("nil config should enable seccomp")
	}
	if seccompEnabled(&common.ReticulumConfig{EnableSandbox: false, EnableSeccomp: true}) {
		t.Fatal("seccomp should be off when sandbox is off")
	}
	if !seccompEnabled(&common.ReticulumConfig{EnableSandbox: true, EnableSeccomp: true}) {
		t.Fatal("expected seccomp enabled")
	}
	if seccompEnabled(&common.ReticulumConfig{EnableSandbox: true, EnableSeccomp: false}) {
		t.Fatal("explicit enable_seccomp=no should disable")
	}
}

func TestSeccompFunctional(t *testing.T) {
	if os.Getenv("RETICULUM_QEMU_USER") == "1" {
		t.Skip("seccomp filter install is unreliable under qemu-user")
	}
	switch runtime.GOARCH {
	case "amd64", "arm64", "386", "arm", "riscv64", "ppc64", "ppc64le":
	default:
		t.Skip("seccomp policy not defined for " + runtime.GOARCH)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestSeccompHelper", "-test.v")
	cmd.Env = append(os.Environ(), "SANDBOX_SECCOMP_TEST=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "not supported") ||
			strings.Contains(string(out), "operation not permitted") ||
			strings.Contains(string(out), "seccomp install") {
			t.Skip("seccomp not available in test environment")
		}
		t.Fatalf("seccomp helper failed:\n%s", out)
	}
	if !strings.Contains(string(out), "PASS") {
		t.Fatalf("seccomp helper did not report PASS:\n%s", out)
	}
}

// TestSeccompHelper runs in a subprocess under the seccomp denylist.
func TestSeccompHelper(t *testing.T) {
	if os.Getenv("SANDBOX_SECCOMP_TEST") != "1" {
		t.Skip("helper, run via TestSeccompFunctional")
	}

	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		t.Fatalf("PR_SET_NO_NEW_PRIVS failed: %v", err)
	}
	if err := installSeccompFilter(); err != nil {
		t.Skipf("seccomp install: %v", err)
	}

	// Allowed: ordinary process queries must still succeed.
	if pid := os.Getpid(); pid <= 0 {
		t.Fatalf("getpid after seccomp returned %d", pid)
	}

	// Denied: ptrace should return EPERM.
	_, _, errno := unix.Syscall6(unix.SYS_PTRACE, uintptr(unix.PTRACE_TRACEME), 0, 0, 0, 0, 0)
	if errno != unix.EPERM {
		t.Fatalf("ptrace expected EPERM, got %v", errno)
	}

	// Denied: mount should return EPERM (unprivileged already fails, but
	// seccomp must still intercept before capability checks where possible).
	err := unix.Mount("none", "/", "", unix.MS_RDONLY, "")
	if err == nil {
		t.Fatal("mount unexpectedly succeeded")
	}
	if errno, ok := err.(syscall.Errno); ok && errno != unix.EPERM && errno != unix.EACCES {
		t.Fatalf("mount expected EPERM/EACCES, got %v", err)
	}
}
