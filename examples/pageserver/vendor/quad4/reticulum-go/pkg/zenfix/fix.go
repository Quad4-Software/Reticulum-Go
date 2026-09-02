// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"bytes"
	"fmt"
	"os"
)

func applyFixes(findings []Finding) (int, error) {
	byFile := make(map[string][]Finding)
	for _, f := range findings {
		if f.Fix == nil {
			continue
		}
		byFile[f.File] = append(byFile[f.File], f)
	}
	fixed := 0
	for file, list := range byFile {
		data, err := os.ReadFile(file) // #nosec G304 -- paths from findings under scan root
		if err != nil {
			return fixed, fmt.Errorf("read %s: %w", file, err)
		}
		content := data
		for _, f := range list {
			if f.Fix == nil || len(f.Fix.From) == 0 {
				continue
			}
			if !bytes.Contains(content, f.Fix.From) {
				continue
			}
			content = bytes.Replace(content, f.Fix.From, f.Fix.To, 1)
			fixed++
		}
		if !bytes.Equal(content, data) {
			info, err := os.Stat(file)
			perm := os.FileMode(0o644)
			if err == nil {
				perm = info.Mode().Perm()
			}
			if err := os.WriteFile(file, content, perm); err != nil { // #nosec G703 -- paths from findings under scan root
				return fixed, fmt.Errorf("write %s: %w", file, err)
			}
		}
	}
	return fixed, nil
}
