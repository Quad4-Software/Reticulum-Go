// SPDX-License-Identifier: LicenseRef-Reticulum

//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxmf"
)

func main() {
	b, err := json.MarshalIndent(lxmf.MessagingInteropFieldsPython(""), "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dir := filepath.Join("pkg", "lxmf", "testdata")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	path := filepath.Join(dir, "messaging_interop_fields.json")
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(path)
}
