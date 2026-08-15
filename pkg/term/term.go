// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package term

import (
	"io"
	"os"
)

// FileOf returns w when it is an *os.File, otherwise nil.
func FileOf(w io.Writer) *os.File {
	if f, ok := w.(*os.File); ok {
		return f
	}
	return nil
}

// ColorEnabled reports whether ANSI colors should be used for w.
// Honors NO_COLOR (disable), FORCE_COLOR / CLICOLOR_FORCE (enable).
func ColorEnabled(w *os.File) bool {
	if w == nil {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" || os.Getenv("CLICOLOR_FORCE") != "" {
		return true
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

func wrap(w *os.File, code, s string) string {
	if !ColorEnabled(w) {
		return s
	}
	return code + s + "\033[0m"
}

// Green wraps s in green ANSI when enabled.
func Green(w *os.File, s string) string {
	return wrap(w, "\033[32m", s)
}

// Red wraps s in red ANSI when enabled.
func Red(w *os.File, s string) string {
	return wrap(w, "\033[31m", s)
}

// Yellow wraps s in yellow ANSI when enabled.
func Yellow(w *os.File, s string) string {
	return wrap(w, "\033[33m", s)
}

// Cyan wraps s in cyan ANSI when enabled.
func Cyan(w *os.File, s string) string {
	return wrap(w, "\033[36m", s)
}

// Bold wraps s in bold ANSI when enabled.
func Bold(w *os.File, s string) string {
	return wrap(w, "\033[1m", s)
}

// GreenW colors s green when w is a color-enabled *os.File.
func GreenW(w io.Writer, s string) string {
	return Green(FileOf(w), s)
}

// RedW colors s red when w is a color-enabled *os.File.
func RedW(w io.Writer, s string) string {
	return Red(FileOf(w), s)
}

// YellowW colors s yellow when w is a color-enabled *os.File.
func YellowW(w io.Writer, s string) string {
	return Yellow(FileOf(w), s)
}

// CyanW colors s cyan when w is a color-enabled *os.File.
func CyanW(w io.Writer, s string) string {
	return Cyan(FileOf(w), s)
}

// BoldW colors s bold when w is a color-enabled *os.File.
func BoldW(w io.Writer, s string) string {
	return Bold(FileOf(w), s)
}
