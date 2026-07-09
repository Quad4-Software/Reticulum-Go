// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package term

import (
	"os"
)

// ColorEnabled reports whether ANSI colors should be used for w.
// Honors NO_COLOR (disable), FORCE_COLOR / CLICOLOR_FORCE (enable).
func ColorEnabled(w *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" || os.Getenv("CLICOLOR_FORCE") != "" {
		return true
	}
	if w == nil {
		return false
	}
	fi, err := w.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// ProgressClear returns the ANSI clear-to-end-of-line sequence when color/TTY
// progress is enabled, otherwise a plain carriage return.
func ProgressClear(w *os.File) string {
	if ColorEnabled(w) {
		return "\r\033[2K"
	}
	return "\r"
}

// Green wraps s in green ANSI when enabled.
func Green(w *os.File, s string) string {
	if !ColorEnabled(w) {
		return s
	}
	return "\033[32m" + s + "\033[0m"
}

// Red wraps s in red ANSI when enabled.
func Red(w *os.File, s string) string {
	if !ColorEnabled(w) {
		return s
	}
	return "\033[31m" + s + "\033[0m"
}
