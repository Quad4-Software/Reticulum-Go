// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"runtime"
	"testing"
	"time"
)

func ptyRequest(cmdline ...string) ExecRequest {
	return ExecRequest{Cmdline: cmdline, Term: "xterm"}
}

// A remote stdin EOF on a PTY session used to close the master immediately,
// SIGHUPing the child and surfacing a signal exit code (-1 on the wire,
// mirrored by Python rnsh as 255). The close now sends EOT and only closes
// the master after a grace window, so a fast command keeps its real code.
func TestLocalProcessPTYStdinEOFDoesNotKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no PTY on windows")
	}
	proc, err := StartLocalProcess(ptyRequest("/bin/echo", "pty-stdin-eof-ok"))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !proc.(*LocalProcess).usePTY {
		t.Skip("PTY unavailable")
	}
	// Race the stdin EOF against the child's exit like a live session does.
	_ = proc.Stdin().Close()
	code, _ := proc.Wait()
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stdin EOF killed the child)", code)
	}
}

// A PTY child reading stdin in canonical mode must see EOT as a natural EOF
// and exit 0, not get SIGHUPed through the master close.
func TestLocalProcessPTYStdinEOFReadsAsEOF(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no PTY on windows")
	}
	proc, err := StartLocalProcess(ptyRequest("/bin/cat"))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !proc.(*LocalProcess).usePTY {
		t.Skip("PTY unavailable")
	}
	_ = proc.Stdin().Close()
	done := make(chan int, 1)
	go func() {
		code, _ := proc.Wait()
		done <- code
	}()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("cat exit code = %d, want 0 (EOT should end reads cleanly)", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cat did not exit on stdin EOF")
	}
}

// A PTY child that ignores the EOT still gets the upstream-style kill once the
// grace window lapses, and the reported code is the masked signal number, not
// -1 (Python waitpid & 0xff semantics).
func TestLocalProcessPTYStdinEOFLingeringChildKilled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no PTY on windows")
	}
	proc, err := StartLocalProcess(ptyRequest("/bin/sh", "-c", "trap '' 4; sleep 30"))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !proc.(*LocalProcess).usePTY {
		t.Skip("PTY unavailable")
	}
	_ = proc.Stdin().Close()
	done := make(chan int, 1)
	go func() {
		code, _ := proc.Wait()
		done <- code
	}()
	select {
	case code := <-done:
		if code == 0 || code == -1 {
			t.Fatalf("exit code = %d, want masked signal code for a HUPed child", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("lingering child not killed after stdin EOF grace")
	}
}

// Kill() must surface Python's "waitpid & 0xff" signal code (e.g. 9 for
// SIGKILL) rather than Go's -1, which remote rnsh clients mirror as 255.
func TestLocalProcessKillReportsSignalCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("waitstatus signal semantics are unix-only")
	}
	proc, err := StartLocalProcess(ExecRequest{Cmdline: []string{"/bin/sleep", "30"}, PipeStdin: true, PipeStdout: true, PipeStderr: true})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	code, _ := proc.Wait()
	if code != 9 {
		t.Fatalf("exit code = %d, want 9 (SIGKILL masked as waitpid & 0xff)", code)
	}
}
