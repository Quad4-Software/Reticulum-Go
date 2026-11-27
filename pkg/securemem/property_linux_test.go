// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package securemem

import (
	"bytes"
	"crypto/rand"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// TestHelperProcess re-executes the test binary as a fault-injection child.
// SECUREMEM_HELPER picks the fault: "guard" writes past a dedicated
// mapping's guard page, "protnone" reads a protected buffer outside Use,
// "rlimit" drops RLIMIT_MEMLOCK to zero before allocating.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("SECUREMEM_HELPER") == "" {
		return
	}
	switch os.Getenv("SECUREMEM_HELPER") {
	case "guard":
		b, err := NewProtected(32)
		if err != nil {
			os.Exit(2)
		}
		// Write one byte into the trailing guard page.
		guard := unsafe.Slice(&b.usable[0], len(b.usable)+1)
		guard[len(guard)-1] = 1
		os.Exit(3)
	case "protnone":
		b, err := NewProtected(32)
		if err != nil {
			os.Exit(2)
		}
		// Read the PROT_NONE region without unprotecting. The value feeds a
		// global so the compiler cannot elide the load.
		sinkByte = b.usable[0]
		os.Exit(3)
	case "rlimit":
		var lim unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_MEMLOCK, &lim); err != nil {
			os.Exit(2)
		}
		lim.Cur = 0
		if err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, &lim); err != nil {
			os.Exit(2)
		}
		b, err := New(32)
		if err != nil {
			os.Exit(2)
		}
		if b.Locked() {
			os.Exit(4)
		}
		if err := b.CopyFrom(bytes.Repeat([]byte{7}, 32)); err != nil {
			os.Exit(5)
		}
		_ = b.Close()
		os.Exit(0)
	}
	os.Exit(0)
}

var sinkByte byte

// runHelper returns whether the child died from the expected memory fault.
// The Go runtime turns a fault on unmapped/protected memory into a
// "fatal error: fault" abort (exit 2) rather than a signal death, so the
// fault signature is checked on stderr as well as the wait status.
func runHelper(t *testing.T, mode string) bool {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = append(os.Environ(), "SECUREMEM_HELPER="+mode)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return false
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("helper error: %v", err)
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() && ws.Signal() == syscall.SIGSEGV {
		return true
	}
	return bytes.Contains(out, []byte("unexpected fault address")) ||
		bytes.Contains(out, []byte("SIGSEGV"))
}

// The guard page after a dedicated mapping must fault a wild write.
func TestGuardPageFaults(t *testing.T) {
	if !runHelper(t, "guard") {
		t.Fatal("guard write did not fault")
	}
}

// A protected buffer read outside Use must fault on PROT_NONE.
func TestProtectedIdleFaults(t *testing.T) {
	if !runHelper(t, "protnone") {
		t.Fatal("idle read did not fault")
	}
}

// With RLIMIT_MEMLOCK at zero the buffer soft-fails to unlocked and works.
func TestMlockDeniedSoftFail(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = append(os.Environ(), "SECUREMEM_HELPER=rlimit")
	if err := cmd.Run(); err != nil {
		t.Fatalf("rlimit helper: %v", err)
	}
}

type memRegion struct {
	start, end uintptr
	perms      string
}

func selfRegions(t *testing.T) []memRegion {
	t.Helper()
	b, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		t.Fatal(err)
	}
	var regions []memRegion
	for line := range strings.SplitSeq(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		se := strings.Split(f[0], "-")
		if len(se) != 2 {
			continue
		}
		start, err1 := strconv.ParseUint(se[0], 16, 64)
		end, err2 := strconv.ParseUint(se[1], 16, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		regions = append(regions, memRegion{uintptr(start), uintptr(end), f[1]})
	}
	return regions
}

// A wiped+closed buffer must leave no copy of the marker anywhere in the
// process's readable memory. The marker needle is kept in a protected
// buffer (PROT_NONE, unreadable to the scan) and only unprotected inside
// Use. Regions holding the needle or the scan scratch are excluded.
func TestMarkerGoneAfterClose(t *testing.T) {
	if isHeapFallback {
		t.Skip("heap fallback")
	}
	marker := make([]byte, 32)
	if _, err := rand.Read(marker); err != nil {
		t.Fatal(err)
	}
	ref, err := NewProtected(32)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ref.Close() }()
	if err := ref.CopyFrom(marker); err != nil {
		t.Fatal(err)
	}
	b, err := New(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.CopyFrom(marker); err != nil {
		t.Fatal(err)
	}
	WipeBytes(marker)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	runtime.GC()

	fd, err := os.Open("/proc/self/mem")
	if err != nil {
		t.Skipf("cannot open /proc/self/mem: %v", err)
	}
	defer func() { _ = fd.Close() }()

	hay := make([]byte, 4<<20)
	needleAddr := uintptr(unsafe.Pointer(&ref.usable[0]))
	hayAddr := uintptr(unsafe.Pointer(&hay[0]))
	skip := func(r memRegion) bool {
		if !strings.HasPrefix(r.perms, "r") {
			return true
		}
		if needleAddr >= r.start && needleAddr < r.end {
			return true
		}
		if hayAddr >= r.start && hayAddr < r.end {
			return true
		}
		return false
	}
	found := false
	err = ref.Use(func(needle []byte) {
		for _, r := range selfRegions(t) {
			if skip(r) {
				continue
			}
			size := r.end - r.start
			for off := uintptr(0); off < size; off += uintptr(len(hay)) {
				chunk := min(size-off, uintptr(len(hay)))
				if _, err := fd.ReadAt(hay[:chunk], int64(r.start+off)); err != nil {
					break
				}
				if bytes.Contains(hay[:chunk], needle) {
					found = true
					return
				}
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("marker still readable in process memory after Close")
	}
}

// Linux must flag arena mappings DONTDUMP and WIPEONFORK in smaps.
func TestMadviseFlagsApplied(t *testing.T) {
	if isHeapFallback {
		t.Skip("heap fallback")
	}
	b, err := New(32)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	addr := uintptr(unsafe.Pointer(&b.cell.ar.data[0]))
	smaps, err := os.ReadFile("/proc/self/smaps")
	if err != nil {
		t.Skipf("no smaps: %v", err)
	}
	var inRegion bool
	var flags string
	for line := range strings.SplitSeq(string(smaps), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && strings.Contains(f[0], "-") {
			se := strings.SplitN(f[0], "-", 2)
			if start, err1 := strconv.ParseUint(se[0], 16, 64); err1 == nil {
				if end, err2 := strconv.ParseUint(se[1], 16, 64); err2 == nil {
					inRegion = addr >= uintptr(start) && addr < uintptr(end)
					continue
				}
			}
		}
		if inRegion && strings.HasPrefix(line, "VmFlags:") {
			flags = line
			break
		}
	}
	for _, want := range []string{" dd", " wf"} {
		if !strings.Contains(flags, want+" ") && !strings.HasSuffix(flags, want) {
			t.Fatalf("arena region missing flag %q: %q", strings.TrimSpace(want), flags)
		}
	}
}

// The finalizer wipes the cell of a buffer whose owner forgot Close.
func TestFinalizerWipesSecret(t *testing.T) {
	if isHeapFallback {
		t.Skip("checks the native mapping")
	}
	var cellData []byte
	marker := bytes.Repeat([]byte{0x9e}, 32)
	func() {
		b, err := New(32)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.CopyFrom(marker); err != nil {
			t.Fatal(err)
		}
		off := b.cell.idx * b.cell.ar.stride
		cellData = b.cell.ar.data[off : off+b.cell.ar.stride]
	}()
	for range 3 {
		runtime.GC()
	}
	if !bytes.Equal(cellData, make([]byte, len(cellData))) {
		t.Fatal("abandoned cell was not wiped by finalizer")
	}
}
