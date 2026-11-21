// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

import (
	"os"
	"path/filepath"
	"strings"
)

type policy struct {
	ro []string
	rw []string
}

func defaultRO() []string {
	return []string{"/usr", "/lib", "/lib64", "/bin", "/sbin", "/etc", "/proc", "/sys"}
}

func defaultRW() []string {
	return []string{"/dev", "/run", "/tmp", "/var/tmp"}
}

func forbiddenRWRoots() []string {
	return []string{"/", "/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc", "/boot", "/sys", "/proc", "/root"}
}

func buildPolicy(paths Paths) policy {
	ro := uniqueExisting(defaultRO())
	rw := uniqueExisting(append(defaultRW(), os.TempDir()))
	for _, p := range paths.ReadWrite {
		if allowRW(p) {
			rw = append(rw, p)
		}
	}
	return policy{
		ro: uniqueExisting(ro),
		rw: uniqueExisting(rw),
	}
}

func allowRW(p string) bool {
	p = cleanAbs(p)
	if p == "" {
		return false
	}
	for _, root := range forbiddenRWRoots() {
		if underRoot(p, root) {
			return false
		}
	}
	return true
}

func uniqueExisting(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		p = cleanAbs(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		if !pathExists(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func underRoot(p, root string) bool {
	p = filepath.Clean(p)
	root = filepath.Clean(root)
	return p == root || strings.HasPrefix(p, root+string(os.PathSeparator))
}
