// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux && !tinygo

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/landlock-lsm/go-landlock/landlock"
	"golang.org/x/sys/unix"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

func applyPlatform(cfg *common.ReticulumConfig) error {
	strict := cfg != nil && cfg.SandboxStrict

	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		debug.Log(debug.DebugError, "PR_SET_NO_NEW_PRIVS failed", "error", err)
		if strict {
			return err
		}
	}

	if err := applyLandlock(cfg); err != nil {
		debug.Log(debug.DebugError, "Landlock failed", "error", err)
		warnSoftUnavailable("landlock", err.Error())
		if strict {
			return err
		}
	}

	if os.Geteuid() == 0 {
		if err := dropAllCapabilities(); err != nil {
			debug.Log(debug.DebugError, "Capability drop failed", "error", err)
		}
		if err := unix.Unshare(unix.CLONE_NEWNS); err != nil {
			debug.Log(debug.DebugError, "Unshare(CLONE_NEWNS) failed", "error", err)
		} else {
			_ = unix.Mount("none", "/", "", unix.MS_REC|unix.MS_PRIVATE, "")
		}
	} else {
		debug.Log(debug.DebugVerbose, "Skipping privileged sandbox steps (not root)")
	}

	if err := setResourceLimits(); err != nil {
		debug.Log(debug.DebugError, "Setrlimit failed", "error", err)
	}

	if err := applySeccomp(cfg); err != nil {
		if strict {
			return err
		}
	}

	debug.Log(debug.DebugInfo, "Sandbox applied", "platform", "linux")
	return nil
}

// applyLandlock restricts filesystem access and IPC scopes with Landlock ABI
// V9 via go-landlock. TCP port rules are intentionally omitted so the P2P
// mesh can bind and dial arbitrary peers. BestEffort downgrades on older
// kernels. Missing optional paths are ignored.
func applyLandlock(cfg *common.ReticulumConfig) error {
	// Go AllThreadsSyscall (Landlock restrict without TSYNC) fatals under
	// qemu-user when per-thread results diverge. Skip rather than crash.
	if os.Getenv("RETICULUM_QEMU_USER") == "1" {
		return fmt.Errorf("landlock skipped under qemu-user")
	}

	if err := probeLandlock(); err != nil {
		return err
	}

	rules, err := landlockPathRules(cfg)
	if err != nil {
		return err
	}

	// RestrictPaths only. Do not call Restrict or RestrictNet: V4+ would
	// deny TCP bind/connect unless every mesh port is allowlisted.
	if err := landlock.V9.BestEffort().RestrictPaths(rules...); err != nil {
		return fmt.Errorf("landlock restrict paths: %w", err)
	}

	// V6+ scopes abstract UNIX sockets and signals toward more privileged
	// domains. Pathname UNIX sockets (session bus, journald) need
	// WithResolveUnix on their path trees instead. GUI hosts skip this
	// because WebKitGTK helpers use abstract sockets and signals.
	if shouldRestrictScoped(cfg) {
		if err := landlock.V9.BestEffort().RestrictScoped(); err != nil {
			return fmt.Errorf("landlock restrict scoped: %w", err)
		}
	}

	debug.Log(debug.DebugInfo, "Landlock sandbox applied", "abi", "V9")
	return nil
}

func shouldRestrictScoped(cfg *common.ReticulumConfig) bool {
	return cfg == nil || !cfg.SandboxSkipScoped
}

func probeLandlock() error {
	_, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET,
		0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno == unix.ENOSYS {
		return fmt.Errorf("landlock not supported by kernel")
	}
	if errno == unix.EOPNOTSUPP {
		return fmt.Errorf("landlock is currently disabled")
	}
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset version check: %w", errno)
	}
	return nil
}

func landlockPathRules(cfg *common.ReticulumConfig) ([]landlock.Rule, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	homeCfg := filepath.Join(home, ".reticulum-go")

	rwDirs := []string{homeCfg, "/tmp", "/var/tmp"}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	// ResolveUnix covers pathname session bus under XDG_RUNTIME_DIR (V9).
	runtimeRule := landlock.RWDirs(runtimeDir).WithResolveUnix().IgnoreIfMissing()

	if cfg != nil && cfg.ConfigPath != "" {
		parent := filepath.Dir(cfg.ConfigPath)
		if parent != "" && parent != "." && parent != homeCfg {
			rwDirs = append(rwDirs, parent)
		}
	}
	if cfg != nil && cfg.LogFile != "" {
		logParent := filepath.Dir(cfg.LogFile)
		if logParent != "" && logParent != "." {
			rwDirs = append(rwDirs, logParent)
		}
	}

	rules := []landlock.Rule{
		landlock.RWDirs(rwDirs...).IgnoreIfMissing(),
		runtimeRule,
		// Journald pathname socket (V9 resolve unix). Read-only tree.
		landlock.RODirs("/run/systemd").WithResolveUnix().IgnoreIfMissing(),
		landlock.ROFiles(
			"/etc/resolv.conf",
			"/etc/hosts",
			"/etc/ssl/cert.pem",
			"/dev/null",
			"/dev/urandom",
			"/etc/localtime",
			"/etc/protocols",
			"/etc/services",
		).IgnoreIfMissing(),
		// Syslog pathname socket (often a symlink into /run).
		landlock.ROFiles("/dev/log").WithResolveUnix().IgnoreIfMissing(),
		landlock.RODirs(
			"/etc/ssl/certs",
			"/proc/self",
			"/lib",
			"/lib64",
			"/usr/lib",
		).IgnoreIfMissing(),
	}
	if !isRouterProfile(cfg) {
		rules = append(rules, landlock.RODirs(
			"/bin",
			"/usr/bin",
			"/usr/local/bin",
		).IgnoreIfMissing())
	}
	rules = append(rules, extraLandlockRules(cfg)...)
	return rules, nil
}

func extraLandlockRules(cfg *common.ReticulumConfig) []landlock.Rule {
	var rules []landlock.Rule
	for _, p := range collectExtraPaths(cfg) {
		switch p.kind {
		case pathRWDir:
			rules = append(rules, landlock.RWDirs(p.path).IgnoreIfMissing())
		case pathRODir:
			rules = append(rules, landlock.RODirs(p.path).IgnoreIfMissing())
		case pathROFile:
			rules = append(rules, landlock.ROFiles(p.path).IgnoreIfMissing())
		default:
			rules = append(rules, landlock.RWFiles(p.path).WithIoctlDev().IgnoreIfMissing())
		}
	}
	return rules
}

func dropAllCapabilities() error {
	lastCap, err := readCapLastCap()
	if err != nil {
		lastCap = 40
	}

	var dropped int
	for capIdx := 0; capIdx <= lastCap; capIdx++ {
		err := unix.Prctl(unix.PR_CAPBSET_DROP, uintptr(capIdx), 0, 0, 0)
		if err == nil {
			dropped++
		}
	}

	if dropped == 0 && lastCap > 0 {
		return fmt.Errorf("no capabilities dropped")
	}
	debug.Log(debug.DebugInfo, "Capabilities dropped", "count", dropped)
	return nil
}

func readCapLastCap() (int, error) {
	data, err := os.ReadFile("/proc/sys/kernel/cap_last_cap")
	if err != nil {
		return 0, err
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	return v, nil
}

func setResourceLimits() error {
	const maxFDs = 65536
	if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &unix.Rlimit{Cur: maxFDs, Max: maxFDs}); err != nil {
		debug.Log(debug.DebugError, "RLIMIT_NOFILE failed", "error", err)
	}

	// Do not set RLIMIT_AS. A 2GiB address-space cap aborts Go under normal
	// mesh load (runtime: out of memory / unknown pc during GC).

	if err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		debug.Log(debug.DebugError, "RLIMIT_CORE failed", "error", err)
	}

	const stackLimit = 8 << 20 // 8 MiB
	if err := unix.Setrlimit(unix.RLIMIT_STACK, &unix.Rlimit{Cur: stackLimit, Max: unix.RLIM_INFINITY}); err != nil {
		debug.Log(debug.DebugError, "RLIMIT_STACK failed", "error", err)
	}

	const procLimit = 65536
	if err := unix.Setrlimit(unix.RLIMIT_NPROC, &unix.Rlimit{Cur: procLimit, Max: procLimit}); err != nil {
		debug.Log(debug.DebugError, "RLIMIT_NPROC failed", "error", err)
	}

	// Raise soft MEMLOCK so identity securemem pages can mlock (a few KB).
	var memlock unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_MEMLOCK, &memlock); err == nil {
		const want = 64 << 10 // 64 KiB
		if memlock.Cur < want {
			memlock.Cur = want
			if memlock.Max < want && memlock.Max != unix.RLIM_INFINITY {
				memlock.Max = want
			}
			if err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, &memlock); err != nil {
				debug.Log(debug.DebugVerbose, "RLIMIT_MEMLOCK raise failed", "error", err)
			}
		}
	}

	return nil
}
