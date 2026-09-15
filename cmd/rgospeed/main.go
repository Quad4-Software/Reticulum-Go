// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Command rgospeed is a thin wrapper around reticulum-go speedtest.
package main

import (
	"os"

	"github.com/Quad4-Software/Reticulum-Go/pkg/cli"
)

func main() {
	os.Exit(cli.RunSpeedtest(os.Args[1:]))
}
