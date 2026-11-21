// SPDX-License-Identifier: LicenseRef-Reticulum

// Package sandbox applies Linux Landlock and seccomp-bpf to RGESP CLIs.
// Restrictions cannot be disabled by environment variables, flags, or config keys.
package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Paths are extra directories the process must keep after restriction.
type Paths struct {
	ReadWrite []string
}

// Report is the outcome of Apply. Empty strings mean the layer was not used.
type Report struct {
	Landlock string
	Seccomp  string
}

var applyOnce sync.Once
var applyRep Report
var applyErr error

// Apply hardens the process and installs Landlock plus seccomp-bpf.
// It ignores environment variables and cannot be turned off.
func Apply(paths Paths) (Report, error) {
	applyOnce.Do(func() {
		applyRep, applyErr = applyAll(paths)
	})
	return applyRep, applyErr
}

func applyAll(paths Paths) (Report, error) {
	hardenProcess()
	pol := buildPolicy(paths)
	var rep Report
	if err := restrictLandlock(pol); err != nil {
		return rep, fmt.Errorf("landlock: %w", err)
	}
	rep.Landlock = landlockStatus()
	if err := restrictSeccomp(); err != nil {
		return rep, fmt.Errorf("seccomp: %w", err)
	}
	rep.Seccomp = seccompStatus()
	return rep, nil
}

func landlockStatus() string {
	if landlockAvailable() {
		return "on"
	}
	return "unavailable"
}

func seccompStatus() string {
	if seccompAvailable() {
		return "on"
	}
	return "unavailable"
}

// ForbiddenConfigKey reports keys that must never configure or disable the sandbox.
func ForbiddenConfigKey(k string) bool {
	k = strings.ToLower(strings.TrimSpace(k))
	k = strings.ReplaceAll(k, "_", "-")
	switch k {
	case "sandbox", "no-sandbox", "nosandbox",
		"disable-sandbox", "enable-sandbox", "sandbox-disable",
		"seccomp", "no-seccomp", "disable-seccomp",
		"landlock", "no-landlock", "disable-landlock":
		return true
	default:
		return false
	}
}

func cleanAbs(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.ContainsRune(p, 0) {
		return ""
	}
	p = filepath.Clean(p)
	if !filepath.IsAbs(p) {
		abs, err := filepath.Abs(p)
		if err != nil {
			return ""
		}
		p = abs
	}
	return p
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
