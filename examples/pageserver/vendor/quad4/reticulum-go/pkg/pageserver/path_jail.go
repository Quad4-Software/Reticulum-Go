// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package pageserver

import (
	"os"
	"path/filepath"
	"strings"
)

// resolveJailedPath joins and cleans path under baseDir, then resolves
// symlinks in both baseDir and the result before checking containment.
// A Clean+HasPrefix check alone is not enough: a symlink inside baseDir
// (or a symlinked ancestor of baseDir itself) can point outside the jail
// while the unresolved string still looks contained. The returned path is
// safe to open directly.
func resolveJailedPath(baseDir, joinedPath string) (string, bool) {
	baseDir = filepath.Clean(baseDir)
	joinedPath = filepath.Clean(joinedPath)

	if joinedPath != baseDir && !strings.HasPrefix(joinedPath, baseDir+string(os.PathSeparator)) {
		return "", false
	}

	resolvedBase, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		resolvedBase = baseDir
	}

	resolved, ok := evalExistingAncestor(joinedPath)
	if !ok {
		return "", false
	}

	if resolved != resolvedBase && !strings.HasPrefix(resolved, resolvedBase+string(os.PathSeparator)) {
		return "", false
	}
	return resolved, true
}

// evalExistingAncestor resolves symlinks in path, walking up to the nearest
// existing ancestor when the leaf (or more) does not exist yet, then
// reattaches the missing suffix unresolved. This lets a legitimate 404 for a
// path under a real, non-symlinked directory still resolve, while a symlinked
// ancestor cannot smuggle a request outside the jail just because the final
// component happens not to exist.
func evalExistingAncestor(path string) (string, bool) {
	suffix := ""
	cur := path
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			if suffix == "" {
				return resolved, true
			}
			return filepath.Join(resolved, suffix), true
		}
		if !os.IsNotExist(err) {
			return "", false
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		base := filepath.Base(cur)
		if suffix == "" {
			suffix = base
		} else {
			suffix = filepath.Join(base, suffix)
		}
		cur = parent
	}
}
