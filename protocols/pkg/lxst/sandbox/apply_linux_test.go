//go:build linux

// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestApplyLinux(t *testing.T) {
	if os.Getenv("RGESP_TEST_SANDBOX_CHILD") != "" {
		os.Exit(runSandboxChild())
	}

	t.Run("ignoresDisableEnv", func(t *testing.T) {
		dir := t.TempDir()
		out, err := runChild(t, "disable-env", dir, map[string]string{
			"RGESP_NOSANDBOX": "1",
			"NOSANDBOX":       "1",
			"RGESP_SANDBOX":   "0",
			"LANDLOCK":        "off",
			"SECCOMP":         "0",
		})
		if err != nil {
			t.Fatalf("child: %v\n%s", err, out)
		}
	})

	t.Run("seccompDeniesPtrace", func(t *testing.T) {
		dir := t.TempDir()
		out, err := runChild(t, "ptrace", dir, nil)
		if exitCode(err) == 3 {
			t.Skip("seccomp not enforced on this kernel")
		}
		if err != nil {
			t.Fatalf("child: %v\n%s", err, out)
		}
	})

	t.Run("landlockDeniesSibling", func(t *testing.T) {
		allowed := t.TempDir()
		blocked := t.TempDir()
		marker := filepath.Join(blocked, "marker")
		if err := os.WriteFile(marker, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		out, err := runChild(t, "landlock", allowed, map[string]string{
			"RGESP_TEST_SANDBOX_BLOCKED": blocked,
		})
		if exitCode(err) == 4 {
			t.Skip("landlock not enforced on this kernel")
		}
		if err != nil {
			t.Fatalf("child: %v\n%s", err, out)
		}
	})
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}

func runChild(t *testing.T, kind, dir string, extra map[string]string) ([]byte, error) {
	t.Helper()
	return childCmd(t, kind, dir, extra).CombinedOutput()
}

func childCmd(t *testing.T, kind, dir string, extra map[string]string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestApplyLinux")
	cmd.Env = append(os.Environ(),
		"RGESP_TEST_SANDBOX_CHILD="+kind,
		"RGESP_TEST_SANDBOX_DIR="+dir,
	)
	for k, v := range extra {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return cmd
}

func runSandboxChild() int {
	dir := os.Getenv("RGESP_TEST_SANDBOX_DIR")
	switch os.Getenv("RGESP_TEST_SANDBOX_CHILD") {
	case "disable-env":
		if _, err := Apply(Paths{ReadWrite: []string{dir}}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		path := filepath.Join(dir, "ok")
		if err := os.WriteFile(path, []byte("ok"), 0600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	case "ptrace":
		if _, err := Apply(Paths{ReadWrite: []string{dir}}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		_, _, errno := unix.Syscall(unix.SYS_PTRACE, unix.PTRACE_TRACEME, 0, 0)
		if errno == 0 {
			return 3
		}
		if errno != unix.EPERM {
			fmt.Fprintln(os.Stderr, errno)
			return 1
		}
		return 0
	case "landlock":
		blocked := os.Getenv("RGESP_TEST_SANDBOX_BLOCKED")
		hardenProcess()
		if err := restrictLandlock(policy{
			ro: []string{"/usr", "/lib", "/lib64", "/proc"},
			rw: []string{dir},
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := os.WriteFile(filepath.Join(dir, "ok"), []byte("ok"), 0600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if _, err := os.Open(filepath.Join(blocked, "marker")); err == nil {
			return 4
		}
		return 0
	default:
		fmt.Fprintln(os.Stderr, "unknown child")
		return 1
	}
}
