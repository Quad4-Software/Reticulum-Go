// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

// Options configures zen analysis.
type Options struct {
	// Patterns are package or file globs relative to Dir (default ./...).
	// ./... walks recursively. ./pkg/foo is that directory only.
	Patterns []string
	// Dir is the module root when Patterns are relative.
	Dir string
	// Fix applies safe automatic rewrites.
	Fix bool
	// Tests includes *_test.go files.
	Tests bool
	// Python scans .py files under Dir when set.
	Python bool
}
