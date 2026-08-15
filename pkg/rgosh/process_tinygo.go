// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build tinygo

package rgosh

import "fmt"

func StartLocalProcess(req ExecRequest) (ProcessHandle, error) {
	_ = req
	return nil, fmt.Errorf("local process is not supported on TinyGo")
}

func DefaultShell() []string {
	return []string{"/bin/sh"}
}
