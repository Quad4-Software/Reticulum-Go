// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package term

import (
	"io"
	"os"
	"strings"
)

const reset = "\033[0m"

// FileOf returns w when it is an *os.File, otherwise nil.
func FileOf(w io.Writer) *os.File {
	if f, ok := w.(*os.File); ok {
		return f
	}
	return nil
}

// ColorEnabled reports whether ANSI colors should be used for w.
// Honors NO_COLOR and CLICOLOR=0 (disable), FORCE_COLOR and CLICOLOR_FORCE (enable).
// Default is color on interactive terminals that support ANSI, including Windows
// consoles with virtual terminal processing enabled.
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
	if v := os.Getenv("CLICOLOR"); v == "0" {
		return false
	}
	if term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM"))); term == "dumb" {
		return false
	}
	return prepareColorFile(w)
}

// ColorEnabledW is ColorEnabled for any io.Writer backed by an *os.File.
func ColorEnabledW(w io.Writer) bool {
	return ColorEnabled(FileOf(w))
}

// ProgressClear returns the ANSI clear-to-end-of-line sequence when color is
// enabled, otherwise a plain carriage return.
func ProgressClear(w *os.File) string {
	if ColorEnabled(w) {
		return "\r\033[2K"
	}
	return "\r"
}

// ClearScreen returns an ANSI full-screen clear when color is enabled.
func ClearScreen(w *os.File) string {
	if !ColorEnabled(w) {
		return ""
	}
	return "\033[2J\033[H"
}

// ClearScreenW is ClearScreen for any io.Writer backed by an *os.File.
func ClearScreenW(w io.Writer) string {
	return ClearScreen(FileOf(w))
}

func wrap(w *os.File, code, s string) string {
	if !ColorEnabled(w) {
		return s
	}
	return code + s + reset
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

// Blue wraps s in blue ANSI when enabled.
func Blue(w *os.File, s string) string {
	return wrap(w, "\033[34m", s)
}

// Magenta wraps s in magenta ANSI when enabled.
func Magenta(w *os.File, s string) string {
	return wrap(w, "\033[35m", s)
}

// Bold wraps s in bold ANSI when enabled.
func Bold(w *os.File, s string) string {
	return wrap(w, "\033[1m", s)
}

// Dim wraps s in dim ANSI when enabled.
func Dim(w *os.File, s string) string {
	return wrap(w, "\033[2m", s)
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

// BlueW colors s blue when w is a color-enabled *os.File.
func BlueW(w io.Writer, s string) string {
	return Blue(FileOf(w), s)
}

// MagentaW colors s magenta when w is a color-enabled *os.File.
func MagentaW(w io.Writer, s string) string {
	return Magenta(FileOf(w), s)
}

// BoldW colors s bold when w is a color-enabled *os.File.
func BoldW(w io.Writer, s string) string {
	return Bold(FileOf(w), s)
}

// DimW colors s dim when w is a color-enabled *os.File.
func DimW(w io.Writer, s string) string {
	return Dim(FileOf(w), s)
}
