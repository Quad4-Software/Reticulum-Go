// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Quad4-Software/reticulum-go-protocols/internal/lxstcli"
)

func resolveConfigDir(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	var candidates []string
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".config", "rnphone"), filepath.Join(home, ".rnphone"))
	}
	candidates = append(candidates, "/etc/rnphone")
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c, nil
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, ".config", "rnphone")
		if err := os.MkdirAll(dir, appcli.DirMode); err != nil {
			return "", err
		}
		return dir, nil
	}
	return "", fmt.Errorf("no home directory")
}
