// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

func applyPlatform(cfg *common.ReticulumConfig) error {
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		debug.Log(debug.DebugError, "PR_SET_NO_NEW_PRIVS failed", "error", err)
	}

	if err := applyLandlock(cfg); err != nil {
		debug.Log(debug.DebugError, "Landlock failed", "error", err)
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

	debug.Log(debug.DebugInfo, "Sandbox applied", "platform", "linux")
	return nil
}

// applyLandlock restricts filesystem access using the Landlock LSM
// (kernel 5.13+). It whitelists only the directories and files the daemon
// legitimately needs. On kernels without Landlock support it returns a
// descriptive error and the caller logs it as a warning.
func applyLandlock(cfg *common.ReticulumConfig) error {
	// Superset of filesystem access rights we may grant to any path.
	attr := unix.LandlockRulesetAttr{
		Access_fs: landlockAccessFS,
	}

	fd, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attr)), // #nosec G103 -- required for direct Landlock syscall interface
		uintptr(unsafe.Sizeof(attr)),
		0)
	if errno == unix.ENOSYS {
		return fmt.Errorf("landlock not supported by kernel")
	}
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset: %w", errno)
	}
	rulesetFD := int(fd) // #nosec G115 -- syscall fd is always a small non-negative integer on Linux
	defer unix.Close(rulesetFD)

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Build the whitelist. Directories get full access; files get read-only.
	paths := []landlockRule{
		{filepath.Join(home, ".reticulum-go"), landlockFullAccess},
		{"/tmp", landlockFullAccess},
		{"/var/tmp", landlockFullAccess},
		{"/etc/resolv.conf", landlockReadOnlyFile},
		{"/etc/hosts", landlockReadOnlyFile},
		{"/etc/ssl/cert.pem", landlockReadOnlyFile},
		{"/etc/ssl/certs", landlockReadOnlyDir},
		{"/proc/self", landlockReadOnlyFile},
		{"/dev/null", landlockReadOnlyFile},
		{"/dev/urandom", landlockReadOnlyFile},
	}

	// If the config lives outside ~/.reticulum-go, whitelist its parent dir.
	if cfg != nil && cfg.ConfigPath != "" {
		parent := filepath.Dir(cfg.ConfigPath)
		if parent != filepath.Join(home, ".reticulum-go") {
			paths = append(paths, landlockRule{parent, landlockFullAccess})
		}
	}

	for _, rule := range paths {
		if err := landlockAddRule(rulesetFD, rule.path, rule.access); err != nil {
			// Skip paths that do not exist; not every system has every file.
			if err != unix.ENOENT {
				debug.Log(debug.DebugError, "Landlock rule failed", "path", rule.path, "error", err)
			}
		}
	}

	_, _, errno = unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF,
		uintptr(rulesetFD), // #nosec G115 -- converting syscall fd to uintptr for raw syscall
		0, 0)
	if errno != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", errno)
	}

	debug.Log(debug.DebugInfo, "Landlock sandbox applied")
	return nil
}

type landlockRule struct {
	path   string
	access uint64
}

// landlockAccessFS is the superset of filesystem rights declared when
// creating the ruleset. Individual rules can only grant a subset.
var landlockAccessFS = uint64(
	unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR |
		unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
		unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
		unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
		unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM |
		unix.LANDLOCK_ACCESS_FS_TRUNCATE |
		unix.LANDLOCK_ACCESS_FS_REFER,
)

var landlockReadOnlyFile = uint64(unix.LANDLOCK_ACCESS_FS_READ_FILE)
var landlockReadOnlyDir = uint64(unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR)
var landlockFullAccess = landlockAccessFS

// landlockAddRule adds a single path-beneath rule to the ruleset.
// Symlinks are resolved to their targets before the rule is added, and
// directory-only rights are stripped when the target is a file.
func landlockAddRule(rulesetFD int, path string, access uint64) error {
	// Resolve symlinks so the rule applies to the real inode.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}

	fd, err := unix.Open(resolved, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)

	// Determine whether the resolved path is a file or directory so we
	// don't request directory rights on a file (that yields EINVAL).
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	allowed := access
	if !info.IsDir() {
		allowed &= uint64(unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_TRUNCATE)
	}

	attr := unix.LandlockPathBeneathAttr{
		Allowed_access: allowed,
		Parent_fd:      int32(fd), // #nosec G115 -- O_PATH fd from unix.Open is always small non-negative
	}

	_, _, errno := unix.Syscall(unix.SYS_LANDLOCK_ADD_RULE,
		uintptr(rulesetFD), // #nosec G115 -- converting syscall fd to uintptr for raw syscall
		uintptr(unix.LANDLOCK_RULE_PATH_BENEATH),
		uintptr(unsafe.Pointer(&attr))) // #nosec G103 -- required for direct Landlock syscall interface
	if errno != 0 {
		return errno
	}
	return nil
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

	const memLimit = 2 << 30 // 2 GiB
	if err := unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: memLimit, Max: memLimit}); err != nil {
		debug.Log(debug.DebugError, "RLIMIT_AS failed", "error", err)
	}

	if err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		debug.Log(debug.DebugError, "RLIMIT_CORE failed", "error", err)
	}

	const stackLimit = 8 << 20 // 8 MiB
	if err := unix.Setrlimit(unix.RLIMIT_STACK, &unix.Rlimit{Cur: stackLimit, Max: unix.RLIM_INFINITY}); err != nil {
		debug.Log(debug.DebugError, "RLIMIT_STACK failed", "error", err)
	}

	const procLimit = 4096
	if err := unix.Setrlimit(unix.RLIMIT_NPROC, &unix.Rlimit{Cur: procLimit, Max: procLimit}); err != nil {
		debug.Log(debug.DebugError, "RLIMIT_NPROC failed", "error", err)
	}

	return nil
}
