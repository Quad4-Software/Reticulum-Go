// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package sandbox

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestSetResourceLimits_DoesNotCapAddressSpace verifies the daemon sandbox
// no longer installs a 2GiB RLIMIT_AS. That cap aborts Go under mesh load.
func TestSetResourceLimits_DoesNotCapAddressSpace(t *testing.T) {
	var before unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_AS, &before); err != nil {
		t.Fatal(err)
	}
	if err := setResourceLimits(); err != nil {
		t.Fatal(err)
	}
	var after unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_AS, &after); err != nil {
		t.Fatal(err)
	}
	if after.Cur != before.Cur || after.Max != before.Max {
		t.Fatalf("RLIMIT_AS changed by setResourceLimits: before=%+v after=%+v", before, after)
	}
	const oldBadCap = 2 << 30
	if after.Cur == oldBadCap && after.Max == oldBadCap {
		t.Fatal("RLIMIT_AS must not be capped at 2GiB")
	}
}

// TestRegression_RLIMIT_AS_2GiBAbortsGo reproduces the daemon crash mode:
// with RLIMIT_AS=2GiB, heap growth under load fatals the runtime.
func TestRegression_RLIMIT_AS_2GiBAbortsGo(t *testing.T) {
	if os.Getenv("SANDBOX_AS_OOM_HELPER") == "1" {
		const memLimit = 2 << 30
		if err := unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: memLimit, Max: memLimit}); err != nil {
			t.Fatalf("setrlimit: %v", err)
		}
		var hold [][]byte
		for range 200000 {
			b := make([]byte, 64*1024)
			for j := 0; j < len(b); j += 4096 {
				b[j] = 1
			}
			hold = append(hold, b)
		}
		t.Fatalf("expected OOM abort, retained %d slabs", len(hold))
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRegression_RLIMIT_AS_2GiBAbortsGo", "-test.v")
	cmd.Env = append(os.Environ(), "SANDBOX_AS_OOM_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("helper should have aborted under RLIMIT_AS=2GiB:\n%s", out)
	}
	text := string(out)
	// Under -race, ThreadSanitizer often dies on the AS cap before the Go
	// runtime can print "out of memory". That still proves the 2GiB limit bites.
	if !strings.Contains(text, "out of memory") &&
		!strings.Contains(text, "fatal error") &&
		!strings.Contains(text, "ThreadSanitizer failed to allocate") {
		t.Fatalf("expected Go OOM fatal or TSAN AS failure, got err=%v out:\n%s", err, text)
	}
}

// TestSetResourceLimits_AllowsLargeHeap confirms current limits survive a
// multi-GiB allocation pattern that used to kill the sandboxed daemon.
func TestSetResourceLimits_AllowsLargeHeap(t *testing.T) {
	if os.Getenv("SANDBOX_AS_OK_HELPER") == "1" {
		if err := setResourceLimits(); err != nil {
			t.Fatal(err)
		}
		var hold [][]byte
		// ~1.5GiB touched heap, previously fatal under 2GiB RLIMIT_AS once
		// runtime overhead and GC arenas are counted.
		for i := range 24000 {
			b := make([]byte, 64*1024)
			for j := 0; j < len(b); j += 4096 {
				b[j] = byte(i)
			}
			hold = append(hold, b)
		}
		time.Sleep(50 * time.Millisecond)
		if len(hold) != 24000 {
			t.Fatalf("hold len=%d", len(hold))
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestSetResourceLimits_AllowsLargeHeap", "-test.v")
	cmd.Env = append(os.Environ(), "SANDBOX_AS_OK_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("large heap under current sandbox limits failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PASS") {
		t.Fatalf("helper did not pass:\n%s", out)
	}
}
