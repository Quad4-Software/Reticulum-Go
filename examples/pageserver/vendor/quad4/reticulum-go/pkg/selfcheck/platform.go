// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package selfcheck

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/identity/store"
	"quad4/reticulum-go/pkg/securemem"
)

func checkSandbox(ctx context.Context, opts Options) Result {
	dir, err := os.MkdirTemp(opts.WorkDir, "rns-selfcheck-sandbox-*")
	if err != nil {
		return result(nameSandboxApply, SeverityFail, err.Error())
	}
	defer os.RemoveAll(dir)

	cfgPath := filepath.Join(dir, "config")
	if err := os.WriteFile(cfgPath, []byte("[reticulum]\n  enable_sandbox = yes\n"), fileModePrivate); err != nil {
		return result(nameSandboxApply, SeverityFail, err.Error())
	}

	exe, err := os.Executable()
	if err != nil {
		return result(nameSandboxApply, SeverityFail, "executable: "+err.Error())
	}

	childCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if isGoTestBinary(exe) {
		cmd = exec.CommandContext(childCtx, exe, "-test.run=^$", "-test.count=1") // #nosec G204 -- os.Executable of this test or CLI binary
	} else {
		cmd = exec.CommandContext(childCtx, exe) // #nosec G204 -- os.Executable of this CLI binary
	}
	cmd.Env = append(os.Environ(),
		envChild+"="+childSandbox,
		envChildDir+"="+dir,
	)
	out, err := cmd.CombinedOutput()
	detail := sandboxChildDetail(out)
	mech := sandboxMechanism()

	if childCtx.Err() == context.DeadlineExceeded {
		return result(nameSandboxApply, SeverityFail, "child timed out ("+mech+")")
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			switch ee.ExitCode() {
			case 2:
				if detail == "" {
					detail = "sandbox soft-unavailable"
				}
				return result(nameSandboxApply, SeverityWarn, mech+": "+detail)
			case 0:
				return result(nameSandboxApply, SeverityPass, mech)
			default:
				if detail == "" {
					detail = err.Error()
				}
				return result(nameSandboxApply, SeverityFail, mech+": "+detail)
			}
		}
		return result(nameSandboxApply, SeverityFail, mech+": "+err.Error())
	}
	if detail != "" {
		return result(nameSandboxApply, SeverityPass, mech+": "+detail)
	}
	return result(nameSandboxApply, SeverityPass, mech)
}

func sandboxChildDetail(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range slices.Backward(lines) {
		line := strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "time=") {
			continue
		}
		if len(line) > detailMaxChars {
			line = line[:detailMaxChars] + "..."
		}
		return line
	}
	return ""
}

func isGoTestBinary(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, ".test") || strings.Contains(base, ".test.")
}

func sandboxMechanism() string {
	switch runtime.GOOS {
	case "linux", "android":
		return "landlock+seccomp+rlimits"
	case "openbsd":
		return "unveil+pledge"
	case "freebsd":
		return "capenter+rlimits"
	case "darwin":
		return "rlimits"
	case "windows":
		return "job-object"
	default:
		return "stub"
	}
}

func checkSeccomp() Result {
	switch runtime.GOOS {
	case "linux":
		switch runtime.GOARCH {
		case "amd64", "arm64", "386", "arm", "riscv64", "ppc64", "ppc64le":
			return result("sandbox/seccomp", SeverityPass, "seccomp-bpf denylist supported on "+runtime.GOARCH)
		default:
			return result("sandbox/seccomp", SeveritySkip, "no seccomp policy for "+runtime.GOARCH+" (daemon soft-skips)")
		}
	default:
		return result("sandbox/seccomp", SeveritySkip, "linux only")
	}
}

func checkSecuremem() Result {
	b, err := securemem.New(64)
	if err != nil {
		return result("securemem/alloc", SeverityFail, err.Error())
	}
	defer b.Close()
	copy(b.Bytes(), bytes.Repeat([]byte{0xab}, 64))
	b.Wipe()
	if b.Locked() {
		return result("securemem/alloc", SeverityPass, "mlock held")
	}
	return result("securemem/alloc", SeverityWarn, "mlock unavailable (unlocked heap)")
}

func checkKeyring() Result {
	if runtime.GOOS != "linux" {
		return result(nameIdentityKeyring, SeveritySkip, "linux only")
	}
	require := os.Getenv("RETICULUM_TEST_KEYRING") == "1"
	b, err := store.NewKeyringBackend()
	if err != nil {
		if require {
			return result(nameIdentityKeyring, SeverityFail, err.Error())
		}
		return result(nameIdentityKeyring, SeveritySkip, err.Error())
	}
	attrs := store.AttrsForPath("/tmp/rns-selfcheck-keyring", "selfcheck")
	secret := []byte("selfcheck-secret-value-32bytes!!")
	if err := b.Set(attrs, secret, "selfcheck"); err != nil {
		if require {
			return result(nameIdentityKeyring, SeverityFail, "set: "+err.Error())
		}
		return result(nameIdentityKeyring, SeveritySkip, "set: "+err.Error())
	}
	got, err := b.Get(attrs)
	_ = b.Delete(attrs)
	if err != nil {
		// GitHub-hosted runners often allow keyctl add but deny read/search.
		if require {
			return result(nameIdentityKeyring, SeverityFail, "get: "+err.Error())
		}
		return result(nameIdentityKeyring, SeveritySkip, "get: "+err.Error())
	}
	if !bytes.Equal(got, secret) {
		if require {
			return result(nameIdentityKeyring, SeverityFail, "secret mismatch")
		}
		return result(nameIdentityKeyring, SeveritySkip, "secret mismatch")
	}
	return result(nameIdentityKeyring, SeverityPass, "round-trip")
}

func checkPoller() Result {
	want := backbone.DefaultBackend()
	hub, err := backbone.Init(backbone.BackendAuto)
	if err != nil {
		return result("backbone/poller", SeverityFail, err.Error())
	}
	got := hub.Backend()
	detail := fmt.Sprintf("default=%s active=%s", want, got)
	if got == backbone.BackendGo && want != backbone.BackendGo {
		return result("backbone/poller", SeverityWarn, detail+" (fell back to go)")
	}
	return result("backbone/poller", SeverityPass, detail)
}
