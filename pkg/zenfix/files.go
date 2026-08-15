// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func listGoFiles(dir string, patterns []string, includeTests bool) ([]string, error) {
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dir = wd
	}
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	seen := make(map[string]struct{})
	var out []string
	for _, pat := range patterns {
		files, err := listGoFilesPattern(dir, pat, includeTests)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if _, ok := seen[f]; ok {
				continue
			}
			seen[f] = struct{}{}
			out = append(out, f)
		}
	}
	return out, nil
}

func listGoFilesPattern(dir, pattern string, includeTests bool) ([]string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		pattern = "."
	}
	recursive := false
	switch {
	case pattern == "./..." || pattern == "...":
		recursive = true
		pattern = "."
	case strings.HasSuffix(pattern, "/..."):
		recursive = true
		pattern = strings.TrimSuffix(pattern, "/...")
	}
	root := pattern
	if !filepath.IsAbs(root) {
		root = filepath.Join(dir, root)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if isGoSource(root, includeTests) {
			return []string{root}, nil
		}
		return nil, nil
	}
	if !recursive {
		return listGoFilesInDir(root, includeTests)
	}
	var out []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && skipDirName(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if isGoSource(path, includeTests) {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

func listGoFilesInDir(dir string, includeTests bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if isGoSource(path, includeTests) {
			out = append(out, path)
		}
	}
	return out, nil
}

func skipDirName(name string) bool {
	if name == "vendor" || name == "testdata" {
		return true
	}
	if name == "" {
		return true
	}
	c := name[0]
	return c == '.' || c == '_'
}

func isGoSource(path string, includeTests bool) bool {
	if !strings.HasSuffix(path, ".go") {
		return false
	}
	base := filepath.Base(path)
	if strings.HasSuffix(base, "_test.go") && !includeTests {
		return false
	}
	return true
}
